package geometry

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/goifc/model"
	"github.com/blox-eng/goifc/step"
)

func loadFileAndModel(t *testing.T, name string) (*step.File, *model.Result) {
	t.Helper()
	path := filepath.Join("testdata", "synthetic", name)
	f, err := step.ParseFile(path)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	r, err := model.Extract(f)
	if err != nil {
		t.Fatalf("extract %s: %v", path, err)
	}
	return f, r
}

// obb_revolved.ifc documents an IfcRevolvedAreaSolid, which has no tessellation
// path. The diagnostic must name it.
//
// step.Instance.Type() (like the STEP/SPF keyword syntax itself) is always
// upper-cased — the file never carries a mixed-case original to recover — so
// the map key is "IFCREVOLVEDAREASOLID", not "IfcRevolvedAreaSolid". Lookup is
// case-insensitive here for the same reason IsA() is.
func TestUnhandledItemTypesNamesRevolvedSolid(t *testing.T) {
	f, r := loadFileAndModel(t, "obb_revolved.ifc")
	got := UnhandledItemTypes(f, r)
	if got["IFCREVOLVEDAREASOLID"] == 0 {
		t.Errorf("UnhandledItemTypes() = %v; want IfcRevolvedAreaSolid (IFCREVOLVEDAREASOLID) counted", got)
	}
}

// obb_boolean_revolved.ifc wraps that same IfcRevolvedAreaSolid as the FIRST
// operand of an IfcBooleanClippingResult. The wrapper is in handledItemTypes,
// so stopping at it reports nothing and the gap never ranks — even though the
// element still falls back to a box, because clipMeshByDifference recurses into
// that operand and finds no path. The half-space second operand is consumed by
// halfSpacePlane, not tessellateItemDepth, so it is not a gap and must not be
// counted.
func TestUnhandledItemTypesDescendsIntoBooleanOperands(t *testing.T) {
	f, r := loadFileAndModel(t, "obb_boolean_revolved.ifc")
	got := UnhandledItemTypes(f, r)
	if got["IFCREVOLVEDAREASOLID"] == 0 {
		t.Errorf("UnhandledItemTypes() = %v; want IfcRevolvedAreaSolid counted through the boolean wrapper", got)
	}
	if n := got["IFCHALFSPACESOLID"]; n != 0 {
		t.Errorf("UnhandledItemTypes() counted IfcHalfSpaceSolid %d time(s); the supported path consumes it, so it is not a gap", n)
	}
	if n := got["IFCBOOLEANCLIPPINGRESULT"]; n != 0 {
		t.Errorf("UnhandledItemTypes() counted the boolean wrapper %d time(s); a DIFFERENCE wrapper is dispatched, the operand is the gap", n)
	}
}

// A non-DIFFERENCE boolean is an unsupported OPERATION, not an unsupported
// operand, so the boolean itself is the gap and must be named.
func TestUnhandledItemTypesNamesNonDifferenceBoolean(t *testing.T) {
	f, r := loadFileAndModel(t, "obb_boolean_union.ifc")
	got := UnhandledItemTypes(f, r)
	if got["IFCBOOLEANRESULT"] == 0 {
		t.Errorf("UnhandledItemTypes() = %v; want the UNION IfcBooleanResult counted", got)
	}
}

// mapped_cycle.ifc's IfcMappedItem (#72) maps to a representation (#60) whose
// only item is itself (#72) — a self-referencing MappingSource chain. The
// resolution in countUnhandledItem must terminate at maxMapDepth instead of
// recursing forever, and it must not panic. There is no real geometry in the
// cycle to attribute, so the result is empty, not a crash.
func TestUnhandledItemTypesCyclicMappingTerminates(t *testing.T) {
	f, r := loadFileAndModel(t, "mapped_cycle.ifc")
	done := make(chan map[string]int, 1)
	go func() {
		done <- UnhandledItemTypes(f, r)
	}()
	select {
	case got := <-done:
		if len(got) != 0 {
			t.Errorf("UnhandledItemTypes() = %v, want empty (cycle has no real geometry)", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("UnhandledItemTypes() did not terminate on a cyclic IfcMappedItem chain")
	}
}

// A file whose every item is handled reports nothing. known_box.ifc's shape
// representation holds exactly one item, the brep (#61), so the map must be
// empty.
func TestUnhandledItemTypesEmptyWhenAllHandled(t *testing.T) {
	f, r := loadFileAndModel(t, "known_box.ifc")
	if got := UnhandledItemTypes(f, r); len(got) != 0 {
		t.Errorf("UnhandledItemTypes() = %v, want empty", got)
	}
}

// Presentation entities carry colour, not shape. If they were counted, a style
// assignment would outrank real missing solids in the gap ranking.
func TestUnhandledItemTypesIgnoresPresentation(t *testing.T) {
	// The lists must not overlap: an item cannot be both a tessellation path and
	// a presentation entity, and an entry in both would silently win one way.
	for _, p := range presentationItemTypes {
		for _, h := range handledItemTypes {
			if p == h {
				t.Errorf("%s appears in both handledItemTypes and presentationItemTypes", p)
			}
		}
	}
	if len(presentationItemTypes) == 0 {
		t.Fatal("presentationItemTypes is empty; IfcStyledItem would be ranked as a geometry gap")
	}
}

// The dispatch switch in tessellateItemDepth and handledItemTypes must name the
// same set of types. A silent divergence makes the coverage harness lie, which
// is worse than having no harness. This reads the source rather than trusting a
// comment.
func TestHandledItemTypesMatchesDispatch(t *testing.T) {
	want := dispatchedTypes(t, "geometry.go", "tessellateItemDepth")
	if len(want) == 0 {
		t.Fatal("found no IsA(...) calls in tessellateItemDepth; update this test")
	}

	got := map[string]bool{}
	for _, ty := range handledItemTypes {
		got[ty] = true
	}

	for ty := range want {
		if !got[ty] {
			t.Errorf("%s is dispatched in tessellateItemDepth but missing from handledItemTypes", ty)
		}
	}
	for ty := range got {
		if !want[ty] {
			t.Errorf("%s is in handledItemTypes but not dispatched in tessellateItemDepth", ty)
		}
	}
}

// dispatchedTypes returns every IFC type name passed as a string literal to an
// IsA call anywhere inside the named function.
//
// It parses the file rather than matching source text. The regex this replaced
// required the literal spelling `item.IsA("Ifc…")`, so a dispatch case added
// through a differently named receiver, a renamed parameter, or a helper
// predicate was invisible to it — and the reverse half of this test then
// blamed handledItemTypes and pushed the author to delete a correct entry.
// Walking the AST sees any receiver, and the recursion into nested function
// literals means a case tucked inside a closure still counts.
func dispatchedTypes(t *testing.T, filename, funcName string) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}
	var body *ast.BlockStmt
	for _, d := range file.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if ok && fd.Name.Name == funcName {
			body = fd.Body
			break
		}
	}
	if body == nil {
		t.Fatalf("function %q not found in %s", funcName, filename)
	}
	out := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "IsA" || len(call.Args) != 1 {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		s, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		if strings.HasPrefix(s, "Ifc") {
			out[s] = true
		}
		return true
	})
	return out
}
