package geometry

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/goifc/model"
)

// poolCount elements share one expensive shape. Each stays inside its own
// element budget, so only a budget for the whole Build bounds them (#106).
const poolCount = 2_000

// sharedShapeModels are shapes whose shared work no cache removes: one body
// representation every element names, of poolCount small solids.
func sharedShapeModels() map[string]func(*strings.Builder, *model.Result) {
	element := func(d *strings.Builder, r *model.Result, e, rep int) {
		id := 1_000_000 + 2*e
		fmt.Fprintf(d, "#%d=IFCWALL('w%d',$,$,$,$,$,#%d,$,$);\n#%d=IFCPRODUCTDEFINITIONSHAPE($,$,(#%d));\n", id, e, id+1, id+1, rep)
		r.Elements = append(r.Elements, model.Element{GlobalID: fmt.Sprint("w", e), ExpressID: id, Placement: model.Identity()})
	}
	return map[string]func(*strings.Builder, *model.Result){
		"one representation of many solids named by every element": func(d *strings.Builder, r *model.Result) {
			d.WriteString("#10=IFCCARTESIANPOINT((0.,0.));\n#11=IFCCARTESIANPOINT((1.,0.));\n#12=IFCCARTESIANPOINT((1.,1.));\n#13=IFCCARTESIANPOINT((0.,1.));\n" +
				"#14=IFCPOLYLINE((#10,#11,#12,#13,#10));\n#15=IFCARBITRARYCLOSEDPROFILEDEF(.AREA.,$,#14);\n#16=IFCDIRECTION((0.,0.,1.));\n")
			solids := make([]string, poolCount)
			for i := range poolCount {
				fmt.Fprintf(d, "#%d=IFCEXTRUDEDAREASOLID(#15,$,#16,%d.);\n", 100+i, 1+i%7)
				solids[i] = fmt.Sprintf("#%d", 100+i)
			}
			fmt.Fprintf(d, "#20=IFCSHAPEREPRESENTATION($,'Body','SweptSolid',(%s));\n", strings.Join(solids, ","))
			for e := range poolCount {
				element(d, r, e, 20)
			}
		},
	}
}

// TestASharedShapeCannotMultiplyTheBuild builds each model twice. Both builds
// must stay bounded and agree: which elements the Build budget boxes or leaves
// out cannot depend on which worker ran first. In element order, the first
// elements mesh, the next are boxed, and only then are any left out.
func TestASharedShapeCannotMultiplyTheBuild(t *testing.T) {
	for name, build := range sharedShapeModels() {
		t.Run(name, func(t *testing.T) {
			var d strings.Builder
			r := &model.Result{UnitScale: 1}
			build(&d, r)
			f := ifcFile(t, d.String())
			var first, again *Scene
			boundedWithin(t, "first build", buildAllocLimit, func() { first, _ = Build(f, r) })
			boundedWithin(t, "second build", buildAllocLimit, func() { again, _ = Build(f, r) })
			out := 0
			for i, e := range first.Elements {
				if !reflect.DeepEqual(e, again.Elements[i]) {
					t.Fatalf("%s differs between two builds of one file", e.GlobalID)
				}
				switch {
				case len(e.Tris) == 0:
					out++
				case out > 0:
					t.Fatalf("%s has geometry after an element left out: the budget is not settled in element order", e.GlobalID)
				}
			}
			var boxes, left int
			for _, w := range first.Warnings {
				if _, err := fmt.Sscanf(w, "build budget exceeded: %d elements boxed and %d left out", &boxes, &left); err == nil {
					break
				}
			}
			if boxes == 0 || left != out {
				t.Errorf("warning counts %d boxed and %d left out, want some boxed and %d left out", boxes, left, out)
			}
			if !strings.Contains(strings.Join(first.Warnings, "\n"), "build budget") {
				t.Errorf("no warning names the Build budget: %d warnings", len(first.Warnings))
			}
		})
	}
}

// sharedLoopModels are shapes whose one expensive loop every element names
// through its own item: a comb face under each element's brep (the case in
// #106), and a comb profile under each element's extrusion.
func sharedLoopModels() map[string]func(*strings.Builder, *model.Result) {
	element := func(d *strings.Builder, r *model.Result, e, rep int) {
		id := 1_000_000 + 2*e
		fmt.Fprintf(d, "#%d=IFCWALL('w%d',$,$,$,$,$,#%d,$,$);\n#%d=IFCPRODUCTDEFINITIONSHAPE($,$,(#%d));\n", id, e, id+1, id+1, rep)
		r.Elements = append(r.Elements, model.Element{GlobalID: fmt.Sprint("w", e), ExpressID: id, Placement: model.Identity()})
	}
	return map[string]func(*strings.Builder, *model.Result){
		"a comb face named by every element's brep": func(d *strings.Builder, r *model.Result) {
			p := comb(500)
			refs := make([]string, len(p))
			for i, q := range p {
				fmt.Fprintf(d, "#%d=IFCCARTESIANPOINT((%g,%g,0.));\n", 100+i, q[0], q[1])
				refs[i] = fmt.Sprintf("#%d", 100+i)
			}
			fmt.Fprintf(d, "#10=IFCPOLYLOOP((%s));\n#11=IFCFACEOUTERBOUND(#10,.T.);\n#12=IFCFACE((#11));\n", strings.Join(refs, ","))
			for e := range poolCount {
				id := 2_000_000 + 3*e
				fmt.Fprintf(d, "#%d=IFCCLOSEDSHELL((#12));\n#%d=IFCFACETEDBREP(#%d);\n#%d=IFCSHAPEREPRESENTATION($,'Body','Brep',(#%d));\n", id, id+1, id, id+2, id+1)
				element(d, r, e, id+2)
			}
		},
		"a comb profile named by every element's extrusion": func(d *strings.Builder, r *model.Result) {
			p := comb(150)
			refs := make([]string, len(p)+1)
			for i, q := range p {
				fmt.Fprintf(d, "#%d=IFCCARTESIANPOINT((%g,%g));\n", 100+i, q[0], q[1])
				refs[i] = fmt.Sprintf("#%d", 100+i)
			}
			refs[len(p)] = refs[0]
			fmt.Fprintf(d, "#10=IFCPOLYLINE((%s));\n#11=IFCARBITRARYCLOSEDPROFILEDEF(.AREA.,$,#10);\n#12=IFCDIRECTION((0.,0.,1.));\n", strings.Join(refs, ","))
			for e := range poolCount {
				id := 2_000_000 + 2*e
				fmt.Fprintf(d, "#%d=IFCEXTRUDEDAREASOLID(#11,$,#12,%d.);\n#%d=IFCSHAPEREPRESENTATION($,'Body','SweptSolid',(#%d));\n", id, 1+e%7, id+1, id)
				element(d, r, e, id+1)
			}
		},
	}
}

// TestASharedLoopIsTriangulatedOncePerBuild: a loop many elements name is
// triangulated once per Build, and the Build budget charges it once, so every
// element keeps its real mesh (#106).
func TestASharedLoopIsTriangulatedOncePerBuild(t *testing.T) {
	for name, build := range sharedLoopModels() {
		t.Run(name, func(t *testing.T) {
			var d strings.Builder
			r := &model.Result{UnitScale: 1}
			build(&d, r)
			f := ifcFile(t, d.String())
			var s *Scene
			boundedWithin(t, "build", buildAllocLimit, func() { s, _ = Build(f, r) })
			for _, e := range s.Elements {
				if len(e.Tris) == 0 || e.Source == SourceOBB {
					t.Fatalf("%s is %s with %d triangles, want its real mesh", e.GlobalID, e.Source, len(e.Tris)/3)
				}
			}
			for _, w := range s.Warnings {
				if strings.Contains(w, "budget") {
					t.Fatalf("warning %q: a shared loop is charged more than once", w)
				}
			}
		})
	}
}

// TestACachedLoopIsChargedAsIfTriangulatedHere: a loop's triangulation is
// cached, but each element is charged its cost as if it ran it, so the cache
// changes no mesh. The comb face costs more than a mapped shape's share of a
// budget and less than an element's: the element that names it meshes it, and
// the element that maps it is refused it, whichever of the two comes first.
func TestACachedLoopIsChargedAsIfTriangulatedHere(t *testing.T) {
	var d strings.Builder
	p := comb(500)
	refs := make([]string, len(p))
	for i, q := range p {
		fmt.Fprintf(&d, "#%d=IFCCARTESIANPOINT((%g,%g,0.));\n", 1000+i, q[0], q[1])
		refs[i] = fmt.Sprintf("#%d", 1000+i)
	}
	fmt.Fprintf(&d, "#10=IFCPOLYLOOP((%s));\n#11=IFCFACEOUTERBOUND(#10,.T.);\n#12=IFCFACE((#11));\n", strings.Join(refs, ","))
	d.WriteString("#20=IFCCLOSEDSHELL((#12));\n#21=IFCFACETEDBREP(#20);\n#22=IFCSHAPEREPRESENTATION($,'Body','Brep',(#21));\n" +
		"#30=IFCCLOSEDSHELL((#12));\n#31=IFCFACETEDBREP(#30);\n#32=IFCSHAPEREPRESENTATION($,'Body','Brep',(#31));\n" +
		"#33=IFCCARTESIANPOINT((0.,0.,0.));\n#34=IFCAXIS2PLACEMENT3D(#33,$,$);\n#35=IFCREPRESENTATIONMAP(#34,#32);\n" +
		"#36=IFCCARTESIANTRANSFORMATIONOPERATOR3D($,$,#33,$,$);\n#37=IFCMAPPEDITEM(#35,#36);\n#38=IFCSHAPEREPRESENTATION($,'Body','MappedRepresentation',(#37));\n" +
		"#40=IFCWALL('named',$,$,$,$,$,#41,$,$);\n#41=IFCPRODUCTDEFINITIONSHAPE($,$,(#22));\n" +
		"#50=IFCWALL('mapped',$,$,$,$,$,#51,$,$);\n#51=IFCPRODUCTDEFINITIONSHAPE($,$,(#38));\n")
	f := ifcFile(t, d.String())
	named := model.Element{GlobalID: "named", ExpressID: 40, Placement: model.Identity()}
	mapped := model.Element{GlobalID: "mapped", ExpressID: 50, Placement: model.Identity()}
	for _, order := range [][]model.Element{{named, mapped}, {mapped, named}} {
		s, err := Build(f, &model.Result{UnitScale: 1, Elements: order})
		if err != nil {
			t.Fatal(err)
		}
		for i, e := range s.Elements {
			want := elementMesh(f, order[i].ExpressID, 1, nil)
			if e.Source != want.src || !reflect.DeepEqual(e.Tris, want.tris) {
				t.Errorf("%s first: %s is %s with %d triangles, want %s with %d as meshed alone", order[0].GlobalID, e.GlobalID, e.Source, len(e.Tris)/3, want.src, len(want.tris)/3)
			}
		}
		if s.Elements[0].Source == s.Elements[1].Source {
			t.Fatalf("both elements are %s: the face no longer costs between a mapped share and an element budget", s.Elements[0].Source)
		}
	}
}

// TestARefusedTriangulationRunsWithinWhatTheCallerHasLeft: a cached loop is
// triangulated within what its caller can still pay, and is cached only when
// it completes. A full-size run for a caller that cannot pay for it would be
// work no budget charges.
func TestARefusedTriangulationRunsWithinWhatTheCallerHasLeft(t *testing.T) {
	p := comb(100)
	loop := make([]v3, len(p))
	for i, q := range p {
		loop[i] = v3{q[0], q[1], 0}
	}
	c := &meshCache{}
	var ran int64
	triangulate := func(run *budget) ([]uint32, bool) {
		tris, ok := triangulateFace(loop, run)
		ran = run.spent()
		return tris, ok
	}
	poor := sizedBudget(1000)
	poor.cache = c
	// The budget stops a clip within one ear test, which reads at most every
	// point of the loop.
	if _, ok := poor.triangulated(12, triangulate); ok || ran > 1000+int64(len(loop)) || !poor.exhausted() {
		t.Fatalf("a 1,000-unit caller: ok=%v after %d units of work, exhausted=%v; want a refusal within its 1,000 and one ear test", ok, ran, poor.exhausted())
	}
	rich := newBudget()
	rich.cache = c
	if tris, ok := rich.triangulated(12, triangulate); !ok || len(tris) == 0 {
		t.Fatalf("a full budget after a refused one: ok=%v with %d indices, want the face", ok, len(tris))
	}
}

// TestSharedWorkOfOneInstanceIsChargedPerKind: a face named as an item is box
// walked, and named by a brep is triangulated. The Build budget charges both,
// not the second as a reuse of the first.
func TestSharedWorkOfOneInstanceIsChargedPerKind(t *testing.T) {
	var d strings.Builder
	p := comb(100)
	refs := make([]string, len(p))
	for i, q := range p {
		fmt.Fprintf(&d, "#%d=IFCCARTESIANPOINT((%g,%g,0.));\n", 1000+i, q[0], q[1])
		refs[i] = fmt.Sprintf("#%d", 1000+i)
	}
	fmt.Fprintf(&d, "#10=IFCPOLYLOOP((%s));\n#11=IFCFACEOUTERBOUND(#10,.T.);\n#12=IFCFACE((#11));\n", strings.Join(refs, ","))
	d.WriteString("#20=IFCCLOSEDSHELL((#12));\n#21=IFCFACETEDBREP(#20);\n" + wallWith("#12,#21"))
	f := ifcFile(t, d.String())
	_, own, shared := meshAs(f, 1, 1, &meshCache{}, stateMeshed)
	if len(shared) != 2 {
		t.Fatalf("%d pieces of shared work, want the face's box walk and its triangulation", len(shared))
	}
	full := own + shared[0].cost + shared[1].cost
	charged := map[sharedKey]bool{}
	got := poolCost(own, shared, func(k sharedKey) bool {
		first := !charged[k]
		charged[k] = true
		return first
	})
	if got != full {
		t.Errorf("the Build budget charged %d, want %d: one kind of work passed as a reuse of the other", got, full)
	}
}

// TestAPanickedTriangulationReleasesItsLoop: Build's workers recover a panic,
// so a triangulation that panics must not keep its loop locked: the next
// element to name the loop would wait forever.
func TestAPanickedTriangulationReleasesItsLoop(t *testing.T) {
	b := newBudget()
	b.cache = &meshCache{}
	func() {
		defer func() { _ = recover() }()
		b.triangulated(12, func(*budget) ([]uint32, bool) { panic("malformed loop") })
	}()
	done := make(chan bool)
	go func() {
		_, ok := b.triangulated(12, func(*budget) ([]uint32, bool) { return []uint32{0, 1, 2}, true })
		done <- ok
	}()
	select {
	case ok := <-done:
		if !ok {
			t.Errorf("the loop after a panic: ok=false, want it triangulated")
		}
	case <-time.After(timeLimit(boundedTime)):
		t.Fatal("the loop stayed locked after its triangulation panicked")
	}
}
