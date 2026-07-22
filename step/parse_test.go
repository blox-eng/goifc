package step

import "testing"

func TestParseBytes_Minimal(t *testing.T) {
	f, err := ParseBytes(readTestdata(t, "minimal.ifc"))
	if err != nil {
		t.Fatal(err)
	}
	if f.SchemaID() != "IFC2X3" {
		t.Fatalf("schema %q", f.SchemaID())
	}
	if f.Len() != 3 {
		t.Fatalf("instances %d want 3", f.Len())
	}
	w, ok := f.ByID(2)
	if !ok || w.Type() != "IFCWALL" {
		t.Fatalf("byID(2) = %v %v", w, ok)
	}
	if w.Len() != 8 {
		t.Fatalf("wall args %d want 8", w.Len())
	}
	a0, _ := w.Get(0)
	if a0.Kind != KindString || a0.Str != "0abc" {
		t.Fatalf("wall arg0 %+v", a0)
	}
	a5, _ := w.Get(5)
	if a5.Kind != KindRef || a5.RefID != 3 {
		t.Fatalf("wall arg5 (placement ref) %+v", a5)
	}
	unit, _ := f.ByID(1)
	u0, _ := unit.Get(0)
	if u0.Kind != KindDerived {
		t.Fatalf("unit arg0 %+v want derived", u0)
	}
	if len(f.ByType("ifcwall")) != 1 { // case-insensitive
		t.Fatalf("byType ifcwall = %d", len(f.ByType("ifcwall")))
	}
	cnt := 0
	for range f.All() {
		cnt++
	}
	if cnt != 3 {
		t.Fatalf("All() yielded %d want 3", cnt)
	}
	// header fields captured
	if len(f.Head.Name) < 6 || f.Head.Name[5] != "ARCHICAD" {
		t.Fatalf("FILE_NAME not captured: %+v", f.Head.Name)
	}
}
