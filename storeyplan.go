package ifc

import (
	"fmt"
	"math"
	"slices"
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

// planWorkPerMeshPart sizes the storey plans' work budget: the vertices read
// and triangles cut, plus the loop points shipped, per vertex and triangle of
// the model's meshes. A real element is cut on the one or two storeys it
// stands on; a crafted model of tall elements through many storeys cuts each
// of them on every storey, and past this budget its plans leave elements out.
// The corpus uses at most 1.34 per mesh part (duplex_a).
const (
	planWorkPerMeshPart = 16
	planWorkFloor       = 1 << 20
)

// planElement is one meshed node, as the storey bands test it. cost is what
// one cut of it reads: every vertex and every triangle.
type planElement struct {
	node       int
	zmin, zmax float64
	cost       int
}

// buildStoreyPlans assembles per-storey plans from the imported nodes, the
// per-GlobalID world mesh, and storey elevations. floorZ per storey is SEEDED
// from the min world-Z of its spatially-contained mesh elements (containment
// only seeds the band; geometry decides membership). Storeys with no contained
// mesh element are skipped (no reliable cut plane). Deterministic: identical
// input yields an identical slice.
//
// The work is bounded by planWorkPerMeshPart; plans past it leave elements out,
// and the returned warnings say how many.
func buildStoreyPlans(nodes []ImportNode, meshByGID map[string]geometry.Element, storeyElev map[string]float64) ([]StoreyPlan, []string) {
	storeyOf := enclosingStoreys(nodes)

	// The meshed nodes, in nodes order, read once.
	var elems []planElement
	budget := planWorkFloor
	for i, n := range nodes {
		ge, ok := meshByGID[n.GlobalID]
		if !ok || len(ge.Verts) == 0 {
			continue
		}
		cost := len(ge.Verts)/3 + len(ge.Tris)/3
		elems = append(elems, planElement{node: i, zmin: ge.BBoxMin[2], zmax: ge.BBoxMax[2], cost: cost})
		budget += planWorkPerMeshPart * cost
	}

	// 2. Seed floorZ[storeyGID] = min BBoxMin[2] over contained mesh elements.
	//    A NaN bound seeds nothing: as a floor it would become the top of the
	//    storey below, and that storey's reach would hold no element.
	floorZ := make(map[string]float64)
	for _, e := range elems {
		sgid := storeyOf[e.node]
		if sgid == "" || math.IsNaN(e.zmin) {
			continue
		}
		if cur, seen := floorZ[sgid]; !seen || e.zmin < cur {
			floorZ[sgid] = e.zmin
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

	// 4. Membership: for each storey band, the mesh elements its height
	//    reaches, in nodes order. A sweep finds them (bandReach).
	plans := make([]StoreyPlan, 0, len(bands))
	leftOut := 0
	reach := newBandReach(elems)
	for _, b := range bands {
		entities := make([]StoreyEntity, 0)
		for _, ei := range reach.within(b.floorZ-zEps, math.Max(b.cutZ+zEps, b.bandTop)) {
			e := elems[ei]
			spans := e.zmin-zEps <= b.cutZ && b.cutZ <= e.zmax+zEps  // mesh crosses the cut plane
			overlaps := e.zmax > b.floorZ-zEps && e.zmin < b.bandTop // Z-range intersects the half-open band [floorZ, bandTop)
			if !spans && !overlaps {
				continue
			}
			if e.cost > budget {
				leftOut++
				continue
			}
			budget -= e.cost
			n := nodes[e.node]
			loops := geometry.Footprint(meshByGID[n.GlobalID], b.cutZ) // cut if spans, else below
			loops = dropDegenerateLoops(loops)
			if len(loops) == 0 {
				continue
			}
			for _, l := range loops {
				budget -= len(l.Points)
			}
			entities = append(entities, StoreyEntity{GlobalID: n.GlobalID, IFCClass: n.IFCClass, Loops: loops})
		}
		plans = append(plans, StoreyPlan{StoreyGlobalID: b.gid, Elevation: storeyElev[b.gid], Entities: entities})
	}
	var warnings []string
	if leftOut > 0 {
		warnings = append(warnings, fmt.Sprintf("storey plans left out %d element sections: the model's elements cross more storeys than %d times its mesh can cut", leftOut, planWorkPerMeshPart))
	}
	return plans, warnings
}

// bandReach finds, for each storey band in turn, the elements whose height
// can reach it: those starting at or below the band's top and ending above its
// floor. Bands come in rising order, so both bounds only rise: an element joins
// once, when the top passes its lowest point, and leaves once, when the floor
// passes its highest. Each element is visited once to join and once to leave,
// however many storeys there are; testing every element against every storey
// cost storeys × elements. Elements with a NaN bound are never in reach, as no
// comparison holds for them.
type bandReach struct {
	elems  []planElement
	byLow  []int // element indices by zmin, rising
	next   int   // the next of byLow to join
	active []int // a min-heap of element indices by zmax
}

func newBandReach(elems []planElement) *bandReach {
	r := &bandReach{elems: elems}
	for i, e := range elems {
		if !math.IsNaN(e.zmin) && !math.IsNaN(e.zmax) {
			r.byLow = append(r.byLow, i)
		}
	}
	sort.SliceStable(r.byLow, func(a, b int) bool { return elems[r.byLow[a]].zmin < elems[r.byLow[b]].zmin })
	return r
}

// within returns, in elements order, the elements with zmin <= top and
// zmax > floor. floor and top must not fall from one call to the next.
func (r *bandReach) within(floor, top float64) []int {
	for r.next < len(r.byLow) && r.elems[r.byLow[r.next]].zmin <= top {
		r.push(r.byLow[r.next])
		r.next++
	}
	for len(r.active) > 0 && r.elems[r.active[0]].zmax <= floor {
		r.pop()
	}
	out := slices.Clone(r.active)
	slices.Sort(out)
	return out
}

func (r *bandReach) push(i int) {
	h := append(r.active, i)
	for c := len(h) - 1; c > 0; {
		p := (c - 1) / 2
		if r.elems[h[p]].zmax <= r.elems[h[c]].zmax {
			break
		}
		h[p], h[c] = h[c], h[p]
		c = p
	}
	r.active = h
}

func (r *bandReach) pop() {
	h := r.active
	last := len(h) - 1
	h[0] = h[last]
	h = h[:last]
	for p := 0; ; {
		c := 2*p + 1
		if c >= len(h) {
			break
		}
		if c+1 < len(h) && r.elems[h[c+1]].zmax < r.elems[h[c]].zmax {
			c++
		}
		if r.elems[h[p]].zmax <= r.elems[h[c]].zmax {
			break
		}
		h[p], h[c] = h[c], h[p]
		p = c
	}
	r.active = h
}

// enclosingStoreys returns, per node, the GlobalID of the IfcBuildingStorey it
// climbs to through ParentIndex, or "" when it climbs to none. Each node is
// climbed once: a climb stops at a node already answered, and a ParentIndex
// cycle answers "" for every node on it.
func enclosingStoreys(nodes []ImportNode) []string {
	storey := make([]string, len(nodes))
	known := make([]bool, len(nodes))
	onPath := make([]bool, len(nodes))
	var path []int
	for i := range nodes {
		path = path[:0]
		j, found := i, ""
		for {
			if known[j] {
				found = storey[j]
				break
			}
			if onPath[j] { // a cycle: no storey above it
				break
			}
			if strings.EqualFold(nodes[j].IFCClass, "IfcBuildingStorey") {
				found = nodes[j].GlobalID
				break
			}
			onPath[j] = true
			path = append(path, j)
			if nodes[j].ParentIndex == nil {
				break
			}
			j = *nodes[j].ParentIndex
		}
		for _, k := range path {
			storey[k], known[k], onPath[k] = found, true, false
		}
	}
	return storey
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
