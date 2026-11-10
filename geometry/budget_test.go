package geometry

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/goifc/model"
	"github.com/blox-eng/goifc/step"
)

// comb returns a closed comb-shaped polygon with teeth teeth: the ear-clipper's
// expensive case, since most vertices are reflex and every ear test scans the
// whole remaining loop.
func comb(teeth int) [][2]float64 {
	var p [][2]float64
	for i := range teeth {
		x := float64(2 * i)
		p = append(p, [2]float64{x, 0}, [2]float64{x, 10}, [2]float64{x + 1, 10}, [2]float64{x + 1, 1})
	}
	return append(p, [2]float64{float64(2 * teeth), 1}, [2]float64{float64(2 * teeth), -1}, [2]float64{0, -1})
}

func coveredArea(p [][2]float64, tris []uint32) float64 {
	var s float64
	for i := 0; i+2 < len(tris); i += 3 {
		s += math.Abs(cross2D(p[tris[i]], p[tris[i+1]], p[tris[i+2]])) / 2
	}
	return s
}

// timeLimit scales a wall-clock bound for an instrumented run: atomic coverage
// over ./..., as make test and CI run the suite, and -race make the meshing
// loops up to about fifteen times slower (the box walk's hub ladder: 0.8 s
// plain, 11 s covered). The bounds separate bounded work from work that grows
// per reference or per level, which is minutes, not seconds.
func timeLimit(d time.Duration) time.Duration {
	if raceBuild || testing.CoverMode() != "" {
		return 15 * d
	}
	return d
}

func TestTriangulatePolygonStopsAtItsBudget(t *testing.T) {
	b := sizedBudget(100_000)
	start := time.Now()
	tris, ok := triangulatePolygon(comb(2_000), b)
	if ok || tris != nil {
		t.Fatalf("an 8k-vertex comb on a 100k budget returned ok=%v with %d indices, want a decline", ok, len(tris))
	}
	if !b.exhausted() {
		t.Error("the budget is not marked exhausted after the decline")
	}
	if d := time.Since(start); d > timeLimit(time.Second) {
		t.Errorf("declining took %v; the budget does not bound the work", d)
	}
}

func TestTriangulatePolygonCoversRepeatedPointsAndSpikes(t *testing.T) {
	cases := map[string][][2]float64{
		"repeated point": {{0, 0}, {1, 0}, {1, 0}, {1, 1}, {0, 1}},
		"repeated close": {{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0, 0}},
		"spike":          {{0, 0}, {2, 0}, {2, 2}, {1, 2}, {1, 3}, {1, 2}, {0, 2}},
		"collinear":      {{0, 0}, {1, 0}, {2, 0}, {2, 1}, {2, 2}, {1, 2}, {0, 2}, {0, 1}},
		"near repeats":   {{0, -1}, {2, -1}, {2, 1}, {0, 1}, {6e-17, 1}, {-1, 0}, {-1.8e-16, -1}},
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			tris, ok := triangulatePolygon(p, nil)
			if !ok {
				t.Fatal("declined a polygon whose area it can cover")
			}
			want := math.Abs(polygonArea2D(p))
			if got := coveredArea(p, tris); math.Abs(got-want) > 1e-9 {
				t.Errorf("triangles cover %v, polygon area is %v", got, want)
			}
		})
	}
}

// A loop the ear-clipper cannot cover must decline, never ship the part it
// managed: a partial face makes a mesh smaller than the solid.
func TestTriangulatePolygonDeclinesWhatItCannotCover(t *testing.T) {
	cases := map[string][][2]float64{
		"bowtie":    {{0, 0}, {2, 2}, {2, 0}, {0, 2}},
		"crossing":  {{0, 0}, {4, 0}, {4, 4}, {2, -1}, {0, 4}},
		"hourglass": {{0, 0}, {3, 0}, {1, 1.5}, {3, 3}, {0, 3}, {2, 1.5}},
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			if tris, ok := triangulatePolygon(p, nil); ok {
				t.Errorf("returned %d triangles covering %v of %v; want a decline",
					len(tris)/3, coveredArea(p, tris), math.Abs(polygonArea2D(p)))
			}
		})
	}
}

// ifcFile wraps DATA lines in a minimal STEP envelope and parses it.
func ifcFile(t testing.TB, data string) *step.File {
	t.Helper()
	src := "ISO-10303-21;\nHEADER;\nFILE_DESCRIPTION((''),'2;1');\nFILE_NAME('t','',(''),(''),'','','');\n" +
		"FILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n" + data + "ENDSEC;\nEND-ISO-10303-21;\n"
	f, err := step.ParseBytes([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return f
}

// wallWith declares #1 as a wall whose Body representation holds items.
func wallWith(items string) string {
	return "#1=IFCWALL('w',$,$,$,$,$,#2,$,$);\n" +
		"#2=IFCPRODUCTDEFINITIONSHAPE($,$,(#3));\n" +
		"#3=IFCSHAPEREPRESENTATION($,'Body','Brep',(" + items + "));\n"
}

// boxSolid declares a 1x1x1 extruded box at #id..#id+2.
func boxSolid(id int) string {
	return fmt.Sprintf("#%d=IFCRECTANGLEPROFILEDEF(.AREA.,$,$,1.,1.);\n"+
		"#%d=IFCDIRECTION((0.,0.,1.));\n"+
		"#%d=IFCEXTRUDEDAREASOLID(#%d,$,#%d,1.);\n", id, id+1, id+2, id, id+1)
}

// A small file that names one large face many times must cost bounded work:
// the element declines to its box instead of re-triangulating the face per
// reference.
func TestElementMeshBoundsARepeatedHugeFace(t *testing.T) {
	var d strings.Builder
	d.WriteString(wallWith("#4"))
	p := comb(500)
	refs := make([]string, len(p))
	for i, q := range p {
		fmt.Fprintf(&d, "#%d=IFCCARTESIANPOINT((%g,%g,0.));\n", 100+i, q[0], q[1])
		refs[i] = fmt.Sprintf("#%d", 100+i)
	}
	fmt.Fprintf(&d, "#10=IFCPOLYLOOP((%s));\n#11=IFCFACEOUTERBOUND(#10,.T.);\n#12=IFCFACE((#11));\n", strings.Join(refs, ","))
	faces := strings.TrimSuffix(strings.Repeat("#12,", 5_000), ",")
	fmt.Fprintf(&d, "#5=IFCCLOSEDSHELL((%s));\n#4=IFCFACETEDBREP(#5);\n", faces)
	f := ifcFile(t, d.String())

	start := time.Now()
	m := elementMesh(f, 1, 1, nil)
	if d := time.Since(start); d > timeLimit(15*time.Second) {
		t.Fatalf("meshing took %v; the work is not bounded", d)
	}
	if !m.overBudget || m.src != SourceOBB {
		t.Errorf("overBudget=%v src=%s, want a budget decline to the box", m.overBudget, m.src)
	}
	if len(m.tris) != 36 {
		t.Errorf("got %d triangle indices, want the 36 of a box", len(m.tris))
	}
}

func TestElementMeshMarksAnEmptyItemPartial(t *testing.T) {
	// #20 is a mapped item with no mapping source: it meshes to nothing and
	// has no points to box, while the box beside it meshes fine.
	f := ifcFile(t, wallWith("#12,#20")+boxSolid(10)+"#20=IFCMAPPEDITEM($,$);\n")
	m := elementMesh(f, 1, 1, nil)
	if !m.partial {
		t.Error("an element that dropped an item is not marked partial")
	}
	if m.src != SourceExtrude || len(m.tris) == 0 {
		t.Errorf("src=%s with %d indices, want the surviving extrusion", m.src, len(m.tris))
	}
}

func TestElementMeshPropagatesPartialFromAMappedShape(t *testing.T) {
	// The wall maps #30, whose representation holds a box and a broken mapped item.
	f := ifcFile(t, wallWith("#40")+boxSolid(10)+
		"#20=IFCMAPPEDITEM($,$);\n"+
		"#30=IFCSHAPEREPRESENTATION($,'Body','MappedRepresentation',(#12,#20));\n"+
		"#31=IFCCARTESIANPOINT((0.,0.,0.));\n#32=IFCAXIS2PLACEMENT3D(#31,$,$);\n"+
		"#33=IFCREPRESENTATIONMAP(#32,#30);\n"+
		"#34=IFCCARTESIANTRANSFORMATIONOPERATOR3D($,$,#31,$,$);\n"+
		"#40=IFCMAPPEDITEM(#33,#34);\n")
	for _, c := range []*meshCache{nil, {}} {
		if m := elementMesh(f, 1, 1, c); !m.partial || len(m.tris) == 0 {
			t.Errorf("cache=%v: partial=%v with %d indices, want a partial mesh", c != nil, m.partial, len(m.tris))
		}
	}
}

func TestBuildWarnsAndCountsPartialAndOverBudgetElements(t *testing.T) {
	f := ifcFile(t, wallWith("#12,#20")+boxSolid(10)+"#20=IFCMAPPEDITEM($,$);\n")
	r := &model.Result{UnitScale: 1, Elements: []model.Element{{GlobalID: "w", ExpressID: 1, Placement: model.Identity()}}}
	s, err := Build(f, r)
	if err != nil {
		t.Fatal(err)
	}
	if st := s.Stats(); st.Partial != 1 || st.Extrude != 1 {
		t.Errorf("Stats = %+v, want Partial 1 counted alongside its Extrude source", st)
	}
	if !strings.Contains(strings.Join(s.Warnings, "\n"), "partial geometry for w") {
		t.Errorf("Warnings = %q, want a partial-geometry warning for w", s.Warnings)
	}
}

// Elements that share one large point list must not each re-read it.
func BenchmarkBuildSharedPointList(b *testing.B) {
	const points, elements = 200_000, 2_000
	var d strings.Builder
	d.WriteString("#1=IFCCARTESIANPOINTLIST3D((")
	for i := range points {
		if i > 0 {
			d.WriteByte(',')
		}
		fmt.Fprintf(&d, "(%d.,0.,0.)", i)
	}
	d.WriteString("));\n")
	// Each face set's CoordIndex is out of range, so it declines and boxes.
	r := &model.Result{UnitScale: 1}
	for e := range elements {
		id := 10 + 4*e
		fmt.Fprintf(&d, "#%d=IFCWALL('w%d',$,$,$,$,$,#%d,$,$);\n#%d=IFCPRODUCTDEFINITIONSHAPE($,$,(#%d));\n"+
			"#%d=IFCSHAPEREPRESENTATION($,'Body','Tessellation',(#%d));\n#%d=IFCTRIANGULATEDFACESET(#1,$,$,((1,2,%d)),$);\n",
			id, e, id+1, id+1, id+2, id+2, id+3, id+3, points+1)
		r.Elements = append(r.Elements, model.Element{GlobalID: fmt.Sprint("w", e), ExpressID: id, Placement: model.Identity()})
	}
	f := ifcFile(b, d.String())
	b.ResetTimer()
	for range b.N {
		if _, err := Build(f, r); err != nil {
			b.Fatal(err)
		}
	}
}

func TestDeclinedFaceSetBoxSpansItsWholePointList(t *testing.T) {
	f := ifcFile(t, wallWith("#5")+
		"#4=IFCCARTESIANPOINTLIST3D(((0.,0.,0.),(3.,1.,0.),(1.,5.,2.)));\n"+
		"#5=IFCTRIANGULATEDFACESET(#4,$,$,((1,2,9)),$);\n")
	for _, c := range []*meshCache{nil, {}} {
		m := elementMesh(f, 1, 1, c)
		lo, hi := worldAABB(m.verts, model.Identity())
		if m.src != SourceOBB || lo != [3]float64{0, 0, 0} || hi != [3]float64{3, 5, 2} {
			t.Errorf("cache=%v: src=%s box %v..%v, want the obb 0,0,0..3,5,2", c != nil, m.src, lo, hi)
		}
	}
}

// FuzzTriangulatePolygon holds the triangulator to its contract on arbitrary
// loops: indices stay in range, an accepted result covers the polygon's area,
// and the work never runs past the budget.
func FuzzTriangulatePolygon(f *testing.F) {
	f.Add([]byte{0, 0, 4, 0, 4, 4, 0, 4})
	f.Add([]byte{0, 0, 8, 8, 8, 0, 0, 8})
	f.Add([]byte{0, 0, 4, 0, 4, 0, 4, 4, 0, 4, 0, 0})
	f.Fuzz(func(t *testing.T, b []byte) {
		var p [][2]float64
		for i := 0; i+1 < len(b) && len(p) < 256; i += 2 {
			p = append(p, [2]float64{fuzzCoord(b[i]), fuzzCoord(b[i+1])})
		}
		bud := sizedBudget(50_000)
		tris, ok := triangulatePolygon(p, bud)
		if bud.left < -int64(len(p))*int64(len(p)+1) {
			t.Fatalf("overspent by %d: one pass costs at most n(n+1)", -bud.left)
		}
		if !ok {
			return
		}
		for _, i := range tris {
			if int(i) >= len(p) {
				t.Fatalf("index %d out of range for %d points", i, len(p))
			}
		}
		if len(p) >= 3 {
			want := math.Abs(polygonArea2D(p))
			if got := coveredArea(p, tris); math.Abs(got-want) > 1e-6*math.Max(got, want)+1e-9 {
				t.Fatalf("accepted triangles cover %v of %v", got, want)
			}
		}
	})
}

// A profile thinner than float32 can hold still has area: collapsing it to
// nothing must decline to the box, not ship side walls with no caps. A loop
// with no area at all (a spike) still meshes to nothing.
func TestTriangulatePolygonDeclinesALoopItCollapsedAway(t *testing.T) {
	thin := [][2]float64{{0, 0}, {1, 0}, {1, 1e-10}, {0, 1e-10}}
	if tris, ok := triangulatePolygon(thin, nil); ok {
		t.Errorf("a 1 x 1e-10 rectangle returned ok with %d indices, want a decline", len(tris))
	}
	if tris, ok := triangulatePolygon([][2]float64{{0, 0}, {1, 1}, {0, 0}}, nil); !ok || len(tris) != 0 {
		t.Errorf("a spike-only loop returned ok=%v with %d indices, want ok and nothing", ok, len(tris))
	}
}

// mappedComb declares a shape at #(id+3) whose body is a comb-faced brep: a
// mapped representation whose meshing costs real budget.
func mappedComb(id, teeth int) string {
	var d strings.Builder
	p := comb(teeth)
	refs := make([]string, len(p))
	for i, q := range p {
		fmt.Fprintf(&d, "#%d=IFCCARTESIANPOINT((%g,%g,0.));\n", id+10+i, q[0], q[1])
		refs[i] = fmt.Sprintf("#%d", id+10+i)
	}
	fmt.Fprintf(&d, "#%d=IFCPOLYLOOP((%s));\n#%d=IFCFACEOUTERBOUND(#%d,.T.);\n#%d=IFCFACE((#%d));\n"+
		"#%d=IFCFACETEDBREP(#%d);\n#%d=IFCCLOSEDSHELL((#%d));\n"+
		"#%d=IFCSHAPEREPRESENTATION($,'Body','Brep',(#%d));\n"+
		"#%d=IFCREPRESENTATIONMAP(#%d,#%d);\n#%d=IFCMAPPEDITEM(#%d,#%d);\n",
		id, strings.Join(refs, ","), id+1, id, id+2, id+1,
		id+4, id+5, id+5, id+2,
		id+6, id+4,
		id+7, 99, id+6, id+3, id+7, 98)
	return d.String()
}

const mappedFrame = "#97=IFCCARTESIANPOINT((0.,0.,0.));\n#99=IFCAXIS2PLACEMENT3D(#97,$,$);\n" +
	"#98=IFCCARTESIANTRANSFORMATIONOPERATOR3D($,$,#97,$,$);\n"

// A mapped shape's work is charged to every element that maps it, cached or
// not, so an element mapping many distinct shapes stays within one budget.
func TestMappedShapeWorkIsChargedToTheElement(t *testing.T) {
	f := ifcFile(t, mappedFrame+mappedComb(1000, 40)+mappedComb(2000, 40))
	a, _ := f.ByID(1003)
	bItem, _ := f.ByID(2003)
	for _, c := range []*meshCache{nil, {}} {
		probe := newBudget()
		if m := tessellateItemDepth(a, 1, 0, c, probe); len(m.tris) == 0 {
			t.Fatal("the mapped comb did not mesh")
		}
		cost := probe.spent()
		if cost <= 0 {
			t.Fatalf("cache=%v: mapping a comb shape charged %d, want its meshing cost", c != nil, cost)
		}
		// Room for one and a half shapes: the second is taken whole while budget
		// remains and spends it, so the third is refused.
		b := sizedBudget(cost + cost/2)
		first := tessellateItemDepth(a, 1, 0, c, b)
		second := tessellateItemDepth(bItem, 1, 0, c, b)
		third := tessellateItemDepth(a, 1, 0, c, b)
		if len(first.tris) == 0 || len(second.tris) == 0 {
			t.Errorf("cache=%v: a shape was refused with budget left", c != nil)
		}
		if !b.exhausted() {
			t.Errorf("cache=%v: two shapes did not spend a budget of one and a half", c != nil)
		}
		if len(third.tris) != 0 || !third.overBudget {
			t.Errorf("cache=%v: the third shape shipped %d indices, overBudget=%v; want it refused over budget",
				c != nil, len(third.tris), third.overBudget)
		}
	}
}

// arcBand is a thin C: an outer arc and an inner path back that dips into each
// outer ear, so every ear test runs to the end of the loop before failing.
func arcBand(m int) [][2]float64 {
	d := 3.0 / float64(m)
	var poly [][2]float64
	for k := range m {
		poly = append(poly, [2]float64{math.Cos(float64(k) * d), math.Sin(float64(k) * d)})
	}
	r := (1 + math.Cos(d)) / 2
	for k := m - 2; k >= 1; k-- {
		poly = append(poly, [2]float64{r * math.Cos(float64(k)*d), r * math.Sin(float64(k)*d)})
	}
	return poly
}

// One ear-clip pass costs up to n²: the budget is checked within the pass, so
// it overshoots by at most one ear test, and a spent budget clips nothing.
func TestTriangulatePolygonStopsWithinAPass(t *testing.T) {
	poly := arcBand(20_000)
	b := sizedBudget(1_000_000)
	if _, ok := triangulatePolygon(poly, b); ok {
		t.Fatal("an arc band on a 1M budget returned ok, want a decline")
	}
	if !b.exhausted() {
		t.Fatalf("declined with %d budget left; the decline was not budget-driven", b.left)
	}
	if over := -b.left; over > int64(len(poly))+1 {
		t.Errorf("overshot the budget by %d units, more than one ear test (%d)", over, len(poly)+1)
	}
	spent := sizedBudget(0)
	spent.spend(1)
	start := time.Now()
	if _, ok := triangulatePolygon(poly, spent); ok {
		t.Error("a spent budget triangulated a loop")
	}
	if d := time.Since(start); d > timeLimit(50*time.Millisecond) {
		t.Errorf("a spent budget still worked for %v", d)
	}
}

// Loops that touch or overlap themselves decline: a keyhole (until holes are
// read with earcut, #91), a hole wound the same way as its outline, a loop
// re-entering through a corner, and two lobes meeting at a point. Clipping
// such a loop either stalls or covers part of it twice.
func TestTriangulatePolygonDeclinesLoopsThatTouchThemselves(t *testing.T) {
	cases := map[string][][2]float64{
		"keyhole":             {{0, 0}, {10, 0}, {10, 10}, {0, 10}, {0, 5}, {3, 5}, {3, 8}, {6, 8}, {6, 5}, {3, 5}, {0, 5}},
		"same-direction hole": {{0, 0}, {10, 0}, {10, 10}, {0, 10}, {0, 5}, {3, 5}, {6, 5}, {6, 8}, {3, 8}, {3, 5}, {0, 5}},
		"corner loop":         {{0, 0}, {4, 0}, {4, 4}, {0, 4}, {0, 0}, {1, 0}, {1, 1}, {0, 1}},
		"figure-8":            {{0, 0}, {1, 0}, {1, 1}, {2, 1}, {2, 2}, {1, 2}, {1, 1}, {0, 1}},
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			if tris, ok := triangulatePolygon(p, nil); ok {
				t.Errorf("returned ok covering %v; want a decline", coveredArea(p, tris))
			}
		})
	}
}

// A brep that declines at once still has its box walked for every reference:
// an element naming one large brep many times must not walk it each time.
func TestElementMeshWalksARepeatedDecliningItemOnce(t *testing.T) {
	var d strings.Builder
	refs := make([]string, 2_000)
	for i := range refs {
		refs[i] = "#4"
	}
	d.WriteString(wallWith(strings.Join(refs, ",")))
	pts := make([]string, 50_000)
	for i := range pts {
		fmt.Fprintf(&d, "#%d=IFCCARTESIANPOINT((%d.,%d.,0.));\n", 100+i, i%977, i/977)
		pts[i] = fmt.Sprintf("#%d", 100+i)
	}
	// The first face's loop is not an IfcPolyLoop, so the brep declines before
	// any triangulation; the second face holds every point.
	fmt.Fprintf(&d, "#10=IFCPOLYLOOP((%s));\n#11=IFCFACEOUTERBOUND(#10,.T.);\n#12=IFCFACE((#11));\n", strings.Join(pts, ","))
	d.WriteString("#13=IFCFACEOUTERBOUND(#14,.T.);\n#14=IFCEDGELOOP(());\n#15=IFCFACE((#13));\n#5=IFCCLOSEDSHELL((#15,#12));\n#4=IFCFACETEDBREP(#5);\n")
	f := ifcFile(t, d.String())
	for _, c := range []*meshCache{nil, {}} {
		start := time.Now()
		m := elementMesh(f, 1, 1, c)
		if d := time.Since(start); d > timeLimit(2*time.Second) {
			t.Errorf("cache=%v: meshing took %v; the box walk repeats per reference", c != nil, d)
		}
		if lo, hi := worldAABB(m.verts, model.Identity()); hi[0] != 976 || lo[0] != 0 {
			t.Errorf("cache=%v: box %v..%v, want x 0..976", c != nil, lo, hi)
		}
	}
}

// Solids whose profile lists the solids again make each approximation hop
// re-walk every solid, and each hop's result feeds the next: K references
// compound per hop in both time and points unless every hop is bounded.
func TestCollectPointsBoundsAProfileThatReferencesItsSolids(t *testing.T) {
	for _, tc := range []struct{ solids, points int }{{7, 1}, {7, 0}, {2_000, 0}} {
		var d strings.Builder
		solids := make([]string, tc.solids)
		for i := range tc.solids {
			fmt.Fprintf(&d, "#%d=IFCEXTRUDEDAREASOLID(#1,$,$,1.);\n", 10+i)
			solids[i] = fmt.Sprintf("#%d", 10+i)
		}
		if tc.points > 0 {
			d.WriteString("#2=IFCCARTESIANPOINT((1.,2.));\n")
			solids = append(solids, "#2")
		}
		fmt.Fprintf(&d, "#1=IFCCOMPOSITEPROFILEDEF(.AREA.,$,(%s),$);\n", strings.Join(solids, ","))
		f := ifcFile(t, d.String())
		solid, _ := f.ByID(10)
		start := time.Now()
		pts, _ := collectPoints(solid, nil)
		if d := time.Since(start); d > timeLimit(500*time.Millisecond) {
			t.Errorf("%d solids, %d points: collectPoints took %v; the approximation ladder compounds", tc.solids, tc.points, d)
		}
		// Each solid contributes at most its profile's representative points,
		// extruded twice; nothing compounds from one hop to the next.
		if limit := (tc.solids + 1) * 2 * maxApproxPoints; len(pts) > limit {
			t.Errorf("%d solids, %d points: collectPoints returned %d points, more than %d; the hops compound", tc.solids, tc.points, len(pts), limit)
		}
	}
}

func TestBuildWarnsAboutAnOverBudgetElement(t *testing.T) {
	var d strings.Builder
	d.WriteString(wallWith("#4"))
	p := comb(500)
	refs := make([]string, len(p))
	for i, q := range p {
		fmt.Fprintf(&d, "#%d=IFCCARTESIANPOINT((%g,%g,0.));\n", 100+i, q[0], q[1])
		refs[i] = fmt.Sprintf("#%d", 100+i)
	}
	fmt.Fprintf(&d, "#10=IFCPOLYLOOP((%s));\n#11=IFCFACEOUTERBOUND(#10,.T.);\n#12=IFCFACE((#11));\n", strings.Join(refs, ","))
	fmt.Fprintf(&d, "#5=IFCCLOSEDSHELL((%s));\n#4=IFCFACETEDBREP(#5);\n", strings.TrimSuffix(strings.Repeat("#12,", 5_000), ","))
	r := &model.Result{UnitScale: 1, Elements: []model.Element{{GlobalID: "w", ExpressID: 1, Placement: model.Identity()}}}
	s, err := Build(ifcFile(t, d.String()), r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(s.Warnings, "\n"), "tessellation budget exceeded for w") {
		t.Errorf("Warnings = %q, want a budget warning for w", s.Warnings)
	}
	if st := s.Stats(); st.OBB != 1 {
		t.Errorf("Stats = %+v, want the element boxed", st)
	}
}

// An extrusion whose cap cannot be triangulated declines to a box that still
// spans its whole height: the box is built from the rings, not the caps.
func TestExtrusionWithAnUncoverableProfileBoxesItsFullExtent(t *testing.T) {
	f := ifcFile(t, wallWith("#9")+
		"#4=IFCCARTESIANPOINT((0.,0.));\n#5=IFCCARTESIANPOINT((2.,2.));\n#6=IFCCARTESIANPOINT((2.,0.));\n#7=IFCCARTESIANPOINT((0.,2.));\n"+
		"#8=IFCPOLYLINE((#4,#5,#6,#7,#4));\n#10=IFCARBITRARYCLOSEDPROFILEDEF(.AREA.,$,#8);\n#11=IFCDIRECTION((0.,0.,1.));\n"+
		"#9=IFCEXTRUDEDAREASOLID(#10,$,#11,3.);\n")
	m := elementMesh(f, 1, 1, nil)
	lo, hi := worldAABB(m.verts, model.Identity())
	if m.src != SourceOBB || lo != [3]float64{0, 0, 0} || hi != [3]float64{2, 2, 3} {
		t.Errorf("src=%s box %v..%v, want an obb spanning 0,0,0..2,2,3", m.src, lo, hi)
	}
}

// Elements sharing one point list read its bounds through one cache across
// Build's parallel workers; every one of them must get the whole list's box.
func TestBuildBoxesEveryElementSharingAPointList(t *testing.T) {
	var d strings.Builder
	d.WriteString("#1=IFCCARTESIANPOINTLIST3D(((0.,0.,0.),(3.,1.,0.),(1.,5.,2.)));\n")
	r := &model.Result{UnitScale: 1}
	for e := range 64 {
		id := 10 + 4*e
		fmt.Fprintf(&d, "#%d=IFCWALL('w%d',$,$,$,$,$,#%d,$,$);\n#%d=IFCPRODUCTDEFINITIONSHAPE($,$,(#%d));\n"+
			"#%d=IFCSHAPEREPRESENTATION($,'Body','Tessellation',(#%d));\n#%d=IFCTRIANGULATEDFACESET(#1,$,$,((1,2,9)),$);\n",
			id, e, id+1, id+1, id+2, id+2, id+3, id+3)
		r.Elements = append(r.Elements, model.Element{GlobalID: fmt.Sprint("w", e), ExpressID: id, Placement: model.Identity()})
	}
	s, err := Build(ifcFile(t, d.String()), r)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range s.Elements {
		if e.BBoxMin != [3]float64{0, 0, 0} || e.BBoxMax != [3]float64{3, 5, 2} {
			t.Errorf("%s box %v..%v, want 0,0,0..3,5,2", e.GlobalID, e.BBoxMin, e.BBoxMax)
		}
	}
}

func TestFaceSetsDeclineOverBudgetAndOnUncoverableFaces(t *testing.T) {
	tri := faceSetItem(t, unitCubePoints+"#61=IFCTRIANGULATEDFACESET(#60,$,$,((1,2,3),(1,3,4)),$);\n")
	if _, _, ok := faceSetMesh(tri, sizedBudget(1)); ok {
		t.Error("a triangulated set meshed past its budget")
	}
	bowtie := faceSetItem(t, unitCubePoints+
		"#62=IFCINDEXEDPOLYGONALFACE((1,3,2,4));\n#61=IFCPOLYGONALFACESET(#60,$,(#62),$);\n")
	if _, _, ok := faceSetMesh(bowtie, nil); ok {
		t.Error("a polygonal set with a self-intersecting face meshed; want a decline")
	}
}

func TestTriangulatePolygonDeclinesNonFiniteLoops(t *testing.T) {
	for name, p := range map[string][][2]float64{
		"nan":      {{0, 0}, {1, 0}, {math.NaN(), 1}, {0, 1}},
		"overflow": {{0, 0}, {1e200, 0}, {1e200, 1e200}, {0, 1e200}},
	} {
		if _, ok := triangulatePolygon(p, nil); ok {
			t.Errorf("%s: ok, want a decline", name)
		}
	}
}

// Clipping is charged per triangle: a clip on a spent budget declines.
func TestBooleanClipChargesItsBudget(t *testing.T) {
	f := ifcFile(t, boxSolid(10)+
		"#20=IFCCARTESIANPOINT((0.,0.,0.5));\n#21=IFCDIRECTION((0.,0.,1.));\n#22=IFCAXIS2PLACEMENT3D(#20,#21,$);\n"+
		"#23=IFCPLANE(#22);\n#24=IFCHALFSPACESOLID(#23,.F.);\n#25=IFCBOOLEANCLIPPINGRESULT(.DIFFERENCE.,#12,#24);\n")
	item, _ := f.ByID(25)
	if _, ok := clipMeshByDifference(item, 1, 0, nil, nil); !ok {
		t.Fatal("the clip declined on an unlimited budget; the fixture is wrong")
	}
	b := sizedBudget(13)
	if _, ok := clipMeshByDifference(item, 1, 0, nil, b); ok || !b.exhausted() {
		t.Errorf("a clip on a 13-unit budget returned ok=%v, exhausted=%v; want a budget decline", ok, b.exhausted())
	}
}

// The box walk is cached per item and scale across elements: two elements
// boxing one item share one walk, a different scale gets its own box, and a
// cached walk reports the same cost as a fresh one, so every element is
// charged alike whoever walked first.
func TestItemBoxIsSharedAcrossElementsPerScale(t *testing.T) {
	f := ifcFile(t, "#1=IFCCARTESIANPOINT((2.,4.,6.));\n#2=IFCBOOLEANRESULT(.UNION.,#1,#1);\n")
	item, _ := f.ByID(2)
	c := &meshCache{}
	walks := 0
	walk := func(scale float64) func() (v3, v3, bool, int64) {
		return func() (v3, v3, bool, int64) {
			walks++
			pts, cost := collectPoints(item, c)
			b := pointsBox(pts)
			return scaleV3(b.lo, scale), scaleV3(b.hi, scale), b.ok, cost
		}
	}
	_, hi1, _, cold := c.itemBox(item, 1, walk(1))
	_, _, _, warm := c.itemBox(item, 1, walk(1))
	_, hi2, _, _ := c.itemBox(item, 0.001, walk(0.001))
	_, _, _, fresh := (*meshCache)(nil).itemBox(item, 1, walk(1))
	if walks != 3 {
		t.Errorf("walked %d times for two lookups at scale 1, one at 0.001 and one uncached, want 3", walks)
	}
	if hi1 != (v3{2, 4, 6}) || hi2 != (v3{0.002, 0.004, 0.006}) {
		t.Errorf("boxes %v and %v, want the item at scale 1 and at 0.001", hi1, hi2)
	}
	if cold <= 0 || warm != cold || fresh != cold {
		t.Errorf("walk cost cold %d, warm %d, uncached %d; want one positive figure", cold, warm, fresh)
	}
}

// An opening whose mesh left an item out is not a footprint to deduct.
func TestNetAreasDistrustsAPartialOpening(t *testing.T) {
	_, m := buildNetAreas(t, "testdata/synthetic/netarea_partial_opening.ifc")
	assertUntrusted(t, onlyNet(t, m), "opening")
}

// mappedChain declares a chain of levels representations, each holding
// wrappers distinct breps over one comb shell and mapping the next level.
// It returns the id of the top mapped item.
func mappedChain(d *strings.Builder, levels, wrappers int) int {
	p := comb(150)
	refs := make([]string, len(p))
	for i, q := range p {
		fmt.Fprintf(d, "#%d=IFCCARTESIANPOINT((%g,%g,0.));\n", 100+i, q[0], q[1])
		refs[i] = fmt.Sprintf("#%d", 100+i)
	}
	fmt.Fprintf(d, "#10=IFCPOLYLOOP((%s));\n#11=IFCFACEOUTERBOUND(#10,.T.);\n#12=IFCFACE((#11));\n#13=IFCCLOSEDSHELL((#12));\n", strings.Join(refs, ","))
	d.WriteString(mappedFrame)
	id, next := 100_000, 0
	for level := levels; level >= 1; level-- {
		var items []string
		for range wrappers {
			fmt.Fprintf(d, "#%d=IFCFACETEDBREP(#13);\n", id)
			items = append(items, fmt.Sprintf("#%d", id))
			id++
		}
		if next != 0 {
			items = append(items, fmt.Sprintf("#%d", next))
		}
		rep, rmap, item := id, id+1, id+2
		id += 3
		fmt.Fprintf(d, "#%d=IFCSHAPEREPRESENTATION($,'Body','Brep',(%s));\n#%d=IFCREPRESENTATIONMAP(#99,#%d);\n#%d=IFCMAPPEDITEM(#%d,#98);\n",
			rep, strings.Join(items, ","), rmap, rep, item, rmap)
		next = item
	}
	return next
}

// Each representation in a mapped chain meshes on its own budget, so a chain
// must not let one element spend a full budget per level.
func TestANestedMappedChainStaysNearOneBudget(t *testing.T) {
	var d strings.Builder
	top := mappedChain(&d, maxMapDepth, 321)
	f := ifcFile(t, d.String())
	item, _ := f.ByID(top)
	b := newBudget()
	start := time.Now()
	m := tessellateItemDepth(item, 1, 0, nil, b)
	if spent := b.spent(); spent > 2*elementBudget {
		t.Errorf("an %d-level chain charged %d units, more than two budgets (%d)", maxMapDepth, spent, 2*elementBudget)
	}
	if !m.overBudget {
		t.Error("a chain that spent its budget is not flagged over budget")
	}
	if d := time.Since(start); d > timeLimit(3*time.Second) {
		t.Errorf("an %d-level chain took %v", maxMapDepth, d)
	}
}

// The approximation keeps a profile's own points when they are few, so a thin
// profile turned by its solid's placement keeps a thin box.
func TestTheApproximationKeepsAFewPointsProfileTight(t *testing.T) {
	f := ifcFile(t, "#1=IFCCARTESIANPOINT((0.,0.));\n#2=IFCCARTESIANPOINT((10.,10.));\n#3=IFCCARTESIANPOINT((10.1,9.9));\n#4=IFCCARTESIANPOINT((0.1,-0.1));\n"+
		"#5=IFCPOLYLINE((#1,#2,#3,#4,#1));\n#6=IFCARBITRARYCLOSEDPROFILEDEF(.AREA.,$,#5);\n#7=IFCCOMPOSITEPROFILEDEF(.AREA.,$,(#6),$);\n"+
		"#8=IFCCARTESIANPOINT((0.,0.,0.));\n#9=IFCDIRECTION((0.,0.,1.));\n#10=IFCDIRECTION((1.,-1.,0.));\n#11=IFCAXIS2PLACEMENT3D(#8,#9,#10);\n"+
		"#12=IFCEXTRUDEDAREASOLID(#7,#11,#9,1.);\n")
	solid, _ := f.ByID(12)
	b := pointsBox((&boxWalk{approx: map[[2]int][]v3{}, polys: map[int][][2]float64{}}).extrudedAreaApproxPoints(solid, 0))
	if thin := min(b.hi[0]-b.lo[0], b.hi[1]-b.lo[1]); !b.ok || thin > 0.5 {
		t.Errorf("a 14 x 0.14 strip turned onto an axis boxed to %v..%v; want one side under 0.5", b.lo, b.hi)
	}
}

// An opening whose mesh left an item out is not drawn as a hole.
func TestElevationSkipsAPartialOpening(t *testing.T) {
	_, _, _, v := buildElevation(t, "testdata/synthetic/elevation_partial_opening.ifc", [3]float64{0, 1, 0})
	if e := entityByID(t, v, wallA); len(e.Openings) != 0 {
		t.Errorf("a partial opening mesh was drawn as %d hole(s); want none", len(e.Openings))
	}
}

func TestPointsBoxRejectsAnUnboundedAxis(t *testing.T) {
	if pointsBox(nil).ok {
		t.Error("no points: ok")
	}
	if b := pointsBox([]v3{{1, 2, 3}}); !b.ok || b.lo != b.hi {
		t.Errorf("one point: %+v", b)
	}
	if pointsBox([]v3{{math.NaN(), 0, 0}}).ok {
		t.Error("a NaN axis: ok")
	}
}

// Reflex vertices are charged one unit each: a loop that opens on a long run of
// them runs out exactly at the reflex candidate that overdraws the budget,
// before any ear test.
func TestTriangulatePolygonChargesReflexCorners(t *testing.T) {
	// A square whose bottom edge is dented inward: the dent's vertices are
	// reflex, and the loop starts on them.
	const k = 1_000
	var poly [][2]float64
	for i := 1; i < k; i++ {
		x := 10 * float64(i) / k
		poly = append(poly, [2]float64{x, 2 * math.Sin(math.Pi*x/10)})
	}
	poly = append(poly, [2]float64{10, 0}, [2]float64{10, 10}, [2]float64{0, 10}, [2]float64{0, 0})
	b := sizedBudget(int64(len(poly)) + k/2)
	if _, ok := triangulatePolygon(poly, b); ok || b.left != -1 {
		t.Errorf("ok=%v, left=%d; want a decline at -1 on a reflex candidate", ok, b.left)
	}
}

// The loop's own size is charged before any ear test.
func TestTriangulatePolygonChargesTheLoopUpFront(t *testing.T) {
	sq := [][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}}
	b := sizedBudget(int64(len(sq)) - 1)
	if _, ok := triangulatePolygon(sq, b); ok || b.left != -1 {
		t.Errorf("a %d-unit budget on a %d-point loop: ok=%v, left=%d; want a decline at -1 before any ear test", len(sq)-1, len(sq), ok, b.left)
	}
}

// allocLimit bounds what one hostile element may allocate while it meshes.
// Measured at elementBudget 1<<23: the heaviest bounded fixture (zero-depth
// solids sharing a large profile, as the wall's items) allocates 827 MiB, and
// the lightest of these fixtures on the unbudgeted code allocated 1,988 MiB.
// 1.5 GiB keeps 1.8 times the first and under four fifths of the second;
// re-measure if elementBudget changes.
const allocLimit = 1536 << 20

// buildAllocLimit bounds what one Build of a hostile file may allocate. The
// Build budget lets the elements of a small file mesh about one element budget
// between them, and box as much again. Measured: the heaviest shared-shape
// fixture allocates 1,755 MiB, and the same fixture allocated 8.5 GiB before
// the Build budget (#106). 3 GiB keeps 1.7 times the first and about a third of
// the second.
const buildAllocLimit = 3 << 30

// boundedTime is 4 to 125 times what the bounded fixtures take on an idle
// machine (0.04 to 1.2 s), so a loaded machine still passes. Every blow-up on
// the unbudgeted code still fails a bound: by time (the hub ladder ran past 10
// minutes) or by allocLimit (the others allocated 3,845 to 24,368 MiB).
const boundedTime = 5 * time.Second

// bounded runs fn and fails t if it took longer than boundedTime (scaled by
// timeLimit) or allocated more than allocLimit bytes.
func bounded(t *testing.T, name string, fn func()) {
	t.Helper()
	boundedWithin(t, name, allocLimit, fn)
}

// boundedWithin is bounded with an allocation limit of limit bytes.
func boundedWithin(t *testing.T, name string, limit uint64, fn func()) {
	t.Helper()
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := time.Now()
	fn()
	took := time.Since(start)
	runtime.ReadMemStats(&after)
	alloc := after.TotalAlloc - before.TotalAlloc
	if took > timeLimit(boundedTime) || alloc > limit {
		t.Errorf("%s: took %v (limit %v), allocated %d MiB (limit %d MiB)", name, took, timeLimit(boundedTime), alloc>>20, limit>>20)
	}
}

// buildWall builds data, whose #1 is a wall "w", as a one-element scene.
func buildWall(t *testing.T, f *step.File) *Scene {
	t.Helper()
	r := &model.Result{UnitScale: 1, Elements: []model.Element{{GlobalID: "w", ExpressID: 1, Placement: model.Identity()}}}
	s, err := Build(f, r)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func budgetWarned(s *Scene, gid string) bool {
	return strings.Contains(strings.Join(s.Warnings, "\n"), "tessellation budget exceeded for "+gid)
}

// mappedReps declares a representation map at #rmap over items, framed by
// mappedFrame.
func mappedRep(d *strings.Builder, rep, rmap int, items []string) {
	fmt.Fprintf(d, "#%d=IFCSHAPEREPRESENTATION($,'Body','Brep',(%s));\n#%d=IFCREPRESENTATIONMAP(#99,#%d);\n",
		rep, strings.Join(items, ","), rmap, rep)
}

// mapItems declares n mapped items of #rmap from id on and returns their refs.
func mapItems(d *strings.Builder, id, n, rmap int) []string {
	refs := make([]string, n)
	for i := range n {
		fmt.Fprintf(d, "#%d=IFCMAPPEDITEM(#%d,#98);\n", id+i, rmap)
		refs[i] = fmt.Sprintf("#%d", id+i)
	}
	return refs
}

func checkBoundedOverBudget(t *testing.T, f *step.File) {
	t.Helper()
	var s *Scene
	bounded(t, "Build", func() { s = buildWall(t, f) })
	if !budgetWarned(s, "w") {
		t.Errorf("Warnings = %q, want a budget warning for w", s.Warnings)
	}
	e := s.Elements[0]
	// One budget, one budget of overdraw, and the last item, which ships whole.
	if out := len(e.Tris)/3 + len(e.Verts)/3; out > 3*elementBudget {
		t.Errorf("the element shipped %d output units, more than three budgets", out)
	}
	var m mesh
	bounded(t, "elementMesh without a cache", func() { m = elementMesh(f, 1, 1, nil) })
	if !m.overBudget {
		t.Error("without a cache the element is not flagged over budget")
	}
}

// A shape made only of box fallbacks used to cost nothing, so mapping it
// cloned its whole mesh for free at every placement: 48M triangles here.
func TestMappedBoxClonesAreChargedBySize(t *testing.T) {
	var d strings.Builder
	d.WriteString(mappedFrame)
	a := make([]string, 1_000)
	for i := range a {
		fmt.Fprintf(&d, "#%d=IFCCARTESIANPOINT((%d.,0.,0.));\n", 10_000+i, i)
		a[i] = fmt.Sprintf("#%d", 10_000+i)
	}
	mappedRep(&d, 20, 21, a)
	mappedRep(&d, 30, 31, mapItems(&d, 20_000, 1_000, 21))
	d.WriteString(wallWith(strings.Join(mapItems(&d, 40, 4, 31), ",")))
	checkBoundedOverBudget(t, ifcFile(t, d.String()))
}

// rects declares n distinct 1x1x1 extrusions over one profile from id on.
func rects(d *strings.Builder, id, n int) []string {
	d.WriteString("#5=IFCRECTANGLEPROFILEDEF(.AREA.,$,$,1.,1.);\n#6=IFCDIRECTION((0.,0.,1.));\n")
	refs := make([]string, n)
	for i := range n {
		fmt.Fprintf(d, "#%d=IFCEXTRUDEDAREASOLID(#5,$,#6,1.);\n", id+i)
		refs[i] = fmt.Sprintf("#%d", id+i)
	}
	return refs
}

// A rectangle extrusion costs 9 units of work but emits 8 vertices and 12
// triangles. On a budget its work alone fits in, its output must not.
func TestExtrusionOutputIsChargedToTheElement(t *testing.T) {
	var d strings.Builder
	refs := rects(&d, 1_000, 10_000)
	f := ifcFile(t, d.String())
	items := make([]*step.Instance, len(refs))
	for i := range refs {
		items[i], _ = f.ByID(1_000 + i)
	}
	const size = 100_000
	m := unionItems(items, 1, 0, nil, sizedBudget(size))
	if !m.overBudget {
		t.Error("10,000 rectangles on a 100k budget are not flagged over budget")
	}
	if out := len(m.tris)/3 + len(m.verts)/3; out > 3*size {
		t.Errorf("shipped %d output units on a %d budget", out, size)
	}
}

// Nested maps of one rectangle multiply its output: a few thousand lines
// build millions of boxes unless every clone is charged its size.
func TestNestedRectMapsWarnThroughBuild(t *testing.T) {
	var d strings.Builder
	d.WriteString(mappedFrame)
	mappedRep(&d, 20, 21, rects(&d, 1_000, 1))
	mappedRep(&d, 30, 31, mapItems(&d, 20_000, 1_000, 21))
	// A placement of #31 clones 1,000 rects of 20 output units each, charged
	// inside the map and again as the clone: about 40k units, rounded up to
	// 50k. Two budgets of placements run past the overdraw.
	d.WriteString(wallWith(strings.Join(mapItems(&d, 100_000, 2*elementBudget/50_000, 31), ",")))
	checkBoundedOverBudget(t, ifcFile(t, d.String()))
}

func TestUnionChargesEachItemsSize(t *testing.T) {
	f := ifcFile(t, boxSolid(10))
	item, _ := f.ByID(12)
	b := newBudget()
	if m := unionItems([]*step.Instance{item}, 1, 0, nil, b); len(m.tris) != 36 {
		t.Fatalf("the box meshed %d indices, want 36", len(m.tris))
	}
	if b.spent() < 8+12 {
		t.Errorf("a box that emits 8 vertices and 12 triangles charged %d", b.spent())
	}
}

// corpusModel parses a corpus model: a public one from the parity fixtures, a
// private one from $GOIFC_PRIVATE_CORPUS. ok is false when it is not present.
func corpusModel(t *testing.T, name string, private bool) (*step.File, bool) {
	t.Helper()
	var src []byte
	if private {
		dir := os.Getenv("GOIFC_PRIVATE_CORPUS")
		if dir == "" {
			return nil, false
		}
		b, err := os.ReadFile(filepath.Join(dir, name+".ifc"))
		if err != nil {
			return nil, false
		}
		src = b
	} else {
		gz, err := os.ReadFile(filepath.Join("..", "parity", "testdata", name+".ifc.gz"))
		if err != nil {
			return nil, false // the module zip prunes parity/
		}
		zr, err := gzip.NewReader(bytes.NewReader(gz))
		if err != nil {
			t.Fatal(err)
		}
		if src, err = io.ReadAll(zr); err != nil {
			t.Fatal(err)
		}
	}
	f, err := step.ParseBytes(src)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return f, true
}

// The private corpus's heaviest element and mapped shape (private_a), as
// measured with GOIFC_PRIVATE_CORPUS set. CI runs without it, so the figures
// that hold the budget at 1<<23 are checked here too.
const privateHeaviestElement, privateHeaviestShape = 274_620, 201_388

// Every real element stays sixteen times inside the budget, and every mapped
// shape four times inside its own share (elementBudget / (maxMapDepth+1)), so
// the budget cannot box or drop real geometry.
func TestCorpusElementsKeepSixteenfoldHeadroom(t *testing.T) {
	shapeLimit := int64(elementBudget / (maxMapDepth + 1) / 4)
	if privateHeaviestElement > elementBudget/16 || privateHeaviestShape > shapeLimit {
		t.Errorf("recorded private figures %d and %d break the headroom (%d, %d)", privateHeaviestElement, privateHeaviestShape, elementBudget/16, shapeLimit)
	}
	if os.Getenv("GOIFC_PRIVATE_CORPUS") == "" {
		t.Log("GOIFC_PRIVATE_CORPUS unset: the private models are checked by their recorded figures only")
	}
	models := []struct {
		name    string
		private bool
	}{{"ifcopenhouse", false}, {"duplex_a", false}, {"fzk_haus", false}, {"private_a", true}, {"office_a", true}}
	for _, md := range models {
		f, ok := corpusModel(t, md.name, md.private)
		if !ok {
			t.Logf("%s: not present, skipped", md.name)
			continue
		}
		r, err := model.Extract(f)
		if err != nil {
			t.Fatal(err)
		}
		c := &meshCache{}
		var heaviest int64
		var heaviestID string
		for _, e := range r.Elements {
			b := newBudget()
			m := unionItems(representationItems(f, e.ExpressID), r.UnitScale, 0, c, b)
			if m.overBudget {
				t.Errorf("%s: element %s went over budget", md.name, e.GlobalID)
			}
			if b.spent() > heaviest {
				heaviest, heaviestID = b.spent(), e.GlobalID
			}
		}
		var shape int64
		c.m.Range(func(_, v any) bool {
			shape = max(shape, v.(*cachedMesh).mesh.cost)
			return true
		})
		t.Logf("%s: heaviest element %s spends %d, heaviest mapped shape %d", md.name, heaviestID, heaviest, shape)
		if md.name == "private_a" && (heaviest > privateHeaviestElement || shape > privateHeaviestShape) {
			t.Errorf("private_a now spends %d and %d, past the recorded %d and %d: re-record them", heaviest, shape, privateHeaviestElement, privateHeaviestShape)
		}
		if heaviest > elementBudget/16 {
			t.Errorf("%s: element %s spends %d, more than a sixteenth of the budget (%d)", md.name, heaviestID, heaviest, elementBudget/16)
		}
		if shape > shapeLimit {
			t.Errorf("%s: a mapped shape spends %d, more than a quarter of its budget (%d)", md.name, shape, shapeLimit)
		}
	}
}

// shell50k declares #5, a closed shell whose faceted brep declines before any
// triangulation, over a 50k-point loop spanning x 0..976.
func shell50k(d *strings.Builder) {
	pts := make([]string, 50_000)
	for i := range pts {
		fmt.Fprintf(d, "#%d=IFCCARTESIANPOINT((%d.,%d.,0.));\n", 100+i, i%977, i/977)
		pts[i] = fmt.Sprintf("#%d", 100+i)
	}
	fmt.Fprintf(d, "#10=IFCPOLYLOOP((%s));\n#11=IFCFACEOUTERBOUND(#10,.T.);\n#12=IFCFACE((#11));\n", strings.Join(pts, ","))
	d.WriteString("#13=IFCFACEOUTERBOUND(#14,.T.);\n#14=IFCEDGELOOP(());\n#15=IFCFACE((#13));\n#5=IFCCLOSEDSHELL((#15,#12));\n")
}

// wrappers declares n distinct faceted breps over #5 from id on.
func wrappers(d *strings.Builder, id, n int) []string {
	refs := make([]string, n)
	for i := range n {
		fmt.Fprintf(d, "#%d=IFCFACETEDBREP(#5);\n", id+i)
		refs[i] = fmt.Sprintf("#%d", id+i)
	}
	return refs
}

// Distinct wrappers over one shared shell each walk it for their box: each
// must pay for its walk, or 2,000 of them take 20 s in one element.
func TestDistinctWrappersAreEachChargedTheirWalk(t *testing.T) {
	var d strings.Builder
	shell50k(&d)
	d.WriteString(wallWith(strings.Join(wrappers(&d, 200_000, 2_000), ",")))
	f := ifcFile(t, d.String())
	checkBoundedOverBudget(t, f)
	m := elementMesh(f, 1, 1, &meshCache{})
	if lo, hi := worldAABB(m.verts, model.Identity()); lo[0] != 0 || hi[0] != 976 {
		t.Errorf("box x %v..%v, want 0..976", lo[0], hi[0])
	}
}

// hubLadder declares k solids, each on its own composite profile that lists
// one hub, and the hub lists every solid again (plus one point).
func hubLadder(d *strings.Builder, k int) []string {
	solids := make([]string, k)
	for i := range k {
		fmt.Fprintf(d, "#%d=IFCCOMPOSITEPROFILEDEF(.AREA.,$,(#6),$);\n#%d=IFCEXTRUDEDAREASOLID(#%d,$,$,1.);\n", 10_000+i, 20_000+i, 10_000+i)
		solids[i] = fmt.Sprintf("#%d", 20_000+i)
	}
	fmt.Fprintf(d, "#7=IFCCARTESIANPOINT((1.,2.));\n#6=IFCCOMPOSITEPROFILEDEF(.AREA.,$,(%s,#7),$);\n", strings.Join(solids, ","))
	return solids
}

// Each solid's approximation re-walks the hub at every hop: about 9·K² visits
// from an O(K) file unless the walk is charged per visit.
func TestASharedHubLadderIsBounded(t *testing.T) {
	var d strings.Builder
	d.WriteString(wallWith(strings.Join(hubLadder(&d, 2_000), ",")))
	checkBoundedOverBudget(t, ifcFile(t, d.String()))
}

// A real walk past maxBoxWalk reports it, and its partial box is not shipped.
func TestCollectPointsReportsAnOverWalk(t *testing.T) {
	var d strings.Builder
	fmt.Fprintf(&d, "#4=IFCGEOMETRICSET((%s));\n", strings.Join(hubLadder(&d, 2_000), ","))
	set, _ := ifcFile(t, d.String()).ByID(4)
	if _, cost := collectPoints(set, nil); cost <= maxBoxWalk {
		t.Fatalf("walk cost %d, want past maxBoxWalk (%d)", cost, maxBoxWalk)
	}
	if v, _ := obbFromItem(set, 1, nil, nil); v != nil {
		t.Error("an over walk shipped its partial box")
	}
}

// A ring of more than eight points is kept as its box's corners: the same box,
// placed, in eight points.
func TestACompactedRingKeepsItsBox(t *testing.T) {
	f := ifcFile(t, `#1=IFCEXTRUDEDAREASOLID(#2,#5,#8,2.);
#2=IFCARBITRARYCLOSEDPROFILEDEF(.AREA.,$,#3);
#3=IFCPOLYLINE((#11,#12,#13,#14,#15,#11));
#5=IFCAXIS2PLACEMENT3D(#6,$,$);
#6=IFCCARTESIANPOINT((100.,50.,0.));
#8=IFCDIRECTION((0.,0.,1.));
#11=IFCCARTESIANPOINT((0.,0.));
#12=IFCCARTESIANPOINT((4.,0.));
#13=IFCCARTESIANPOINT((5.,3.));
#14=IFCCARTESIANPOINT((2.,6.));
#15=IFCCARTESIANPOINT((-1.,3.));
`)
	solid, _ := f.ByID(1)
	pts, _ := collectPoints(solid, nil)
	if b := pointsBox(pts); len(pts) != 8 || b.lo != (v3{99, 50, 0}) || b.hi != (v3{105, 56, 2}) {
		t.Errorf("%d points boxed %v..%v; want the 8 corners of 99,50,0..105,56,2", len(pts), b.lo, b.hi)
	}
}

// An item may still be meshed or boxed until the budget is overdrawn by a
// whole second budget, and not once it is.
func TestCanOverdrawStopsAtASecondBudget(t *testing.T) {
	b := sizedBudget(100)
	b.spend(199)
	if !b.canOverdraw() {
		t.Error("refused at 99 past the budget")
	}
	b.spend(1)
	if b.canOverdraw() {
		t.Error("still allowed at a whole budget past it")
	}
}

// One boxed wrapper over K solids sharing one valid M-point profile: each solid
// extrudes the profile's rings again, 2·M points apiece.
func TestSolidsSharingALargeProfileAreChargedTheirRings(t *testing.T) {
	const k, m = 8_000, 8_000
	var d strings.Builder
	pts := make([]string, m)
	for i := range m {
		a := 2 * math.Pi * float64(i) / m
		fmt.Fprintf(&d, "#%d=IFCCARTESIANPOINT((%.6f,%.6f));\n", 100+i, math.Cos(a), math.Sin(a))
		pts[i] = fmt.Sprintf("#%d", 100+i)
	}
	fmt.Fprintf(&d, "#8=IFCPOLYLINE((%s,#100));\n#9=IFCARBITRARYCLOSEDPROFILEDEF(.AREA.,$,#8);\n#6=IFCDIRECTION((0.,0.,1.));\n", strings.Join(pts, ","))
	solids := make([]string, k)
	for i := range k {
		fmt.Fprintf(&d, "#%d=IFCEXTRUDEDAREASOLID(#9,$,#6,%d.);\n", 20_000+i, i+1)
		solids[i] = fmt.Sprintf("#%d", 20_000+i)
	}
	fmt.Fprintf(&d, "#4=IFCGEOMETRICSET((%s));\n", strings.Join(solids, ","))
	d.WriteString(wallWith("#4"))
	checkBoundedOverBudget(t, ifcFile(t, d.String()))
}

// A box is the conservative fallback: a face set that declines still ships
// its box even when the walk costs more than the budget it ran out of, and it
// still pays for the walk.
func TestALargeDecliningFaceSetStillBoxes(t *testing.T) {
	var d strings.Builder
	d.WriteString("#4=IFCCARTESIANPOINTLIST3D((")
	for i := range 5_000 {
		if i > 0 {
			d.WriteString(",")
		}
		fmt.Fprintf(&d, "(%d.,%d.,0.)", i%100, i/100)
	}
	d.WriteString("));\n#5=IFCTRIANGULATEDFACESET(#4,$,$,(")
	for i := range 400 {
		if i > 0 {
			d.WriteString(",")
		}
		fmt.Fprintf(&d, "(%d,%d,%d)", 3*i+1, 3*i+2, 3*i+3)
	}
	d.WriteString("),$);\n")
	f := ifcFile(t, d.String())
	item, _ := f.ByID(5)
	const size = 1_000
	b := sizedBudget(size)
	m := unionItems([]*step.Instance{item}, 1, 0, nil, b)
	if m.src != SourceOBB || len(m.tris) != 36 {
		t.Fatalf("src=%s with %d indices, want the face set's box", m.src, len(m.tris))
	}
	if b.spent() < 5_000 {
		t.Errorf("a box walk over 5,000 points charged %d in all", b.spent())
	}
}

// A face set's box walk is charged by its points, not its index lists: the
// box comes from the point list's cached bounds. Meshing charges 3 units per
// triangle, so a walk charged by the triangle would pass maxBoxWalk before
// meshing declines, and a face set too large to mesh would ship nothing.
func TestAFaceSetsWalkIsChargedByItsPoints(t *testing.T) {
	const n, tris = 100, 20_000
	var d strings.Builder
	d.WriteString("#4=IFCCARTESIANPOINTLIST3D((")
	for i := range n {
		if i > 0 {
			d.WriteString(",")
		}
		fmt.Fprintf(&d, "(%d.,%d.,0.)", i%10, i/10)
	}
	idx := make([]string, tris)
	for i := range idx {
		a := i%(n-2) + 1
		idx[i] = fmt.Sprintf("(%d,%d,%d)", a, a+1, a+2)
	}
	d.WriteString("));\n")
	fmt.Fprintf(&d, "#5=IFCTRIANGULATEDFACESET(#4,$,$,(%s),$);\n", strings.Join(idx, ","))
	faces := make([]string, tris)
	for i := range faces {
		fmt.Fprintf(&d, "#%d=IFCINDEXEDPOLYGONALFACE(%s);\n", 10_000+i, idx[i])
		faces[i] = fmt.Sprintf("#%d", 10_000+i)
	}
	fmt.Fprintf(&d, "#6=IFCPOLYGONALFACESET(#4,$,(%s),$);\n", strings.Join(faces, ","))
	f := ifcFile(t, d.String())
	for _, id := range []int{5, 6} {
		item, _ := f.ByID(id)
		pts, cost := collectPoints(item, nil)
		if b := pointsBox(pts); !b.ok || b.lo != (v3{0, 0, 0}) || b.hi != (v3{9, 9, 0}) {
			t.Errorf("#%d: box %v..%v, want 0,0,0..9,9,0", id, b.lo, b.hi)
		}
		if cost < n || cost > 2*n+64 {
			t.Errorf("#%d: walk over %d points and %d triangles cost %d, want about %d", id, n, tris, cost, n)
		}
	}
}

// A face set far too large to mesh declines without charging the meshing it
// never did, so the budget is not overdrawn past the box and the set still
// ships its box, with the budget warning.
func TestAFaceSetFarTooLargeToMeshStillBoxes(t *testing.T) {
	const size, tris = 1_000, 1_000 // meshing would cost 3,000: past two budgets
	var d strings.Builder
	d.WriteString("#4=IFCCARTESIANPOINTLIST3D(((0.,0.,0.),(4.,0.,0.),(0.,5.,6.)));\n")
	idx := make([]string, tris)
	for i := range idx {
		idx[i] = "(1,2,3)"
	}
	fmt.Fprintf(&d, "#5=IFCTRIANGULATEDFACESET(#4,$,$,(%s),$);\n", strings.Join(idx, ","))
	item, _ := ifcFile(t, d.String()).ByID(5)
	m := unionItems([]*step.Instance{item}, 1, 0, nil, sizedBudget(size))
	if m.src != SourceOBB || len(m.tris) != 36 || !m.overBudget {
		t.Fatalf("src=%s with %d indices, overBudget=%v; want the face set's box, over budget", m.src, len(m.tris), m.overBudget)
	}
}

// One indexed face listing far more corners than the budget declines without
// charging them, so its polygonal face set still ships its box.
func TestAFaceFarTooLargeToMeshStillBoxes(t *testing.T) {
	const size = 1_000
	corners := make([]string, 3*size) // past two budgets, over three points
	for i := range corners {
		corners[i] = fmt.Sprint(i%3 + 1)
	}
	item, _ := ifcFile(t, fmt.Sprintf("#4=IFCCARTESIANPOINTLIST3D(((0.,0.,0.),(4.,0.,0.),(0.,5.,6.)));\n#6=IFCINDEXEDPOLYGONALFACE((%s));\n#5=IFCPOLYGONALFACESET(#4,$,(#6),$);\n", strings.Join(corners, ","))).ByID(5)
	m := unionItems([]*step.Instance{item}, 1, 0, nil, sizedBudget(size))
	if m.src != SourceOBB || len(m.tris) != 36 || !m.overBudget {
		t.Fatalf("src=%s with %d indices, overBudget=%v; want the face set's box, over budget", m.src, len(m.tris), m.overBudget)
	}
}

// A build of many elements sharing hostile geometry gives the same result
// whatever order the workers reach the shared items in.
func TestBuildIsIndependentOfWorkerOrder(t *testing.T) {
	var d strings.Builder
	shell50k(&d)
	refs := strings.Join(wrappers(&d, 200_000, 400), ",")
	r := &model.Result{UnitScale: 1}
	for i := range 8 {
		id := 1_000 + 10*i
		fmt.Fprintf(&d, "#%d=IFCWALL('w%d',$,$,$,$,$,#%d,$,$);\n#%d=IFCPRODUCTDEFINITIONSHAPE($,$,(#%d));\n#%d=IFCSHAPEREPRESENTATION($,'Body','Brep',(%s));\n",
			id, i, id+1, id+1, id+2, id+2, refs)
		r.Elements = append(r.Elements, model.Element{GlobalID: fmt.Sprintf("w%d", i), ExpressID: id, Placement: model.Identity()})
	}
	f := ifcFile(t, d.String())
	run := func(procs int) (*Scene, []int) {
		defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(procs))
		s, err := Build(f, r)
		if err != nil {
			t.Fatal(err)
		}
		tris := make([]int, len(s.Elements))
		for i, e := range s.Elements {
			tris[i] = len(e.Tris)
		}
		return s, tris
	}
	s1, t1 := run(1)
	s8, t8 := run(8)
	if !budgetWarned(s1, "w0") {
		t.Fatalf("Warnings = %q, want the shared wrappers over budget", s1.Warnings)
	}
	if fmt.Sprint(s1.Warnings) != fmt.Sprint(s8.Warnings) || s1.Stats() != s8.Stats() || fmt.Sprint(t1) != fmt.Sprint(t8) {
		t.Errorf("1 worker: %v %+v %v; 8 workers: %v %+v %v", s1.Warnings, s1.Stats(), t1, s8.Warnings, s8.Stats(), t8)
	}
}

// Solids that cannot extrude (zero depth) still read their profile before they
// decline: thousands of them sharing one large profile must not read it once
// apiece for free.
func TestSolidsSharingAProfileTheyCannotExtrudeAreBounded(t *testing.T) {
	const k, m = 8_000, 8_000
	var d strings.Builder
	pts := make([]string, m)
	for i := range m {
		a := 2 * math.Pi * float64(i) / m
		fmt.Fprintf(&d, "#%d=IFCCARTESIANPOINT((%.6f,%.6f));\n", 100+i, math.Cos(a), math.Sin(a))
		pts[i] = fmt.Sprintf("#%d", 100+i)
	}
	fmt.Fprintf(&d, "#8=IFCPOLYLINE((%s,#100));\n#9=IFCARBITRARYCLOSEDPROFILEDEF(.AREA.,$,#8);\n#6=IFCDIRECTION((0.,0.,1.));\n", strings.Join(pts, ","))
	solids := make([]string, k)
	for i := range k {
		fmt.Fprintf(&d, "#%d=IFCEXTRUDEDAREASOLID(#9,$,#6,0.);\n", 20_000+i)
		solids[i] = fmt.Sprintf("#%d", 20_000+i)
	}
	set := d.String() + fmt.Sprintf("#4=IFCGEOMETRICSET((%s));\n", strings.Join(solids, ","))
	// Inside one boxed set, and as the wall's own items, each meshed and boxed.
	// As items, about a thousand walks each read the profile before the two
	// budgets run out.
	for name, data := range map[string]string{"set": set + wallWith("#4"), "items": set + wallWith(strings.Join(solids, ","))} {
		f := ifcFile(t, data)
		for _, c := range []*meshCache{nil, {}} {
			bounded(t, fmt.Sprintf("%s, cache=%v", name, c != nil), func() { elementMesh(f, 1, 1, c) })
		}
	}
}

// Once an element has overdrawn its budget by a second budget, unionItems
// leaves the remaining items out without touching them: each would otherwise
// still read its profile and be charged for it, without end.
func TestUnionLeavesItemsOutOnceOverdrawn(t *testing.T) {
	var d strings.Builder
	pts := make([]string, 2_000)
	for i := range pts {
		a := 2 * math.Pi * float64(i) / float64(len(pts))
		fmt.Fprintf(&d, "#%d=IFCCARTESIANPOINT((%.6f,%.6f));\n", 100+i, math.Cos(a), math.Sin(a))
		pts[i] = fmt.Sprintf("#%d", 100+i)
	}
	fmt.Fprintf(&d, "#8=IFCPOLYLINE((%s,#100));\n#9=IFCARBITRARYCLOSEDPROFILEDEF(.AREA.,$,#8);\n#6=IFCDIRECTION((0.,0.,1.));\n", strings.Join(pts, ","))
	for i := range 2_000 {
		fmt.Fprintf(&d, "#%d=IFCEXTRUDEDAREASOLID(#9,$,#6,%d.);\n", 20_000+i, i+1)
	}
	f := ifcFile(t, d.String())
	items := make([]*step.Instance, 2_000)
	for i := range items {
		items[i], _ = f.ByID(20_000 + i)
	}
	const size = 100_000
	b := sizedBudget(size)
	m := unionItems(items, 1, 0, nil, b)
	if !m.partial || !m.overBudget {
		t.Errorf("partial=%v overBudget=%v, want both", m.partial, m.overBudget)
	}
	if b.spent() > 3*size {
		t.Errorf("spent %d on a %d budget: items past the overdraw were still meshed", b.spent(), size)
	}
}

// A box is refused once the budget is overdrawn by a second budget, even for
// an item that reaches obbFromItem directly.
func TestNoBoxPastTheOverdraw(t *testing.T) {
	f := ifcFile(t, "#1=IFCCARTESIANPOINT((1.,2.,3.));\n")
	item, _ := f.ByID(1)
	b := sizedBudget(100)
	b.spend(250)
	if m := tessellateItemDepth(item, 1, 0, nil, b); len(m.verts) != 0 || !m.overBudget {
		t.Errorf("shipped %d vertices, overBudget=%v; want no box past the overdraw", len(m.verts)/3, m.overBudget)
	}
}

// A walk that ran past maxBoxWalk saw only part of the item: its box is never
// shipped, cached or not.
func TestAnOverWalkShipsNoBox(t *testing.T) {
	f := ifcFile(t, "#1=IFCCARTESIANPOINT((1.,2.,3.));\n")
	item, _ := f.ByID(1)
	c := &meshCache{}
	const cost = maxBoxWalk + 1
	c.itemBox(item, 1, func() (v3, v3, bool, int64) { return v3{}, v3{1, 1, 1}, true, cost })
	b := sizedBudget(1_000)
	if v, _ := obbFromItem(item, 1, c, b); v != nil {
		t.Error("an over walk shipped its partial box")
	}
	if b.spent() != cost {
		t.Errorf("charged %d, want the walk's cost %d", b.spent(), int64(cost))
	}
}

// A clip reserves its triangles before clipping them: a clip far too large to
// finish declines without spending them, so its operand's box still ships.
func TestAClipFarTooLargeToFinishStillBoxes(t *testing.T) {
	f := ifcFile(t, boxSolid(10)+
		"#20=IFCCARTESIANPOINT((0.,0.,0.5));\n#21=IFCDIRECTION((0.,0.,1.));\n#22=IFCAXIS2PLACEMENT3D(#20,#21,$);\n"+
		"#23=IFCPLANE(#22);\n#24=IFCHALFSPACESOLID(#23,.F.);\n#25=IFCBOOLEANCLIPPINGRESULT(.DIFFERENCE.,#12,#24);\n")
	item, _ := f.ByID(25)
	// The operand spends 9 of 10 units; its 12 triangles cannot be reserved.
	m := unionItems([]*step.Instance{item}, 1, 0, nil, sizedBudget(10))
	if m.src != SourceOBB || len(m.tris) != 36 || !m.overBudget {
		t.Errorf("src=%v with %d indices, overBudget=%v; want the clip's box, over budget", m.src, len(m.tris), m.overBudget)
	}
}

// fatCoords is a coordinate list of m zeros: a point no IFC schema allows,
// since a point or direction has at most three coordinates.
func fatCoords(m int) string {
	return "(" + strings.TrimSuffix(strings.Repeat("0.,", m), ",") + ")"
}

// setItems names the face sets #100.. as a wall's n items.
func setItems(n int) string {
	refs := make([]string, n)
	for i := range n {
		refs[i] = fmt.Sprintf("#%d", 100+i)
	}
	return strings.Join(refs, ",")
}

// A point is read only when it has at most three coordinates. One point with 200,000
// coordinates, named thousands of times, must not be read in full each time.
func TestAPointWithTooManyCoordinatesIsNotReadPerReference(t *testing.T) {
	const m, l = 200_000, 2_000
	refs := strings.TrimSuffix(strings.Repeat("#10,", l), ",")
	brep := fmt.Sprintf("#10=IFCCARTESIANPOINT(%s);\n#11=IFCPOLYLOOP((%s));\n#12=IFCFACEOUTERBOUND(#11,.T.);\n"+
		"#13=IFCFACE((#12));\n#14=IFCCLOSEDSHELL((#13));\n#15=IFCFACETEDBREP(#14);\n", fatCoords(m), refs)
	// Face sets sharing one point list each decline at the point, then box.
	var sets strings.Builder
	fmt.Fprintf(&sets, "#10=IFCCARTESIANPOINTLIST3D((%s));\n", fatCoords(m))
	for i := range l {
		fmt.Fprintf(&sets, "#%d=IFCTRIANGULATEDFACESET(#10,$,$,((1,1,1)),$);\n", 100+i)
	}
	for name, data := range map[string]string{"brep loop": brep + wallWith("#15"), "face sets": sets.String() + wallWith(setItems(l))} {
		f := ifcFile(t, data)
		bounded(t, name, func() { elementMesh(f, 1, 1, &meshCache{}) })
	}
}

// Only a face's outer bound shapes its mesh, so inner bounds are not read:
// a face naming one large loop as a thousand holes costs its outer bound.
func TestAFacesInnerBoundsAreNotRead(t *testing.T) {
	const l, holes, faces = 10_000, 1_000, 10
	var d strings.Builder
	pts := make([]string, l)
	for i := range l {
		a := 2 * math.Pi * float64(i) / l
		fmt.Fprintf(&d, "#%d=IFCCARTESIANPOINT((%.6f,%.6f,0.));\n", 100+i, math.Cos(a), math.Sin(a))
		pts[i] = fmt.Sprintf("#%d", 100+i)
	}
	fmt.Fprintf(&d, "#20=IFCPOLYLOOP((%s));\n#21=IFCFACEBOUND(#20,.T.);\n", strings.Join(pts, ","))
	d.WriteString("#30=IFCCARTESIANPOINT((0.,0.,0.));\n#31=IFCCARTESIANPOINT((1.,0.,0.));\n#32=IFCCARTESIANPOINT((0.,1.,0.));\n" +
		"#22=IFCPOLYLOOP((#30,#31,#32));\n#23=IFCFACEOUTERBOUND(#22,.T.);\n")
	fmt.Fprintf(&d, "#24=IFCFACE((%s#23));\n", strings.Repeat("#21,", holes))
	fmt.Fprintf(&d, "#14=IFCCLOSEDSHELL((%s));\n#15=IFCFACETEDBREP(#14);\n", strings.TrimSuffix(strings.Repeat("#24,", faces), ","))
	f := ifcFile(t, d.String()+wallWith("#15"))
	var m mesh
	bounded(t, "elementMesh", func() { m = elementMesh(f, 1, 1, &meshCache{}) })
	if m.src == SourceOBB || m.overBudget {
		t.Errorf("src=%v, overBudget=%v; want the faces meshed within budget", m.src, m.overBudget)
	}
}

// A composite curve that names one parent curve again retraces it: no closed
// profile does that. Ten thousand segments over one 10,000-point polyline
// must not build a hundred million points.
func TestACompositeCurveRepeatingAParentIsNotBuilt(t *testing.T) {
	const l, k = 10_000, 10_000
	var d strings.Builder
	pts := make([]string, l)
	for i := range l {
		a := 2 * math.Pi * float64(i) / l
		fmt.Fprintf(&d, "#%d=IFCCARTESIANPOINT((%.6f,%.6f));\n", 100_000+i, math.Cos(a), math.Sin(a))
		pts[i] = fmt.Sprintf("#%d", 100_000+i)
	}
	fmt.Fprintf(&d, "#40=IFCPOLYLINE((%s));\n", strings.Join(pts, ","))
	segs := make([]string, k)
	for i := range k {
		fmt.Fprintf(&d, "#%d=IFCCOMPOSITECURVESEGMENT(.CONTINUOUS.,.T.,#40);\n", 200_000+i)
		segs[i] = fmt.Sprintf("#%d", 200_000+i)
	}
	fmt.Fprintf(&d, "#50=IFCCOMPOSITECURVE((%s),.F.);\n#51=IFCARBITRARYCLOSEDPROFILEDEF(.AREA.,$,#50);\n"+
		"#6=IFCDIRECTION((0.,0.,1.));\n#15=IFCEXTRUDEDAREASOLID(#51,$,#6,1.);\n", strings.Join(segs, ","))
	f := ifcFile(t, d.String()+wallWith("#15"))
	bounded(t, "elementMesh", func() { elementMesh(f, 1, 1, &meshCache{}) })
	prof, _ := f.ByID(51)
	if poly := profilePolygon(prof); poly != nil {
		t.Errorf("profilePolygon built %d points; want the retracing curve declined", len(poly))
	}
}

// A face's bounds are charged each time a shell names the face: one face with
// thousands of holes, named thousands of times, runs out of budget instead of
// walking every bound at every reference.
func TestAFacesBoundsAreChargedPerReference(t *testing.T) {
	const holes, faces = 40_000, 5_000
	var d strings.Builder
	d.WriteString("#30=IFCCARTESIANPOINT((0.,0.,0.));\n#31=IFCCARTESIANPOINT((1.,0.,0.));\n#32=IFCCARTESIANPOINT((0.,1.,0.));\n" +
		"#20=IFCPOLYLOOP((#30,#31,#32));\n#21=IFCFACEBOUND(#20,.T.);\n#23=IFCFACEOUTERBOUND(#20,.T.);\n")
	fmt.Fprintf(&d, "#24=IFCFACE((%s#23));\n", strings.Repeat("#21,", holes))
	fmt.Fprintf(&d, "#14=IFCCLOSEDSHELL((%s));\n#15=IFCFACETEDBREP(#14);\n", strings.TrimSuffix(strings.Repeat("#24,", faces), ","))
	f := ifcFile(t, d.String()+wallWith("#15"))
	var m mesh
	bounded(t, "elementMesh", func() { m = elementMesh(f, 1, 1, &meshCache{}) })
	if !m.overBudget {
		t.Errorf("src=%v, overBudget=false; want the repeated bounds charged past the budget", m.src)
	}
}

// A bounded half-space's boundary is charged on every clip that reads it:
// distinct clips sharing one long boundary run out of budget.
func TestAClipsBoundaryIsChargedPerClip(t *testing.T) {
	const n, k = 20_000, 1_000
	var d strings.Builder
	d.WriteString(boxSolid(10) + "#20=IFCCARTESIANPOINT((0.,0.,0.5));\n#21=IFCDIRECTION((0.,0.,1.));\n" +
		"#22=IFCAXIS2PLACEMENT3D(#20,#21,$);\n#23=IFCPLANE(#22);\n#30=IFCCARTESIANPOINT((0.,0.));\n")
	fmt.Fprintf(&d, "#31=IFCPOLYLINE((%s));\n#32=IFCPOLYGONALBOUNDEDHALFSPACE(#23,.F.,#22,#31);\n",
		strings.TrimSuffix(strings.Repeat("#30,", n), ","))
	clips := make([]string, k)
	for i := range k {
		fmt.Fprintf(&d, "#%d=IFCBOOLEANCLIPPINGRESULT(.DIFFERENCE.,#12,#32);\n", 1_000+i)
		clips[i] = fmt.Sprintf("#%d", 1_000+i)
	}
	f := ifcFile(t, d.String()+wallWith(strings.Join(clips, ",")))
	var m mesh
	bounded(t, "elementMesh", func() { m = elementMesh(f, 1, 1, &meshCache{}) })
	if !m.overBudget {
		t.Errorf("src=%v, overBudget=false; want the shared boundary charged past the budget", m.src)
	}
}

// A trim is a set of at most two selects. A trim naming one point a hundred
// thousand times is declined, not read at every arc that uses it.
func TestAnArcTrimLongerThanTwoIsDeclined(t *testing.T) {
	const n, k = 100_000, 500
	var d strings.Builder
	d.WriteString("#30=IFCCARTESIANPOINT((0.,0.));\n#31=IFCAXIS2PLACEMENT2D(#30,$);\n#32=IFCCIRCLE(#31,1.);\n" +
		"#33=IFCCARTESIANPOINT((1.,0.));\n#34=IFCCARTESIANPOINT((-1.,0.));\n")
	fmt.Fprintf(&d, "#35=IFCTRIMMEDCURVE(#32,(%s),(#34),.T.,.CARTESIAN.);\n", strings.TrimSuffix(strings.Repeat("#33,", n), ","))
	d.WriteString("#36=IFCTRIMMEDCURVE(#32,(#34),(#33),.T.,.CARTESIAN.);\n" +
		"#37=IFCCOMPOSITECURVESEGMENT(.CONTINUOUS.,.T.,#35);\n#38=IFCCOMPOSITECURVESEGMENT(.CONTINUOUS.,.T.,#36);\n" +
		"#39=IFCCOMPOSITECURVE((#37,#38),.F.);\n#40=IFCARBITRARYCLOSEDPROFILEDEF(.AREA.,$,#39);\n#6=IFCDIRECTION((0.,0.,1.));\n")
	solids := make([]string, k)
	for i := range k {
		fmt.Fprintf(&d, "#%d=IFCEXTRUDEDAREASOLID(#40,$,#6,1.);\n", 1_000+i)
		solids[i] = fmt.Sprintf("#%d", 1_000+i)
	}
	f := ifcFile(t, d.String()+wallWith(strings.Join(solids, ",")))
	bounded(t, "elementMesh", func() { elementMesh(f, 1, 1, &meshCache{}) })
	arc, _ := f.ByID(35)
	if pts := arcPoints(arc); pts != nil {
		t.Errorf("arcPoints read a %d-entry trim; want it declined", n)
	}
}

// An arc trimmed by parameters reads the file's plane angle unit. The lookup
// scans the file's units, so it is made once per file, not at every arc.
func TestThePlaneAngleUnitIsLookedUpOncePerFile(t *testing.T) {
	const units, k = 200_000, 600
	var d strings.Builder
	fmt.Fprintf(&d, "#90=IFCPROJECT('p',$,$,$,$,$,$,$,#91);\n#92=IFCSIUNIT(*,.LENGTHUNIT.,$,.METRE.);\n#91=IFCUNITASSIGNMENT((%s));\n",
		strings.TrimSuffix(strings.Repeat("#92,", units), ","))
	d.WriteString("#30=IFCCARTESIANPOINT((0.,0.));\n#31=IFCAXIS2PLACEMENT2D(#30,$);\n#32=IFCCIRCLE(#31,1.);\n" +
		"#35=IFCTRIMMEDCURVE(#32,(IFCPARAMETERVALUE(0.)),(IFCPARAMETERVALUE(3.)),.T.,.PARAMETER.);\n" +
		"#37=IFCCOMPOSITECURVESEGMENT(.CONTINUOUS.,.T.,#35);\n" +
		"#39=IFCCOMPOSITECURVE((#37),.F.);\n#40=IFCARBITRARYCLOSEDPROFILEDEF(.AREA.,$,#39);\n#6=IFCDIRECTION((0.,0.,1.));\n")
	solids := make([]string, k)
	for i := range k {
		fmt.Fprintf(&d, "#%d=IFCEXTRUDEDAREASOLID(#40,$,#6,1.);\n", 1_000+i)
		solids[i] = fmt.Sprintf("#%d", 1_000+i)
	}
	f := ifcFile(t, d.String()+wallWith(strings.Join(solids, ",")))
	bounded(t, "elementMesh", func() { elementMesh(f, 1, 1, &meshCache{}) })
}
