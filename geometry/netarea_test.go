package geometry

import (
	"strings"
	"testing"

	"github.com/blox-eng/common/ifc/model"
	"github.com/blox-eng/common/ifc/step"
)

// buildNetAreas parses, extracts, builds, and reconciles a synthetic fixture,
// returning the Scene (for Warnings) and the per-host NetArea map.
func buildNetAreas(t *testing.T, path string) (*Scene, map[string]NetArea) {
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
	return s, s.NetAreas(f, r)
}

// onlyNet returns the single host reconciliation in m (each fixture has exactly
// one voided host).
func onlyNet(t *testing.T, m map[string]NetArea) NetArea {
	t.Helper()
	if len(m) != 1 {
		t.Fatalf("want exactly 1 voided host in map, got %d: %v", len(m), m)
	}
	for _, na := range m {
		return na
	}
	return NetArea{}
}

// TestNetAreas_RectWindow: a wall with one rectangular window. Net = gross − w·h,
// Trusted, and — the un-voided invariant — Gross equals the FULL solid wall
// area (the window did not already reduce the host mesh).
func TestNetAreas_RectWindow(t *testing.T) {
	_, m := buildNetAreas(t, "testdata/synthetic/netarea_rect_window.ifc")
	na := onlyNet(t, m)
	if !na.Trusted {
		t.Fatalf("want Trusted, got reason %q", na.Reason)
	}
	closeAbs(t, "Gross", &na.Gross, 12.0, 1e-6) // un-voided invariant: full 4x3 face
	closeAbs(t, "OpeningDeduction", &na.OpeningDeduction, 1.0, 1e-6)
	closeAbs(t, "Net", na.Net, 11.0, 1e-6)
}

// TestNetAreas_ArchWindow: a polygonal (arched) void is deducted by its
// axis-aligned bounding span (1.0), NOT its true curved area (~0.85) — assert
// with tolerance, not an exact curve area.
func TestNetAreas_ArchWindow(t *testing.T) {
	_, m := buildNetAreas(t, "testdata/synthetic/netarea_arch_window.ifc")
	na := onlyNet(t, m)
	if !na.Trusted {
		t.Fatalf("want Trusted, got reason %q", na.Reason)
	}
	closeAbs(t, "Gross", &na.Gross, 12.0, 1e-6)
	closeAbs(t, "OpeningDeduction", &na.OpeningDeduction, 1.0, 1e-3) // bounding span, not 0.85
	closeAbs(t, "Net", na.Net, 11.0, 1e-3)
}

// TestNetAreas_OBBOpening: an opening whose geometry resolves to the OBB
// fallback (no real solid) makes the whole host untrusted.
func TestNetAreas_OBBOpening(t *testing.T) {
	_, m := buildNetAreas(t, "testdata/synthetic/netarea_obb_opening.ifc")
	na := onlyNet(t, m)
	assertUntrusted(t, na, "solid geometry")
}

// TestNetAreas_Overlap: two openings whose bounding rectangles intersect
// (Σ well under 95%) — the deduction would be double-counted, so untrusted.
// This is the review's blind spot.
func TestNetAreas_Overlap(t *testing.T) {
	_, m := buildNetAreas(t, "testdata/synthetic/netarea_overlap.ifc")
	na := onlyNet(t, m)
	assertUntrusted(t, na, "overlap")
}

// TestNetAreas_MostlyGlass: two non-overlapping bands whose Σ footprint is
// ≥95% of gross — over-subtraction gate, untrusted.
func TestNetAreas_MostlyGlass(t *testing.T) {
	_, m := buildNetAreas(t, "testdata/synthetic/netarea_mostly_glass.ifc")
	na := onlyNet(t, m)
	assertUntrusted(t, na, "95%")
}

// TestNetAreas_OrphanFill: a wall with NO voids is absent from the map; a
// filled-but-void-less opening yields exactly ONE aggregated Scene warning.
func TestNetAreas_OrphanFill(t *testing.T) {
	s, m := buildNetAreas(t, "testdata/synthetic/netarea_orphan_fill.ifc")
	if len(m) != 0 {
		t.Fatalf("wall has no voids; want empty map, got %v", m)
	}
	const want = "openings fill a void but void no host"
	n := 0
	for _, w := range s.Warnings {
		if strings.Contains(w, want) {
			n++
			if !strings.HasPrefix(w, "1 ") {
				t.Errorf("warning %q: want count 1", w)
			}
		}
	}
	if n != 1 {
		t.Fatalf("want exactly 1 aggregated orphan warning, got %d in %v", n, s.Warnings)
	}
}

// TestNetAreas_SlabHole: host scope is ANY voided element, not walls only — a
// slab with a hole is reconciled.
func TestNetAreas_SlabHole(t *testing.T) {
	_, m := buildNetAreas(t, "testdata/synthetic/netarea_slab_hole.ifc")
	na := onlyNet(t, m)
	if !na.Trusted {
		t.Fatalf("want Trusted, got reason %q", na.Reason)
	}
	closeAbs(t, "Gross", &na.Gross, 12.0, 1e-6)
	closeAbs(t, "Net", na.Net, 11.0, 1e-6)
}

// TestNetAreas_Oblique: a wall oriented along Y — its elevational face normal is
// axis 0 (YZ plane), a DIFFERENT winning axis than the axis-2 fixtures. Proves
// the host's winning axis is computed and the openings are measured on that same
// axis (not a hardcoded one). Net = gross − w·h exactly.
func TestNetAreas_Oblique(t *testing.T) {
	_, m := buildNetAreas(t, "testdata/synthetic/netarea_oblique.ifc")
	na := onlyNet(t, m)
	if !na.Trusted {
		t.Fatalf("want Trusted, got reason %q", na.Reason)
	}
	closeAbs(t, "Gross", &na.Gross, 12.0, 1e-6)
	closeAbs(t, "OpeningDeduction", &na.OpeningDeduction, 1.0, 1e-6)
	closeAbs(t, "Net", na.Net, 11.0, 1e-6)
}

// TestNetAreas_MillimetreOverlap: a MILLIMETRE file whose two openings overlap
// only when the opening LocalPlacement translation is meter-scaled. Without the
// scale the openings land ~300 m apart, the overlap is missed, and the host is
// wrongly trusted — so asserting untrusted-overlap is what catches the 1000×
// placement bug. Gross==12 also proves the mesh scales mm→m (no 1000× error).
func TestNetAreas_MillimetreOverlap(t *testing.T) {
	_, m := buildNetAreas(t, "testdata/synthetic/netarea_mm_overlap.ifc")
	na := onlyNet(t, m)
	closeAbs(t, "Gross", &na.Gross, 12.0, 1e-6)
	assertUntrusted(t, na, "overlap")
}

// TestNetAreas_SingleOversize: one opening larger than the wall trips the ≥95%
// gate through a SINGLE opening (overlap cannot apply with one opening).
func TestNetAreas_SingleOversize(t *testing.T) {
	_, m := buildNetAreas(t, "testdata/synthetic/netarea_single_oversize.ifc")
	na := onlyNet(t, m)
	assertUntrusted(t, na, "95%")
}

// assertUntrusted checks the all-or-nothing untrusted contract: Net nil,
// OpeningDeduction 0, Gross still present, and a reason containing want.
func assertUntrusted(t *testing.T, na NetArea, want string) {
	t.Helper()
	if na.Trusted {
		t.Fatalf("want untrusted, got Trusted with Net=%v", deref(na.Net))
	}
	if na.Net != nil {
		t.Errorf("Net = %v, want nil when untrusted", *na.Net)
	}
	if na.OpeningDeduction != 0 {
		t.Errorf("OpeningDeduction = %v, want 0 when untrusted", na.OpeningDeduction)
	}
	if na.Gross <= 0 {
		t.Errorf("Gross = %v, want a positive gross even when untrusted", na.Gross)
	}
	if !strings.Contains(na.Reason, want) {
		t.Errorf("Reason = %q, want it to contain %q", na.Reason, want)
	}
}
