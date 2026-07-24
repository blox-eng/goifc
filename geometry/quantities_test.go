package geometry

import (
	"math"
	"os"
	"testing"

	"github.com/blox-eng/common/ifc/model"
	"github.com/blox-eng/common/ifc/step"
)

// A closed box has an exact, sign-fold-invariant volume.
func TestMeshVolume_ClosedBox(t *testing.T) {
	verts, tris := boxMesh(v3{0, 0, 0}, v3{2, 3, 4}) // 2*3*4 = 24
	v, ok := meshVolume(verts, tris)
	if !ok {
		t.Fatal("meshVolume ok=false for a closed box")
	}
	if math.Abs(v-24) > 1e-9 {
		t.Errorf("box volume = %v, want 24", v)
	}
}

// An open shell (single triangle) is not a solid — its signed sum must be rejected.
func TestMeshVolume_OpenMeshRejected(t *testing.T) {
	verts := []float32{0, 0, 0, 5, 0, 0, 0, 5, 0}
	tris := []uint32{0, 1, 2}
	if _, ok := meshVolume(verts, tris); ok {
		t.Error("meshVolume accepted an open single-triangle mesh as a volume")
	}
}

// DerivedQuantities dimensions must equal the element's world-AABB extents.
func TestDerivedQuantities_KnownBox(t *testing.T) {
	const path = "testdata/synthetic/known_box.ifc"
	if _, err := os.Stat(path); err != nil {
		t.Skip("known_box absent")
	}
	f, err := step.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	r, err := model.Extract(f)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Build(f, r)
	if err != nil {
		t.Fatal(err)
	}
	dq := s.DerivedQuantities()
	if len(s.Elements) == 0 {
		t.Fatal("no elements")
	}
	e := s.Elements[0]
	q, ok := dq[e.GlobalID]
	if !ok {
		t.Fatalf("no derived quantities for %s", e.GlobalID)
	}
	dx := e.BBoxMax[0] - e.BBoxMin[0]
	dy := e.BBoxMax[1] - e.BBoxMin[1]
	dz := e.BBoxMax[2] - e.BBoxMin[2]
	wantL, wantW := dx, dy
	if wantW > wantL {
		wantL, wantW = wantW, wantL
	}
	if q.Height == nil || math.Abs(*q.Height-dz) > 1e-9 {
		t.Errorf("Height = %v, want %v (world Z extent)", deref(q.Height), dz)
	}
	if q.Length == nil || math.Abs(*q.Length-wantL) > 1e-9 {
		t.Errorf("Length = %v, want %v", deref(q.Length), wantL)
	}
	if q.Width == nil || math.Abs(*q.Width-wantW) > 1e-9 {
		t.Errorf("Width = %v, want %v", deref(q.Width), wantW)
	}
	// Area is mesh-derived (max-side-area) now, not the AABB footprint. known_box is
	// a 2-face open shell; assert it is present and positive (its exact value is not
	// a clean golden — see TestDerivedQuantities_ClosedBox_Absolute for that).
	if q.Area == nil || *q.Area <= 0 {
		t.Errorf("Area = %v, want present and positive (mesh max-side-area)", deref(q.Area))
	}
	_ = wantW
	// Volume is present only for a closed shell; known_box is a 2-face open shell,
	// so Volume may be nil — but if present it must be a sane, in-box positive.
	if q.Volume != nil && (*q.Volume <= 0 || *q.Volume > dx*dy*dz*1.0001) {
		t.Errorf("Volume = %v out of (0, bbox=%v]", *q.Volume, dx*dy*dz)
	}
}

// TestQuantityTiering_RealFiles is the end-to-end tier check on real models
// (Extract -> Build -> ApplyDerivedQuantities). Skips when the gitignored source
// files are absent, so CI stays green without them.
func TestQuantityTiering_RealFiles(t *testing.T) {
	for _, name := range []string{"office_a", "kb645"} {
		path := "testdata/real/" + name + ".ifc"
		if _, err := os.Stat(path); err != nil {
			t.Skip(name + " absent")
		}
		t.Run(name, func(t *testing.T) {
			f, err := step.ParseFile(path)
			if err != nil {
				t.Fatal(err)
			}
			r, err := model.Extract(f)
			if err != nil {
				t.Fatal(err)
			}
			s, err := Build(f, r)
			if err != nil {
				t.Fatal(err)
			}
			derived := s.DerivedQuantities()
			r.ApplyDerivedQuantities(derived)

			meshed := map[string]bool{}
			for i := range s.Elements {
				if len(s.Elements[i].Tris) > 0 {
					meshed[s.Elements[i].GlobalID] = true
				}
			}

			var qto, geom, none int
			for i := range r.Elements {
				e := &r.Elements[i]
				switch e.QuantitySource {
				case model.QuantitySourceQto:
					qto++
				case model.QuantitySourceGeometry:
					geom++
					if e.Qto.IsEmpty() {
						t.Errorf("%s source=geometry but Qto is empty — phantom upgrade", e.GlobalID)
					}
				case model.QuantitySourceNone:
					none++
					if !e.Qto.IsEmpty() {
						t.Errorf("%s source=none but Qto not empty — no-phantom-0.0 violated", e.GlobalID)
					}
					// A meshed element may legitimately stay "none" ONLY if its
					// derived entry is empty (a fully-degenerate/zero-extent mesh).
					// A meshed element WITH real derived quantities must have been
					// upgraded — if not, the geometry tier missed it.
					if meshed[e.GlobalID] {
						if dq, ok := derived[e.GlobalID]; ok && !dq.IsEmpty() {
							t.Errorf("%s has a non-empty derived entry but stayed source=none — geometry tier missed it", e.GlobalID)
						}
					}
				default:
					t.Errorf("%s unknown quantity_source %q", e.GlobalID, e.QuantitySource)
				}
			}
			t.Logf("%s tiers: qto=%d geometry=%d none=%d (total=%d)", name, qto, geom, none, len(r.Elements))
			if geom == 0 {
				t.Errorf("no elements got the geometry tier — expected many (few real models ship full Qto)")
			}

			// Evidence (not a hard gate): where an element carries BOTH authored
			// Qto.Volume and a valid geometry-derived volume, the gross derived
			// value should be in the ballpark of the net authored one.
			var pairs, inBand int
			for i := range r.Elements {
				e := &r.Elements[i]
				if e.QuantitySource != model.QuantitySourceQto || e.Qto.Volume == nil {
					continue
				}
				dq, ok := derived[e.GlobalID]
				if !ok || dq.Volume == nil || *dq.Volume <= 0 {
					continue
				}
				pairs++
				ratio := *dq.Volume / *e.Qto.Volume
				if ratio >= 0.5 && ratio <= 5.0 {
					inBand++
				}
			}
			if pairs > 0 {
				t.Logf("%s: geometry-gross vs authored-net volume within 0.5x-5x on %d/%d elements", name, inBand, pairs)
			}
		})
	}
}

// explodeMesh rebuilds an indexed mesh so every triangle owns its own 3 fresh
// vertices at the same positions, with no shared indices across triangles —
// this is exactly the shape brepMesh emits (a fresh vertex block per IfcFace).
func explodeMesh(verts []float32, tris []uint32) (ev []float32, et []uint32) {
	for t := 0; t+2 < len(tris); t += 3 {
		for _, idx := range tris[t : t+3] {
			ev = append(ev, verts[3*idx], verts[3*idx+1], verts[3*idx+2])
			et = append(et, uint32(len(ev)/3-1))
		}
	}
	return ev, et
}

// isClosedManifold welds vertices by quantized position because brepMesh emits
// a fresh, unshared vertex block per face — raw-index adjacency alone would see
// every triangle as isolated and call any real mesh "open". This test builds a
// closed box with exactly that per-face-duplicated shape (no shared indices,
// same corner positions repeated across triangles) and checks welding
// reconnects it into a closed manifold. It then nudges one duplicated corner
// beyond the weld quantum (1e-5 m) and checks the same mesh is now correctly
// detected as open — proving the check is position-based, not index-based: an
// index-only adjacency check would have called the exploded box "open" even
// before the nudge (since it never shares indices), so it could never
// distinguish the nudged case from the sound one the way this test requires.
func TestIsClosedManifold_WeldsPerFaceDuplicatedVertices(t *testing.T) {
	verts, tris := boxMesh(v3{0, 0, 0}, v3{2, 3, 4})
	ev, et := explodeMesh(verts, tris)

	if !isClosedManifold(ev, et) {
		t.Fatal("exploded box (per-face-duplicated verts, no shared indices) must weld back into a closed manifold")
	}
	if v, ok := meshVolume(ev, et); !ok || math.Abs(v-24) > 1e-9 {
		t.Errorf("exploded box meshVolume = (%v, %v), want (24, true)", v, ok)
	}

	// Nudge one corner of one triangle by +0.01 in X (>> the 1e-5 m weld quantum):
	// that corner's other copies (shared by neighboring triangles pre-explosion)
	// no longer weld to it, opening a gap in the surface.
	nudged := append([]float32(nil), ev...)
	nudged[0] += 0.01
	if isClosedManifold(nudged, et) {
		t.Error("a >weld-quantum gap at one duplicated corner must break closedness, but isClosedManifold returned true")
	}
}

func deref(p *float64) float64 {
	if p == nil {
		return math.NaN()
	}
	return *p
}

// A meshed element whose world-AABB collapses on an axis must NOT emit a
// fabricated 0.0 for that axis — the #2159 phantom-zero, one tier down.
func TestDerivedQuantities_DegenerateMeshEmitsNoZero(t *testing.T) {
	// Flat element: nonzero X/Y extent, ZERO Z extent (Height must be nil).
	flat := Scene{Elements: []Element{{
		GlobalID:  "FLAT",
		Verts:     []float32{0, 0, 0, 2, 0, 0, 0, 3, 0}, // all z=0
		Tris:      []uint32{0, 1, 2},
		Placement: model.Identity(), // mesh-derived area lifts local verts to world
		BBoxMin:   [3]float64{0, 0, 0},
		BBoxMax:   [3]float64{2, 3, 0}, // dz = 0
	}}}
	q, ok := flat.DerivedQuantities()["FLAT"]
	if !ok {
		t.Fatal("flat element dropped entirely; want it present with Height=nil")
	}
	if q.Height != nil {
		t.Errorf("Height = %v, want nil (zero Z extent must not be a fabricated 0.0)", *q.Height)
	}
	if q.Length == nil || q.Width == nil || q.Area == nil {
		t.Error("nonzero horizontal extents must still be present")
	}

	// Fully-degenerate element (single point): every extent zero -> IsEmpty ->
	// must be OMITTED so ApplyDerivedQuantities leaves it source="none".
	point := Scene{Elements: []Element{{
		GlobalID: "POINT",
		Verts:    []float32{1, 1, 1, 1, 1, 1, 1, 1, 1},
		Tris:     []uint32{0, 1, 2},
		BBoxMin:  [3]float64{1, 1, 1},
		BBoxMax:  [3]float64{1, 1, 1}, // dx=dy=dz=0
	}}}
	if _, ok := point.DerivedQuantities()["POINT"]; ok {
		t.Error("fully-degenerate element must be omitted (stays source=none), got an entry")
	}
}

// An OBB-fallback mesh is the element's bounding box, not a real solid — its
// "volume" is a gross envelope (benchmarked up to ~70x over the true solid),
// so DerivedQuantities must NOT emit Volume for it. Dimensions still come from
// the (accurate) world-AABB.
func TestDerivedQuantities_OBBSourceHasNoVolume(t *testing.T) {
	verts, tris := boxMesh(v3{0, 0, 0}, v3{2, 3, 4}) // a closed box: meshVolume would accept it
	s := Scene{Elements: []Element{{
		GlobalID: "OBBOX",
		Verts:    verts,
		Tris:     tris,
		Source:   SourceOBB,
		BBoxMin:  [3]float64{0, 0, 0},
		BBoxMax:  [3]float64{2, 3, 4},
	}}}
	q := s.DerivedQuantities()["OBBOX"]
	if q.Volume != nil {
		t.Errorf("Volume = %v, want nil for an OBB-fallback (envelope) mesh", *q.Volume)
	}
	if q.Height == nil || q.Length == nil || q.Width == nil {
		t.Error("OBB element must still carry world-AABB dimensions")
	}
}

// A triangle indexing a vertex that doesn't exist is malformed geometry — the
// signed sum is meaningless, so meshVolume must reject it (ok=false) rather
// than silently substituting the origin and returning a corrupted volume.
//
// Starting from an otherwise-valid CLOSED box (so every other invariant —
// closedness, in-bbox volume — is satisfied) and corrupting a single index
// is deliberate: it isolates the bounds check as the only thing that can
// reject this mesh. Without the guard, this corrupted-but-closed-looking
// mesh would still sum to a nonzero, under-bbox volume and be silently
// accepted as ok=true — which is exactly the bug this test must catch.
func TestMeshVolume_OutOfRangeIndexRejected(t *testing.T) {
	verts, tris := boxMesh(v3{0, 0, 0}, v3{2, 3, 4})
	tris[len(tris)-1] = uint32(len(verts) / 3) // one past the last valid vertex index
	v, ok := meshVolume(verts, tris)
	if ok {
		t.Errorf("meshVolume accepted a mesh with an out-of-range vertex index, volume=%v", v)
	}
	if v != 0 {
		t.Errorf("meshVolume returned volume=%v on rejection, want 0", v)
	}
}

// meshVolume must ACCEPT a valid closed box and return its exact volume, and
// REJECT an open box (one face removed) — the divergence sum over a non-watertight
// surface is not a volume. boxMesh(...) is the existing closed-box helper.
func TestMeshVolume_WatertightGate(t *testing.T) {
	verts, tris := boxMesh(v3{0, 0, 0}, v3{2, 3, 4}) // closed, volume 24
	if v, ok := meshVolume(verts, tris); !ok || math.Abs(v-24) > 1e-9 {
		t.Errorf("closed box: got (%v, %v), want (24, true)", v, ok)
	}
	if !isClosedManifold(verts, tris) {
		t.Error("closed box should be a closed manifold")
	}
	// Drop the last triangle-pair (one quad face) → open box.
	openTris := tris[:len(tris)-6]
	if isClosedManifold(verts, openTris) {
		t.Error("open box (missing a face) must NOT be a closed manifold")
	}
	if _, ok := meshVolume(verts, openTris); ok {
		t.Error("meshVolume must reject an open box")
	}
}

// A closed 2-manifold that is INCONSISTENTLY wound (one triangle flipped) must
// also be rejected — the signed sum would be wrong. This is the exact defect
// Task 1 removes at the source; the gate is the backstop.
func TestMeshVolume_InconsistentWindingRejected(t *testing.T) {
	verts, tris := boxMesh(v3{0, 0, 0}, v3{2, 3, 4})
	tris[1], tris[2] = tris[2], tris[1] // flip one triangle's winding
	if isClosedManifold(verts, tris) {
		t.Error("a mesh with one flipped triangle must fail the winding check")
	}
	if _, ok := meshVolume(verts, tris); ok {
		t.Error("meshVolume must reject an inconsistently-wound mesh")
	}
}

// buildSceneOne parses, extracts, and builds a single-element synthetic fixture,
// returning both the Scene (for Scene-level queries like DerivedQuantities) and
// its sole Element. buildOne is the Element-only twin used where the Scene isn't
// needed.
func buildSceneOne(t *testing.T, path string) (*Scene, Element) {
	t.Helper()
	f, err := step.ParseFile(path)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	r, err := model.Extract(f)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	s, err := Build(f, r)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(s.Elements) != 1 {
		t.Fatalf("want 1 element, got %d", len(s.Elements))
	}
	return s, s.Elements[0]
}

// closeAbs asserts a *float64 equals a hand-verified literal within eps.
func closeAbs(t *testing.T, name string, got *float64, want, eps float64) {
	t.Helper()
	if got == nil {
		t.Errorf("%s = nil, want %g", name, want)
		return
	}
	if math.Abs(*got-want) > eps {
		t.Errorf("%s = %g, want %g", name, *got, want)
	}
}

// TestDerivedQuantities_KnownBox_Absolute is an ABSOLUTE golden (child 5, #2211).
// TestDerivedQuantities_KnownBox asserts the derived dims equal the element's own
// computed AABB — a consistency check that stays green even if the AABB is wrongly
// SCALED (both sides move together). This pins the derived dims to hand-verified
// LITERALS instead: known_box.ifc is exactly a 1x1x1 m box at world origin
// (10,20,5), so every horizontal dim is 1.0 m. The mm twin proves the world mesh
// is meters and derived quantities do not double-scale the millimeter input.
func TestDerivedQuantities_KnownBox_Absolute(t *testing.T) {
	for _, path := range []string{
		"testdata/synthetic/known_box.ifc",
		"testdata/synthetic/known_box_mm.ifc",
	} {
		t.Run(path, func(t *testing.T) {
			s, e := buildSceneOne(t, path)
			q, ok := s.DerivedQuantities()[e.GlobalID]
			if !ok {
				t.Fatalf("no derived quantities for %s", e.GlobalID)
			}
			// 1x1x1 m box: every world-AABB extent is exactly 1.0 m. (Area/perimeter
			// are mesh-derived now and known_box is a 2-face open shell, so its
			// mesh quantities aren't a clean golden — those are pinned on the
			// watertight closed_box in TestDerivedQuantities_ClosedBox_Absolute.)
			closeAbs(t, "Height", q.Height, 1.0, 1e-6)
			closeAbs(t, "Length", q.Length, 1.0, 1e-6)
			closeAbs(t, "Width", q.Width, 1.0, 1e-6)
		})
	}
}

// TestDerivedQuantities_ClosedBox_Absolute is the ABSOLUTE mesh-quantity golden
// (child 5 #2211 + #2213). closed_box.ifc is a watertight 6-face unit cube at
// world (10,20,5), so ALL mesh-derived tier-2 quantities are hand-verifiable and
// must match the ifcopenshell definitions exactly, independent of the Python
// oracle: gross mesh volume 1.0 m^3 (divergence-theorem sum), max-side-area 1.0 m^2
// (each unit face), footprint perimeter 4.0 m (the bottom 1x1 square outline).
func TestDerivedQuantities_ClosedBox_Absolute(t *testing.T) {
	s, e := buildSceneOne(t, "testdata/synthetic/closed_box.ifc")
	if e.Source == SourceOBB {
		t.Fatalf("closed_box fell back to OBB (source=%v); expected a tessellated brep", e.Source)
	}
	q, ok := s.DerivedQuantities()[e.GlobalID]
	if !ok {
		t.Fatalf("no derived quantities for %s", e.GlobalID)
	}
	closeAbs(t, "Volume", q.Volume, 1.0, 1e-6)
	closeAbs(t, "Area", q.Area, 1.0, 1e-6)
	closeAbs(t, "Perimeter", q.Perimeter, 4.0, 1e-6)
}
