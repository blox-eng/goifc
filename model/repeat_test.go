package model

import (
	"fmt"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/goifc/step"
)

// membershipTime bounds one lookup per member of a relation with
// membershipCount members. Scanning the relation's list for each member costs
// membershipCount² comparisons, 10 s or more at this size; a lookup by
// attribute takes about 0.3 s for all of them. The margin covers a slow or
// instrumented CI runner.
const (
	membershipCount = 50_000
	membershipTime  = 3 * time.Second
)

// relationFile is membershipCount walls named by one relation, built by rel
// from the list of wall references.
func relationFile(t *testing.T, rel func(walls string) string) (*step.File, []*step.Instance) {
	t.Helper()
	var b strings.Builder
	b.WriteString("ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n")
	b.WriteString("#1=IFCBUILDINGSTOREY('s',$,'Storey',$,$,$,$,$,.ELEMENT.,0.);\n")
	b.WriteString("#2=IFCWALLTYPE('t',$,'Type',$,$,$,$,$,$,.STANDARD.);\n")
	b.WriteString("#3=IFCELEMENTASSEMBLY('a',$,'Assembly',$,$,$,$,$,$,$);\n")
	refs := make([]string, membershipCount)
	for i := range membershipCount {
		fmt.Fprintf(&b, "#%d=IFCWALL('w%d',$,'W',$,$,$,$,$,$);\n", 100+i, i)
		refs[i] = fmt.Sprintf("#%d", 100+i)
	}
	b.WriteString(rel(strings.Join(refs, ",")))
	b.WriteString("ENDSEC;\nEND-ISO-10303-21;\n")
	f := parseString(t, b.String())
	return f, f.ByType("IfcWall")
}

func TestRelationMembershipIsNotQuadratic(t *testing.T) {
	cases := []struct {
		name   string
		rel    string
		lookup func(*step.File, *step.Instance) *step.Instance
		want   int
	}{
		{"contained", "#9=IFCRELCONTAINEDINSPATIALSTRUCTURE('r',$,$,$,(%s),#1);\n",
			func(f *step.File, w *step.Instance) *step.Instance { return containerOf(f, w, 32) }, 1},
		{"aggregated", "#9=IFCRELAGGREGATES('r',$,$,$,#3,(%s));\n", getAggregate, 3},
		{"nested", "#9=IFCRELNESTS('r',$,$,$,#3,(%s));\n", getNest, 3},
		{"typed", "#9=IFCRELDEFINESBYTYPE('r',$,$,$,(%s),#2);\n", GetType, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, walls := relationFile(t, func(w string) string { return fmt.Sprintf(c.rel, w) })
			start := time.Now()
			for _, w := range walls {
				if got := c.lookup(f, w); got == nil || got.ID() != c.want {
					t.Fatalf("wall #%d: got %v, want #%d", w.ID(), got, c.want)
				}
			}
			if d := time.Since(start); d > membershipTime {
				t.Fatalf("%d lookups took %v, want under %v: membership scans the relation's list", len(walls), d, membershipTime)
			}
		})
	}
}

// A relation names the wall at another attribute than its related list: the
// relating side. The wall is not a member, so it has no parent there.
func TestTheRelatingSideIsNotAMember(t *testing.T) {
	f := parseString(t, "ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n"+
		"#1=IFCWALL('a',$,'A',$,$,$,$,$,$);\n#2=IFCWALL('b',$,'B',$,$,$,$,$,$);\n"+
		"#3=IFCRELAGGREGATES('r',$,$,$,#1,(#2));\n#4=IFCRELNESTS('n',$,$,$,#1,(#2));\n"+
		"#5=IFCRELDEFINESBYTYPE('t',$,$,$,(#2),#1);\n"+
		"ENDSEC;\nEND-ISO-10303-21;\n")
	a, _ := f.ByID(1)
	if p := getAggregate(f, a); p != nil {
		t.Errorf("getAggregate(#1) = #%d, want nil: #1 is the relating object", p.ID())
	}
	if p := getNest(f, a); p != nil {
		t.Errorf("getNest(#1) = #%d, want nil: #1 is the relating object", p.ID())
	}
	if p := GetType(f, a); p != nil {
		t.Errorf("GetType(#1) = #%d, want nil: #1 is the relating type", p.ID())
	}
}

// A placement point read at every element that uses it must not cost its
// coordinate list each time: no schema allows more than three.
func TestAPlacementPointWithTooManyCoordinatesIsNotRead(t *testing.T) {
	const walls, coordinates = 500, 1_000_000
	var b strings.Builder
	b.WriteString("ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n")
	b.WriteString("#1=IFCCARTESIANPOINT((" + strings.Repeat("7.,", coordinates-1) + "7.));\n")
	b.WriteString("#2=IFCAXIS2PLACEMENT3D(#1,$,$);\n#3=IFCLOCALPLACEMENT($,#2);\n")
	for i := range walls {
		fmt.Fprintf(&b, "#%d=IFCWALL('w%d',$,'W',$,$,#3,$,$,$);\n", 100+i, i)
	}
	b.WriteString("ENDSEC;\nEND-ISO-10303-21;\n")
	f := parseString(t, b.String())

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	placed := make([]Mat4, 0, walls)
	for _, w := range f.ByType("IfcWall") {
		placed = append(placed, LocalPlacement(w))
	}
	runtime.ReadMemStats(&after)
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 16<<20 {
		t.Fatalf("placing %d walls allocated %d MiB, want under 16: the coordinates were read per wall", walls, alloc>>20)
	}
	for _, m := range placed {
		if x, y, z := m.Translation(); x != 0 || y != 0 || z != 0 {
			t.Fatalf("placed at (%v, %v, %v), want the origin: the point is not a point", x, y, z)
		}
	}
}

// hubCount walls each aggregate into one assembly through a relation of their
// own, and the assembly's containment comes last among its references. Finding
// each wall's storey climbs to the assembly; reading the assembly's references
// again for each wall costs hubCount² comparisons.
const (
	hubCount = 80_000
	hubTime  = 3 * time.Second
)

func TestAHubOfRelationsIsNotQuadratic(t *testing.T) {
	var b strings.Builder
	b.WriteString("ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n")
	b.WriteString("#1=IFCBUILDINGSTOREY('s',$,'Storey',$,$,$,$,$,.ELEMENT.,0.);\n")
	b.WriteString("#3=IFCELEMENTASSEMBLY('a',$,'Assembly',$,$,$,$,$,$,$);\n")
	for i := range hubCount {
		fmt.Fprintf(&b, "#%d=IFCWALL('w%d',$,'W',$,$,$,$,$,$);\n", 100+2*i, i)
		fmt.Fprintf(&b, "#%d=IFCRELAGGREGATES('r%d',$,$,$,#3,(#%d));\n", 101+2*i, i, 100+2*i)
	}
	b.WriteString("#9=IFCRELCONTAINEDINSPATIALSTRUCTURE('c',$,$,$,(#3),#1);\n")
	b.WriteString("ENDSEC;\nEND-ISO-10303-21;\n")
	f := parseString(t, b.String())

	start := time.Now()
	res, err := Extract(f)
	if err != nil {
		t.Fatal(err)
	}
	d := time.Since(start)
	walls := 0
	for _, e := range res.Elements {
		if !strings.EqualFold(e.IFCClass, "IfcWall") {
			continue
		}
		walls++
		if e.Storey != "Storey" {
			t.Fatalf("wall #%d: storey %q, want %q", e.ExpressID, e.Storey, "Storey")
		}
	}
	if walls != hubCount {
		t.Fatalf("got %d walls, want %d", walls, hubCount)
	}
	if d > hubTime {
		t.Fatalf("extracting %d walls took %v, want under %v: each wall reads the assembly's references again", hubCount, d, hubTime)
	}
}

// A related side that is one reference, not a list, is malformed. It still
// names the member, and the inverse index records it at that attribute.
func TestABareReferenceOnTheRelatedSideIsAMember(t *testing.T) {
	f := parseString(t, "ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n"+
		"#1=IFCELEMENTASSEMBLY('a',$,'A',$,$,$,$,$,$,$);\n#2=IFCWALL('b',$,'B',$,$,$,$,$,$);\n"+
		"#3=IFCRELAGGREGATES('r',$,$,$,#1,#2);\n"+
		"ENDSEC;\nEND-ISO-10303-21;\n")
	w, _ := f.ByID(2)
	if p := getAggregate(f, w); p == nil || p.ID() != 1 {
		t.Fatalf("getAggregate(#2) = %v, want #1", p)
	}
}

// A location with an unset coordinate is not a point. Dropping the unset entry
// would shift the others: (1,$,5) would place at (1,5,0).
func TestALocationWithAnUnsetCoordinateIsNotShifted(t *testing.T) {
	f := parseString(t, "ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n"+
		"#1=IFCCARTESIANPOINT((1.,$,5.));\n#2=IFCAXIS2PLACEMENT3D(#1,$,$);\n#3=IFCLOCALPLACEMENT($,#2);\n"+
		"#4=IFCWALL('w',$,'W',$,$,#3,$,$,$);\n"+
		"ENDSEC;\nEND-ISO-10303-21;\n")
	w, _ := f.ByID(4)
	m := LocalPlacement(w)
	if x, y, z := m.Translation(); x != 0 || y != 0 || z != 0 {
		t.Fatalf("placed at (%v, %v, %v), want the origin: an unset coordinate shifted the others", x, y, z)
	}
}

// sharedSetCount elements share one set of sharedSetCount properties. Copying
// the set into each element costs sharedSetCount² entries, gigabytes at this
// size; reading it once and sharing it costs one set.
const (
	sharedSetCount = 2_000
	sharedSetAlloc = 256 << 20
)

// TestASharedPropertySetIsReadOnce covers the three ways elements share a
// set: one relation naming every element, a type's property sets, and a
// type's quantity sets.
func TestASharedPropertySetIsReadOnce(t *testing.T) {
	props := func(b *strings.Builder, first int, entity string) string {
		refs := make([]string, sharedSetCount)
		for i := range sharedSetCount {
			id := first + i
			refs[i] = fmt.Sprintf("#%d", id)
			if entity == "IFCQUANTITYLENGTH" {
				kind, name := "LENGTH", fmt.Sprintf("Q%d", i)
				if i == 0 {
					kind, name = "AREA", "NetSideArea" // the quantity tier needs an area
				}
				fmt.Fprintf(b, "#%d=IFCQUANTITY%s('%s',$,$,%d.,$);\n", id, kind, name, i)
			} else {
				fmt.Fprintf(b, "#%d=IFCPROPERTYSINGLEVALUE('P%d',$,IFCREAL(%d.),$);\n", id, i, i)
			}
		}
		return strings.Join(refs, ",")
	}
	cases := []struct {
		name  string
		build func(b *strings.Builder, walls string)
		check func(e Element) error
	}{
		{"a relation names every element", func(b *strings.Builder, walls string) {
			fmt.Fprintf(b, "#2=IFCPROPERTYSET('p',$,'Pset_Shared',$,(%s));\n", props(b, 10, ""))
			fmt.Fprintf(b, "#3=IFCRELDEFINESBYPROPERTIES('r',$,$,$,(%s),#2);\n", walls)
		}, func(e Element) error {
			if n := len(e.Psets["Pset_Shared"]); n != sharedSetCount {
				return fmt.Errorf("Pset_Shared has %d properties, want %d", n, sharedSetCount)
			}
			return nil
		}},
		{"a type carries the set", func(b *strings.Builder, walls string) {
			fmt.Fprintf(b, "#2=IFCPROPERTYSET('p',$,'Pset_Shared',$,(%s));\n", props(b, 10, ""))
			b.WriteString("#4=IFCWALLTYPE('t',$,'T',$,$,(#2),$,$,$,.STANDARD.);\n")
			fmt.Fprintf(b, "#3=IFCRELDEFINESBYTYPE('r',$,$,$,(%s),#4);\n", walls)
		}, func(e Element) error {
			if n := len(e.Psets["Pset_Shared"]); n != sharedSetCount {
				return fmt.Errorf("Pset_Shared has %d properties, want %d", n, sharedSetCount)
			}
			return nil
		}},
		{"a type carries the quantities", func(b *strings.Builder, walls string) {
			fmt.Fprintf(b, "#2=IFCELEMENTQUANTITY('q',$,'Qto_Shared',$,$,(%s));\n", props(b, 10, "IFCQUANTITYLENGTH"))
			b.WriteString("#4=IFCWALLTYPE('t',$,'T',$,$,(#2),$,$,$,.STANDARD.);\n")
			fmt.Fprintf(b, "#3=IFCRELDEFINESBYTYPE('r',$,$,$,(%s),#4);\n", walls)
		}, func(e Element) error {
			if e.Qto.Area == nil || *e.Qto.Area != 0 {
				return fmt.Errorf("Qto.Area = %v, want 0", e.Qto.Area)
			}
			return nil
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var b strings.Builder
			b.WriteString("ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n")
			refs := make([]string, sharedSetCount)
			for i := range sharedSetCount {
				id := 100_000 + i
				refs[i] = fmt.Sprintf("#%d", id)
				fmt.Fprintf(&b, "#%d=IFCWALL('w%d',$,'W',$,$,$,$,$,$);\n", id, i)
			}
			c.build(&b, strings.Join(refs, ","))
			b.WriteString("ENDSEC;\nEND-ISO-10303-21;\n")
			f := parseString(t, b.String())

			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			res, err := Extract(f)
			runtime.ReadMemStats(&after)
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Elements) != sharedSetCount {
				t.Fatalf("got %d elements, want %d", len(res.Elements), sharedSetCount)
			}
			for _, e := range res.Elements {
				if err := c.check(e); err != nil {
					t.Fatalf("#%d: %v", e.ExpressID, err)
				}
			}
			if alloc := after.TotalAlloc - before.TotalAlloc; alloc > sharedSetAlloc {
				t.Fatalf("extracting %d elements allocated %d MiB, want under %d: the shared set is copied per element", sharedSetCount, alloc>>20, sharedSetAlloc>>20)
			}
		})
	}
}

// TestATypesSetsAreNotCopiedWithoutBound covers the case sharing cannot: each
// element carries a set of its own, so each needs its own map, and each map
// holds every set its type carries. sharedSetCount elements of a type with
// sharedSetCount sets cost sharedSetCount² entries. Past a budget that grows
// with the file, an element's sets are left out, and Extract says so.
func TestATypesSetsAreNotCopiedWithoutBound(t *testing.T) {
	var b strings.Builder
	b.WriteString("ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n")
	b.WriteString("#10=IFCPROPERTYSINGLEVALUE('P',$,IFCREAL(1.),$);\n")
	var sets, walls []string
	for i := range sharedSetCount {
		fmt.Fprintf(&b, "#%d=IFCPROPERTYSET('t%d',$,'Type%d',$,(#10));\n", 10_000+i, i, i)
		sets = append(sets, fmt.Sprintf("#%d", 10_000+i))
		fmt.Fprintf(&b, "#%d=IFCWALL('w%d',$,'W',$,$,$,$,$,$);\n", 100_000+i, i)
		walls = append(walls, fmt.Sprintf("#%d", 100_000+i))
		fmt.Fprintf(&b, "#%d=IFCPROPERTYSET('o%d',$,'Own',$,(#10));\n", 300_000+i, i)
		fmt.Fprintf(&b, "#%d=IFCRELDEFINESBYPROPERTIES('r%d',$,$,$,(#%d),#%d);\n", 200_000+i, i, 100_000+i, 300_000+i)
	}
	fmt.Fprintf(&b, "#4=IFCWALLTYPE('t',$,'T',$,$,(%s),$,$,$,.STANDARD.);\n", strings.Join(sets, ","))
	fmt.Fprintf(&b, "#3=IFCRELDEFINESBYTYPE('r',$,$,$,(%s),#4);\n", strings.Join(walls, ","))
	b.WriteString("ENDSEC;\nEND-ISO-10303-21;\n")
	f := parseString(t, b.String())

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	res, err := Extract(f)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > sharedSetAlloc {
		t.Fatalf("extracting %d elements allocated %d MiB, want under %d: the type's sets are copied per element", sharedSetCount, alloc>>20, sharedSetAlloc>>20)
	}
	kept, left := 0, 0
	for _, e := range res.Elements {
		switch len(e.Psets) {
		case sharedSetCount + 1:
			kept++
		case 0:
			left++
		default:
			t.Fatalf("#%d has %d sets, want all %d or none", e.ExpressID, len(e.Psets), sharedSetCount+1)
		}
	}
	if kept == 0 || left == 0 {
		t.Fatalf("%d elements kept their sets and %d left them out, want some of each", kept, left)
	}
	if !slices.ContainsFunc(res.Warnings, func(w string) bool { return strings.Contains(w, "property sets left out") }) {
		t.Fatalf("no warning names the left-out sets: %q", res.Warnings)
	}
}

// Two of a type's quantity sets name the same quantity. The value must not
// depend on map order: the set last in name order wins, every time.
func TestATypesQuantityNamedTwiceTakesOneValue(t *testing.T) {
	f := parseString(t, "ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n"+
		"#1=IFCQUANTITYAREA('NetSideArea',$,$,1.,$);\n#2=IFCQUANTITYAREA('NetSideArea',$,$,2.,$);\n"+
		"#3=IFCELEMENTQUANTITY('a',$,'Qto_A',$,$,(#1));\n#4=IFCELEMENTQUANTITY('b',$,'Qto_B',$,$,(#2));\n"+
		"#5=IFCWALLTYPE('t',$,'T',$,$,(#4,#3),$,$,$,.STANDARD.);\n#6=IFCWALL('w',$,'W',$,$,$,$,$,$);\n"+
		"#7=IFCRELDEFINESBYTYPE('r',$,$,$,(#6),#5);\n"+
		"ENDSEC;\nEND-ISO-10303-21;\n")
	w, _ := f.ByID(6)
	for range 64 {
		q, ok := QtoQuantities(f, w, 1)
		if !ok || q.Area == nil || *q.Area != 2 {
			t.Fatalf("Qto.Area = %v, want 2 (Qto_B, last in name order)", q.Area)
		}
	}
}

// TestAMergeIntoATypesSetIsCharged covers the copy a merge makes: each
// element's own set has the name of its type's one large set, so the two
// merge, and each merge copies the type's sharedSetCount properties.
func TestAMergeIntoATypesSetIsCharged(t *testing.T) {
	var b strings.Builder
	b.WriteString("ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n")
	var props, walls []string
	for i := range sharedSetCount {
		fmt.Fprintf(&b, "#%d=IFCPROPERTYSINGLEVALUE('P%d',$,IFCREAL(1.),$);\n", 10+i, i)
		props = append(props, fmt.Sprintf("#%d", 10+i))
		fmt.Fprintf(&b, "#%d=IFCWALL('w%d',$,'W',$,$,$,$,$,$);\n", 100_000+i, i)
		walls = append(walls, fmt.Sprintf("#%d", 100_000+i))
		fmt.Fprintf(&b, "#%d=IFCPROPERTYSET('o%d',$,'Pset_X',$,(#10));\n", 300_000+i, i)
		fmt.Fprintf(&b, "#%d=IFCRELDEFINESBYPROPERTIES('r%d',$,$,$,(#%d),#%d);\n", 200_000+i, i, 100_000+i, 300_000+i)
	}
	fmt.Fprintf(&b, "#5=IFCPROPERTYSET('t',$,'Pset_X',$,(%s));\n", strings.Join(props, ","))
	fmt.Fprintf(&b, "#4=IFCWALLTYPE('t',$,'T',$,$,(#5),$,$,$,.STANDARD.);\n")
	fmt.Fprintf(&b, "#3=IFCRELDEFINESBYTYPE('r',$,$,$,(%s),#4);\n", strings.Join(walls, ","))
	b.WriteString("ENDSEC;\nEND-ISO-10303-21;\n")
	f := parseString(t, b.String())

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	res, err := Extract(f)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > sharedSetAlloc {
		t.Fatalf("extracting %d elements allocated %d MiB, want under %d: each merge copies the type's set uncharged", sharedSetCount, alloc>>20, sharedSetAlloc>>20)
	}
	for _, e := range res.Elements {
		if n := len(e.Psets["Pset_X"]); n != 0 && n != sharedSetCount {
			t.Fatalf("#%d: Pset_X has %d properties, want all %d or none", e.ExpressID, n, sharedSetCount)
		}
	}
	if !slices.ContainsFunc(res.Warnings, func(w string) bool { return strings.Contains(w, "property sets left out") }) {
		t.Fatalf("no warning names the left-out sets: %q", res.Warnings)
	}
}
