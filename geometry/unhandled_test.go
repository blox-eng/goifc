package geometry

import (
	"os"
	"path/filepath"
	"regexp"
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
	src, err := os.ReadFile("geometry.go")
	if err != nil {
		t.Fatal(err)
	}
	fn := extractFunc(t, string(src), "func tessellateItemDepth")
	want := map[string]bool{}
	for _, m := range regexp.MustCompile(`item\.IsA\("(Ifc[A-Za-z]+)"\)`).FindAllStringSubmatch(fn, -1) {
		want[m[1]] = true
	}
	if len(want) == 0 {
		t.Fatal("found no item.IsA(...) calls in tessellateItemDepth; update this test")
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

// extractFunc returns the source text of the named function, from its
// declaration to the next top-level closing brace.
func extractFunc(t *testing.T, src, decl string) string {
	t.Helper()
	i := strings.Index(src, decl)
	if i < 0 {
		t.Fatalf("declaration %q not found", decl)
	}
	rest := src[i:]
	if j := strings.Index(rest, "\n}\n"); j >= 0 {
		return rest[:j]
	}
	return rest
}
