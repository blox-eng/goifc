package geometry

import (
	"math"
	"sort"

	"github.com/blox-eng/goifc/model"
	"github.com/blox-eng/goifc/step"
)

// ElevationEntity is one element's contribution to an elevation: its outline in
// the view plane's UV coordinates, plus the openings punched through it.
type ElevationEntity struct {
	GlobalID string
	IFCClass string
	// Outline is the element's silhouette on the view plane, hole-nested. Outer
	// rings CCW, holes CW, meters.
	Outline []Loop
	// Openings are the IfcRelVoidsElement voids projected onto the SAME plane,
	// wound CW where they fall inside an outline ring so an even-odd or nonzero
	// fill renders them as cutouts.
	Openings []Loop
	// Depth is the distance from the view plane along −N to the element's
	// NEAREST point, meters. Smaller is nearer the viewer.
	Depth float64
}

// ElevationView is the orthographic view of a set of elements from one
// direction — the drawable counterpart of a floor plan.
//
// MEMBERSHIP is deliberately narrow: an element appears only when its Facing
// says it is ExposureExterior and its outward normal points at the viewer. Two
// consequences a consumer must know. A courtyard or lightwell wall is weather-
// exposed but belongs on no compass elevation, and counting it would inflate
// the drawing and the quantity together. And an element with no dominant
// vertical face family — a slab, a roof, a column — has no Facing at all, so it
// is ABSENT: this is an elevation of the facade elements, NOT a full
// orthographic render, and it carries no roof line or exposed slab edge.
//
// That narrowness is what keeps every entity here a host whose outline can be
// reconciled against its NetAreas entry.
//
// RECONCILING WITH NetAreas: outline area minus opening area equals that host's
// Net, but ONLY where the two are measured on the same plane. NetAreas projects
// each host onto ITS OWN winning axis, which for a wall square to this view is
// this plane and for a wall at any other orientation is not. On such a host the
// two numbers are both right and describe different projections, and the
// elevation's is the foreshortened one. Compare the host's winning axis with
// this plane's normal before reading a difference as drift. Measured on a
// 29 MB ArchiCAD IFC2X3 export over four compass directions: 73 of 74
// same-plane voided hosts agree to 1e-6.
type ElevationView struct {
	Plane    Plane
	Entities []ElevationEntity // sorted by (Depth, GlobalID)
	Bounds   [2][2]float64     // {{uMin, vMin}, {uMax, vMax}}, meters
}

// ElevationPlane returns the vertical plane an elevation along dir is drawn on:
// V is world up, U is horizontal, and N is dir normalized. dir points from the
// building TOWARD the viewer, matching Plane.N and the convention SilhouetteOn
// reads.
//
// This exists rather than PlaneFromNormal because a drawing has an up. The
// basis PlaneFromNormal derives is deterministic but its in-plane orientation
// is explicitly UNSPECIFIED and free to change between versions of this
// package, which is fine for measuring an area and useless for a facade
// elevation, where it would rotate the page by an arbitrary amount.
//
// ok is false for a non-finite dir, or one with no horizontal component: a view
// straight up or down is a plan, and leaves no world direction to call up on
// the page.
func ElevationPlane(dir [3]float64) (Plane, bool) {
	if !finite3(dir) {
		return Plane{}, false
	}
	// Negated so a NaN takes the same branch as a degenerate value.
	if !(math.Hypot(dir[0], dir[1]) > 1e-12) {
		return Plane{}, false
	}
	l := math.Sqrt(dotv(dir, dir))
	n := v3{dir[0] / l, dir[1] / l, dir[2] / l}
	up := v3{0, 0, 1}
	// u = v x n completes a right-handed frame (n == u x v) with v as world up.
	u := normv(crossv(up, n))
	p := Plane{U: u, V: up, N: n}
	if !p.Valid() {
		return Plane{}, false
	}
	return p, true
}

// SilhouetteOn returns the outline of e as seen from the +p.N side: the
// projected-polygon UNION of the faces opposing p.N, in p's UV coordinates
// (meters), hole-nested and tagged LoopSilhouette.
//
// This is the projection primitive FootprintOn has always used internally and
// never exposed. Where SectionOn answers "what does this plane cut through",
// this answers "what do I see looking at the solid from here" — the shape a
// facade drawing is made of, and the shape a quantity is measured on.
//
// Faces at DIFFERENT depths merge into one filled outline rather than leaving
// internal edges, so a recessed balcony, a set-back storey or a projecting bay
// reads as the one silhouette it looks like.
//
// Returns nil when p's basis is invalid, when the mesh is missing or
// degenerate, or when the boundary walk could not close the outline (see
// unionBoundary). It never substitutes a bounding box the way FootprintOn's
// last-resort branch does: a caller asking for a projection is asking a
// question a rectangle does not answer, so an absent outline is reported as
// absent.
//
// For a CLOSED solid the outline is invariant under flipping p.N. An OPEN mesh
// has no such symmetry — a one-sided surface opposes exactly one of the two
// directions and yields nothing for the other — so a caller holding non-closed
// geometry must choose p.N deliberately.
func (e Element) SilhouetteOn(p Plane) []Loop {
	if !p.Valid() || len(e.Tris) < 3 || len(e.Verts) < 9 {
		return nil
	}
	rings := silhouetteRings(worldPoints(e.Verts, e.Placement), e.Tris, p)
	if len(rings) == 0 {
		return nil
	}
	return nestEvenOdd(rings, LoopSilhouette)
}

// ElevationOn projects the scene onto p and returns the elevation drawn from
// the +p.N side. Mirrors NetAreas: it needs f and r for the same reason, since
// an element's openings live in IfcRelVoidsElement rather than in its mesh.
// [Scene.Elevations] is the entry point for a building's several facades: this
// one classifies the scene on every call, and that classification does not
// depend on the plane.
//
// Deterministic: identical input yields an identical view.
//
// An element whose outline the boundary walk cannot close is OMITTED rather
// than approximated — see Element.SilhouetteOn. The same element is reported
// untrusted by NetAreas, so the drawing and the quantity agree about what
// neither could measure.
func (s *Scene) ElevationOn(f *step.File, r *model.Result, p Plane) ElevationView {
	if !p.Valid() {
		return ElevationView{Plane: p}
	}
	return s.elevationOn(f, r, p, BuildFacings(s.Elements))
}

// Elevations projects the scene onto each of planes and returns one view per
// plane, in the same order. Identical to calling [Scene.ElevationOn] for each,
// except that the scene is classified AT MOST ONCE, on the first valid plane,
// and shared across the rest: BuildFacings rasterizes an occupancy grid over
// every element per distinct mid-height band, and its result does not depend
// on the plane. A building has four facades, so asking one at a time repeats
// that work four times over.
//
// An invalid plane yields its zero view in that position rather than being
// dropped, so the result stays index-aligned with planes, and — matching
// [Scene.ElevationOn] — never triggers BuildFacings on its own.
//
// Deterministic: identical input yields identical views.
func (s *Scene) Elevations(f *step.File, r *model.Result, planes []Plane) []ElevationView {
	if len(planes) == 0 {
		return nil
	}
	var facings map[string]Facing
	views := make([]ElevationView, len(planes))
	for i, p := range planes {
		if !p.Valid() {
			views[i] = ElevationView{Plane: p}
			continue
		}
		if facings == nil {
			facings = BuildFacings(s.Elements)
		}
		views[i] = s.elevationOn(f, r, p, facings)
	}
	return views
}

// elevationOn is the projection itself, over a classification the caller owns.
func (s *Scene) elevationOn(f *step.File, r *model.Result, p Plane, facings map[string]Facing) ElevationView {
	view := ElevationView{Plane: p}

	class := make(map[string]string, len(r.Elements))
	expressID := make(map[string]int, len(r.Elements))
	for i := range r.Elements {
		class[r.Elements[i].GlobalID] = r.Elements[i].IFCClass
		expressID[r.Elements[i].GlobalID] = r.Elements[i].ExpressID
	}

	for i := range s.Elements {
		e := &s.Elements[i]
		facing, ok := facings[e.GlobalID]
		if !ok || facing.Exposure != ExposureExterior || dotv(facing.Normal, p.N) <= 0 {
			continue
		}
		outline := e.SilhouetteOn(p)
		if len(outline) == 0 {
			continue
		}
		view.Entities = append(view.Entities, ElevationEntity{
			GlobalID: e.GlobalID,
			IFCClass: class[e.GlobalID],
			Outline:  outline,
			Openings: openingLoopsOn(f, expressID[e.GlobalID], r.UnitScale, p, outline),
			Depth:    depthOn(p, worldPoints(e.Verts, e.Placement)),
		})
	}

	sort.Slice(view.Entities, func(i, j int) bool {
		a, b := view.Entities[i], view.Entities[j]
		if a.Depth != b.Depth {
			return a.Depth < b.Depth
		}
		return a.GlobalID < b.GlobalID
	})
	view.Bounds = boundsOf(view.Entities)
	return view
}

// depthOn returns the distance from p along −N to the nearest of pts, so a
// smaller value is nearer the viewer standing on the +N side.
func depthOn(p Plane, pts []v3) float64 {
	nearest := math.Inf(-1)
	for _, q := range pts {
		if d := signedDist(p, q); d > nearest {
			nearest = d
		}
	}
	if math.IsInf(nearest, -1) {
		return 0
	}
	return -nearest
}

// openingLoopsOn projects the element's IfcRelVoidsElement voids onto p and
// returns them wound to render as cutouts.
//
// A void is wound CW when it falls inside an outline ring and left CCW when it
// does not. A void reaching past its host's silhouette is a real thing to
// draw — a window in a wall that stops short of it, or a void modelled oversize
// on purpose — and reversing it there would punch a hole through whatever lies
// behind instead of through its host.
func openingLoopsOn(f *step.File, expressID int, unitScale float64, p Plane, outline []Loop) []Loop {
	if f == nil || expressID == 0 {
		return nil
	}
	inst, ok := f.ByID(expressID)
	if !ok {
		return nil
	}
	var out []Loop
	for _, op := range model.OpeningsOf(f, inst) {
		ov, otris, src := elementMesh(f, op.ID(), unitScale)
		if src == SourceOBB || len(otris) < 3 {
			// A box around the void is not its outline; drawing one would punch a
			// rectangle where the model never had one.
			continue
		}
		// The void's LocalPlacement translation is in RAW file units and must be
		// meter-scaled, or a millimetre file's openings land ~1000x away.
		xf := scaleTransformTranslation(model.LocalPlacement(op), unitScale)
		for _, ring := range silhouetteRings(worldPoints(ov, xf), otris, p) {
			pts := ring
			if containedInOutline(ring, outline) {
				pts = reversePts(ring)
			}
			out = append(out, Loop{Role: LoopSilhouette, Points: pts})
		}
	}
	return out
}

// containedInOutline reports whether ring lies inside an OUTER (CCW) ring of
// the outline. Holes are skipped: a void sitting in a hole the host already has
// is not cutting anything.
func containedInOutline(ring [][2]float64, outline []Loop) bool {
	if len(ring) < 3 {
		return false
	}
	c := representativePoint(ring)
	for _, l := range outline {
		if len(l.Points) >= 3 && polygonArea2D(l.Points) > 0 && pointInPolygon(c, l.Points) {
			return true
		}
	}
	return false
}

// boundsOf returns the UV extent of every loop in the view, or a zero extent
// when there is nothing to bound.
func boundsOf(entities []ElevationEntity) [2][2]float64 {
	uMin, vMin := math.Inf(1), math.Inf(1)
	uMax, vMax := math.Inf(-1), math.Inf(-1)
	for _, e := range entities {
		for _, set := range [][]Loop{e.Outline, e.Openings} {
			for _, l := range set {
				for _, q := range l.Points {
					uMin, uMax = math.Min(uMin, q[0]), math.Max(uMax, q[0])
					vMin, vMax = math.Min(vMin, q[1]), math.Max(vMax, q[1])
				}
			}
		}
	}
	if math.IsInf(uMin, 1) {
		return [2][2]float64{}
	}
	return [2][2]float64{{uMin, vMin}, {uMax, vMax}}
}
