package ifc

import (
	"math"
	"sort"
	"strings"

	"github.com/blox-eng/goifc/geometry"
)

// StoreyEntity is one element's plan geometry on a storey: its footprint loops
// (world XY meters, Y-up), tagged by IFC class, keyed by GlobalID. Consumers
// typically resolve GlobalID to their own domain object id when rendering
// the storey's floor plan.
type StoreyEntity struct {
	GlobalID string
	IFCClass string
	Loops    []geometry.Loop
}

// StoreyPlan is one IfcBuildingStorey's 2D floor plan: the entities a horizontal
// section at cutZ = floorZ + 1.2 m draws, chosen by geometric membership.
type StoreyPlan struct {
	StoreyGlobalID string
	Elevation      float64 // meters, for UI ordering (StoreyElevations; 0 if absent)
	Entities       []StoreyEntity
}

const (
	cutOffsetM = 1.2  // architectural plan cut height above floor
	zEps       = 1e-6 // Z tolerance for span/overlap tests
	minLoopA   = 1e-9 // degenerate-loop area floor
)

// storeyBand is one storey's cut plane and Z extent, derived from its seeded floor.
type storeyBand struct {
	gid     string
	floorZ  float64
	cutZ    float64
	bandTop float64
}

// buildStoreyPlans assembles per-storey plans from the imported nodes, the
// per-GlobalID world mesh, and storey elevations. floorZ per storey is SEEDED
// from the min world-Z of its spatially-contained mesh elements (containment
// only seeds the band; geometry decides membership). Storeys with no contained
// mesh element are skipped (no reliable cut plane). Deterministic: identical
// input yields an identical slice.
func buildStoreyPlans(nodes []ImportNode, meshByGID map[string]geometry.Element, storeyElev map[string]float64) []StoreyPlan {
	// 1. Enclosing storey per node index: walk ParentIndex up to an IfcBuildingStorey.
	storeyOf := func(i int) string {
		j := i
		for {
			if strings.EqualFold(nodes[j].IFCClass, "IfcBuildingStorey") {
				return nodes[j].GlobalID
			}
			if nodes[j].ParentIndex == nil {
				return ""
			}
			j = *nodes[j].ParentIndex
		}
	}

	// 2. Seed floorZ[storeyGID] = min BBoxMin[2] over contained mesh elements.
	floorZ := make(map[string]float64)
	for i, n := range nodes {
		ge, ok := meshByGID[n.GlobalID]
		if !ok || len(ge.Verts) == 0 {
			continue
		}
		sgid := storeyOf(i)
		if sgid == "" {
			continue
		}
		if cur, seen := floorZ[sgid]; !seen || ge.BBoxMin[2] < cur {
			floorZ[sgid] = ge.BBoxMin[2]
		}
	}

	// 3. Order storeys by (floorZ, gid) ascending -> bands. cutZ = floorZ + 1.2.
	//    bandTop = next storey's floor; last storey bandTop = +Inf.
	bands := make([]storeyBand, 0, len(floorZ))
	for gid, fz := range floorZ {
		bands = append(bands, storeyBand{gid: gid, floorZ: fz})
	}
	sort.Slice(bands, func(i, j int) bool {
		if bands[i].floorZ != bands[j].floorZ {
			return bands[i].floorZ < bands[j].floorZ
		}
		return bands[i].gid < bands[j].gid
	})
	for i := range bands {
		bands[i].cutZ = bands[i].floorZ + cutOffsetM
		if i < len(bands)-1 {
			bands[i].bandTop = bands[i+1].floorZ
		} else {
			bands[i].bandTop = math.Inf(1)
		}
	}

	// 4. Membership: for each storey band, scan ALL mesh elements (in nodes order).
	plans := make([]StoreyPlan, 0, len(bands))
	for _, b := range bands {
		entities := make([]StoreyEntity, 0)
		for _, n := range nodes {
			ge, ok := meshByGID[n.GlobalID]
			if !ok || len(ge.Verts) == 0 {
				continue
			}
			zmin, zmax := ge.BBoxMin[2], ge.BBoxMax[2]
			spans := zmin-zEps <= b.cutZ && b.cutZ <= zmax+zEps  // mesh crosses the cut plane
			overlaps := zmax > b.floorZ-zEps && zmin < b.bandTop // Z-range intersects the half-open band [floorZ, bandTop)
			if !spans && !overlaps {
				continue
			}
			loops := geometry.Footprint(ge, b.cutZ) // cut if spans, else below
			loops = dropDegenerateLoops(loops)
			if len(loops) == 0 {
				continue
			}
			entities = append(entities, StoreyEntity{GlobalID: n.GlobalID, IFCClass: n.IFCClass, Loops: loops})
		}
		plans = append(plans, StoreyPlan{StoreyGlobalID: b.gid, Elevation: storeyElev[b.gid], Entities: entities})
	}
	return plans
}

// dropDegenerateLoops removes loops whose absolute polygon area is below minLoopA.
// Defensive: every element here has a mesh, so this is normally a no-op.
func dropDegenerateLoops(loops []geometry.Loop) []geometry.Loop {
	out := loops[:0]
	for _, l := range loops {
		if math.Abs(shoelaceArea(l.Points)) >= minLoopA {
			out = append(out, l)
		}
	}
	return out
}

// shoelaceArea returns the signed area of a 2D polygon ring.
func shoelaceArea(pts [][2]float64) float64 {
	n := len(pts)
	if n < 3 {
		return 0
	}
	var a float64
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		a += pts[i][0]*pts[j][1] - pts[j][0]*pts[i][1]
	}
	return a / 2
}
