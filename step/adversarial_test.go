package step

import "testing"

// P2: a leading-dot real (.5) must not be silently swallowed as an enum.
func TestScanner_LeadingDotReal(t *testing.T) {
	f, err := ParseBytes([]byte("ISO-10303-21;\nDATA;\n#1= IFCCARTESIANPOINT((.5,0.,-.25));\nENDSEC;\nEND-ISO-10303-21;"))
	if err != nil {
		t.Fatal(err)
	}
	p, _ := f.ByID(1)
	a0, _ := p.Get(0)
	if a0.Kind != KindList || len(a0.List) != 3 {
		t.Fatalf("coords mis-parsed (leading-dot swallowed?): %+v", a0)
	}
	if a0.List[0].Kind != KindFloat || a0.List[0].F != 0.5 {
		t.Fatalf("arg .5 = %+v want float 0.5", a0.List[0])
	}
	if a0.List[2].Kind != KindFloat || a0.List[2].F != -0.25 {
		t.Fatalf("arg -.25 = %+v want float -0.25", a0.List[2])
	}
}

// P2: an ISO-10303-21 complex instance must not fatal the whole file.
func TestParse_ComplexInstance(t *testing.T) {
	src := "ISO-10303-21;\nDATA;\n" +
		"#1= IFCCARTESIANPOINT((0.,0.,0.));\n" +
		"#2= (IFCLENGTHMEASURE(1.)IFCLABEL('x'));\n" +
		"#3= IFCDIRECTION((1.,0.,0.));\n" +
		"ENDSEC;\nEND-ISO-10303-21;"
	f, err := ParseBytes([]byte(src))
	if err != nil {
		t.Fatalf("complex instance must not be fatal: %v", err)
	}
	if f.Len() != 3 {
		t.Fatalf("instances %d want 3 (complex #2 dropped?)", f.Len())
	}
	c, ok := f.ByID(2)
	if !ok {
		t.Fatal("#2 (complex) not in graph")
	}
	// both part types are queryable, and IsA matches either part
	if !c.IsA("IFCLENGTHMEASURE") || !c.IsA("IFCLABEL") {
		t.Fatalf("complex #2 IsA parts failed: type=%q", c.Type())
	}
	if len(f.ByType("IFCLABEL")) != 1 {
		t.Fatalf("ByType(IFCLABEL) = %d want 1", len(f.ByType("IFCLABEL")))
	}
	// surrounding simple instances still parse
	if _, ok := f.ByID(3); !ok {
		t.Fatal("#3 after complex not parsed")
	}
}

// P3: bounded Traverse must include a node reachable within maxLevels even when a
// longer path to it is discovered first.
func TestTraverse_BoundedDepthShortestWins(t *testing.T) {
	// #1 -> #2 (list) where #1 also -> #4 -> #3 -> #2 ; #1 -> #2 direct at depth 1.
	src := "ISO-10303-21;\nDATA;\n" +
		"#1= IFCX(#4,#2);\n" + // arg0 -> #4 (long path), arg1 -> #2 (depth 1)
		"#4= IFCX(#3,$);\n" +
		"#3= IFCX(#2,$);\n" +
		"#2= IFCX($,$);\n" +
		"ENDSEC;\nEND-ISO-10303-21;"
	f, err := ParseBytes([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	one, _ := f.ByID(1)
	seen := map[int]bool{}
	for _, inst := range f.Traverse(one, 1, DepthFirst) { // depth <=1 from #1
		seen[inst.ID()] = true
	}
	if !seen[2] {
		t.Fatalf("bounded DFS dropped #2 reachable at depth 1: %v", seen)
	}
	if seen[3] {
		t.Fatalf("depth-1 traverse wrongly included #3 (depth 2): %v", seen)
	}
}

// P3: FILE_NAME positional fields stay stable when author/org sub-lists are not
// single-element.
func TestHeader_FileNamePositionsStable(t *testing.T) {
	src := "ISO-10303-21;\nHEADER;\n" +
		"FILE_DESCRIPTION((''),'2;1');\n" +
		"FILE_NAME('n','ts',('a1','a2'),('o1','o2','o3'),'pre','ARCHICAD','auth');\n" +
		"FILE_SCHEMA(('IFC2X3'));\nENDSEC;\nDATA;\nENDSEC;\nEND-ISO-10303-21;"
	f, err := ParseBytes([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	// 7 positional FILE_NAME fields; index 5 = originating_system regardless of
	// how many authors/orgs there are.
	if len(f.Head.Name) != 7 {
		t.Fatalf("FILE_NAME fields = %d want 7 (%v)", len(f.Head.Name), f.Head.Name)
	}
	if f.Head.Name[5] != "ARCHICAD" {
		t.Fatalf("FILE_NAME[5] = %q want ARCHICAD (positions shifted by multi-author list)", f.Head.Name[5])
	}
}
