package geometry

import (
	"fmt"
	"math"
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

func TestTriangulatePolygonStopsAtItsBudget(t *testing.T) {
	b := &budget{left: 100_000}
	start := time.Now()
	tris, ok := triangulatePolygon(comb(2_000), b)
	if ok || tris != nil {
		t.Fatalf("an 8k-vertex comb on a 100k budget returned ok=%v with %d indices, want a decline", ok, len(tris))
	}
	if !b.exhausted() {
		t.Error("the budget is not marked exhausted after the decline")
	}
	if d := time.Since(start); d > time.Second {
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
	if d := time.Since(start); d > 5*time.Second {
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
		bud := &budget{left: 50_000}
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
		b := &budget{left: cost + cost/2}
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
	b := &budget{left: 1_000_000}
	if _, ok := triangulatePolygon(poly, b); ok {
		t.Fatal("an arc band on a 1M budget returned ok, want a decline")
	}
	if !b.exhausted() {
		t.Fatalf("declined with %d budget left; the decline was not budget-driven", b.left)
	}
	if over := -b.left; over > int64(len(poly))+1 {
		t.Errorf("overshot the budget by %d units, more than one ear test (%d)", over, len(poly)+1)
	}
	spent := &budget{left: -1}
	start := time.Now()
	if _, ok := triangulatePolygon(poly, spent); ok {
		t.Error("a spent budget triangulated a loop")
	}
	if d := time.Since(start); d > 50*time.Millisecond {
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
		if d := time.Since(start); d > 2*time.Second {
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
		pts := collectPoints(solid, nil)
		if d := time.Since(start); d > 500*time.Millisecond {
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
	if _, _, ok := faceSetMesh(tri, &budget{left: 1}); ok {
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
	b := &budget{left: 13}
	if _, ok := clipMeshByDifference(item, 1, 0, nil, b); ok || !b.exhausted() {
		t.Errorf("a clip on a 13-unit budget returned ok=%v, exhausted=%v; want a budget decline", ok, b.exhausted())
	}
}

// The box walk is cached per item and scale across elements: two elements
// boxing one item share one walk, and a different scale gets its own box.
func TestItemBoxIsSharedAcrossElementsPerScale(t *testing.T) {
	f := ifcFile(t, "#1=IFCCARTESIANPOINT((2.,4.,6.));\n#2=IFCBOOLEANRESULT(.UNION.,#1,#1);\n")
	item, _ := f.ByID(2)
	c := &meshCache{}
	walks := 0
	walk := func(scale float64) func() (v3, v3, bool) {
		return func() (v3, v3, bool) {
			walks++
			b := pointsBox(collectPoints(item, c))
			return scaleV3(b.lo, scale), scaleV3(b.hi, scale), b.ok
		}
	}
	_, hi1, _ := c.itemBox(item, 1, walk(1))
	c.itemBox(item, 1, walk(1))
	_, hi2, _ := c.itemBox(item, 0.001, walk(0.001))
	if walks != 2 {
		t.Errorf("walked %d times for two lookups at scale 1 and one at 0.001, want 2", walks)
	}
	if hi1 != (v3{2, 4, 6}) || hi2 != (v3{0.002, 0.004, 0.006}) {
		t.Errorf("boxes %v and %v, want the item at scale 1 and at 0.001", hi1, hi2)
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
	if d := time.Since(start); d > 3*time.Second {
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
	b := pointsBox(extrudedAreaApproxPoints(solid, 0, nil, map[[2]int][]v3{}))
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
	b := &budget{left: int64(len(poly)) + k/2}
	if _, ok := triangulatePolygon(poly, b); ok || b.left != -1 {
		t.Errorf("ok=%v, left=%d; want a decline at -1 on a reflex candidate", ok, b.left)
	}
}

// The loop's own size is charged before any ear test.
func TestTriangulatePolygonChargesTheLoopUpFront(t *testing.T) {
	sq := [][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}}
	b := &budget{left: int64(len(sq)) - 1}
	if _, ok := triangulatePolygon(sq, b); ok || b.left != -1 {
		t.Errorf("a %d-unit budget on a %d-point loop: ok=%v, left=%d; want a decline at -1 before any ear test", len(sq)-1, len(sq), ok, b.left)
	}
}
