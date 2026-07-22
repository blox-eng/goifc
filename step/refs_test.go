package step

import "testing"

func TestResolveAndInverse(t *testing.T) {
	f, err := ParseBytes(readTestdata(t, "refs.ifc"))
	if err != nil {
		t.Fatal(err)
	}

	// forward: #4.arg0 resolves to #6
	p, _ := f.ByID(4)
	a0, _ := p.Get(0)
	if a0.Kind != KindRef || a0.Ref == nil || a0.Ref.ID() != 6 {
		t.Fatalf("fwd ref not resolved: %+v", a0)
	}

	// forward inside a list: #1.arg7 == (#2)
	proj, _ := f.ByID(1)
	a7, _ := proj.Get(7)
	if a7.Kind != KindList || len(a7.List) == 0 || a7.List[0].Ref == nil || a7.List[0].Ref.ID() != 2 {
		t.Fatalf("list ref not resolved: %+v", a7)
	}

	// inverse: #2 is referenced by #1 (attr7) and #7 (attr5)
	two, _ := f.ByID(2)
	if got := f.TotalInverses(two); got != 2 {
		t.Fatalf("total inverses of #2 = %d want 2", got)
	}
	ids := map[int]bool{}
	for _, r := range f.Inverse(two) {
		ids[r.ID()] = true
	}
	if !ids[1] || !ids[7] {
		t.Fatalf("inverse referrers of #2 = %v want {1,7}", ids)
	}

	// inverse indices carry the top-level attr index
	foundAttr7 := false
	for _, ir := range f.InverseIndices(two) {
		if ir.From.ID() == 1 && ir.AttrIndex == 7 {
			foundAttr7 = true
		}
	}
	if !foundAttr7 {
		t.Fatalf("expected referrer #1 at attr 7: %+v", f.InverseIndices(two))
	}

	// missing target #999 -> unresolved + non-fatal warning
	w, _ := f.ByID(8)
	a7w, _ := w.Get(7)
	if a7w.Kind != KindRef || a7w.Ref != nil {
		t.Fatalf("missing ref should stay nil: %+v", a7w)
	}
	if len(f.Warnings()) == 0 {
		t.Fatal("expected a warning for #999")
	}

	// traverse: forward closure from #1 reaches #2 and #6 (#1->#2->#4->#6)
	seen := map[int]bool{}
	for _, inst := range f.Traverse(proj, Unbounded, DepthFirst) {
		seen[inst.ID()] = true
	}
	if !seen[2] || !seen[6] {
		t.Fatalf("traverse from #1 missing #2/#6: %v", seen)
	}
}
