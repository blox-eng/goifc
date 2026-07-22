package geometry

import (
	"bytes"
	"os"
	"testing"

	"github.com/blox-eng/common/ifc/model"
	"github.com/blox-eng/common/ifc/step"
)

func TestCorpus_KB645_RendersEveryElement(t *testing.T) {
	const path = "testdata/real/kb645.ifc"
	if _, err := os.Stat(path); err != nil {
		t.Skip("kb645 absent")
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
	st := s.Stats()
	t.Logf("kb645 stats: %+v warnings=%d", st, len(s.Warnings))
	// Empty = elements with no parseable representation. The oracle (OCCT) also
	// renders nothing for these, so they're genuinely geometry-less (Total-Empty
	// == the 1878 oracle count on kb645). Allow up to 5%; a spike means a
	// representation-nav regression dropping real geometry.
	if st.Empty*20 > st.Total {
		t.Errorf("%d/%d elements produced no geometry (>5%%)", st.Empty, st.Total)
	}
	// Per-path FLOORS — the oracle AABB gate cannot tell a tessellated brep from
	// its bounding box (OBB == brep AABB), so an "everything silently boxed"
	// regression passes parity. These floors are the only automated defense of the
	// moat claim (breps/extrusions tessellated, not boxed). Counts are ELEMENT-level
	// (an element may own several representation items); kb645 Build yields
	// Total=1922, Brep=1138, Extrude=737, OBB=47. Floors sit ~20-30% below actual —
	// robust to model/version drift, but a path fully regressing to OBB fails hard.
	if st.Brep < 900 {
		t.Errorf("brep path = %d/%d, want >900 (breps silently boxed?)", st.Brep, st.Total)
	}
	if st.Extrude < 500 {
		t.Errorf("extrude path = %d/%d, want >500 (extrusions silently boxed?)", st.Extrude, st.Total)
	}
	// Tessellation must dominate; OBB fallback stays a small minority.
	if (st.Brep+st.Extrude)*100 < st.Total*85 {
		t.Errorf("tessellated (brep+extrude) = %d/%d (<85%%) — too many boxed", st.Brep+st.Extrude, st.Total)
	}
	if st.OBB*100 > st.Total*15 {
		t.Errorf("OBB fallback = %d/%d (>15%%) — too many elements boxed", st.OBB, st.Total)
	}
	var buf bytes.Buffer
	if err := s.WriteGLB(&buf); err != nil {
		t.Fatalf("WriteGLB: %v", err)
	}
	if buf.Len() < 1000 {
		t.Errorf("GLB suspiciously small: %d bytes", buf.Len())
	}
}
