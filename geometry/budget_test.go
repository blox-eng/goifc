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
