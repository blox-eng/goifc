package step

import "testing"

func TestInstanceAccessors(t *testing.T) {
	f, _ := ParseBytes(readTestdata(t, "refs.ifc"))

	ctx, _ := f.ByID(2)
	if !ctx.IsA("IfcGeometricRepresentationContext") {
		t.Fatal("IsA exact (case-insensitive) failed")
	}
	if ctx.IsA("IfcElement") {
		t.Fatal("IsA must be exact-only (no supertype) in the schema-agnostic layer")
	}

	place, _ := f.ByID(4)
	pt, ok := place.Ref(0)
	if !ok || pt.ID() != 6 {
		t.Fatalf("Ref(0) = %v %v", pt, ok)
	}

	// Ref on a non-ref attribute is (nil,false)
	if _, ok := place.Ref(1); ok {
		t.Fatal("Ref on $ attribute should be false")
	}
	// Ref on a dangling ref is (nil,false)
	w, _ := f.ByID(8)
	if _, ok := w.Ref(7); ok {
		t.Fatal("Ref on dangling #999 should be false")
	}
}
