package model

import (
	"fmt"
	"runtime"
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
