package geometry

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/blox-eng/goifc/model"
)

// poolCount elements share one expensive shape. Each stays inside its own
// element budget, so only a budget for the whole Build bounds them (#106).
const poolCount = 2_000

// sharedShapeModels are the two ways one shape reaches every element: a comb
// face that each element's own brep names (the case in #106), and one body
// representation every element names, of poolCount small solids.
func sharedShapeModels() map[string]func(*strings.Builder, *model.Result) {
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
			bounded(t, "first build", func() { first, _ = Build(f, r) })
			bounded(t, "second build", func() { again, _ = Build(f, r) })
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
