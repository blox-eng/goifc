package ifc

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/goifc/geometry"
)

// planTime bounds the storey plans of each hostile model below. Each takes
// over 10 s unbounded and well under a second bounded; the margin covers a
// slow or instrumented CI runner.
const planTime = 3 * time.Second

// plansWithin runs buildStoreyPlans and fails if it takes longer than
// planTime. A run that never ends is reported, not waited for.
func plansWithin(t *testing.T, nodes []ImportNode, mesh map[string]geometry.Element, elev map[string]float64) ([]StoreyPlan, []string) {
	t.Helper()
	type result struct {
		plans    []StoreyPlan
		warnings []string
	}
	done := make(chan result, 1)
	start := time.Now()
	go func() {
		p, w := buildStoreyPlans(nodes, mesh, elev)
		done <- result{p, w}
	}()
	select {
	case r := <-done:
		if d := time.Since(start); d > planTime {
			t.Fatalf("storey plans took %v, want under %v", d, planTime)
		}
		return r.plans, r.warnings
	case <-time.After(10 * planTime):
		t.Fatalf("storey plans did not finish in %v", 10*planTime)
		return nil, nil
	}
}

// Two elements that name each other as parent have no storey. Climbing from
// either must stop, not go round the cycle forever.
func TestStoreyPlansSurviveAParentCycle(t *testing.T) {
	nodes := []ImportNode{
		{GlobalID: "S", IFCClass: "IfcBuildingStorey"},
		{GlobalID: "W", IFCClass: "IfcWall", ParentIndex: iptr(0)},
		{GlobalID: "A", IFCClass: "IfcElementAssembly", ParentIndex: iptr(3)},
		{GlobalID: "B", IFCClass: "IfcElementAssembly", ParentIndex: iptr(2)},
	}
	mesh := map[string]geometry.Element{
		"W": boxElem("W", [3]float64{0, 0, 0}, [3]float64{1, 0.2, 3}),
		"A": boxElem("A", [3]float64{2, 0, 0}, [3]float64{3, 1, 1}),
	}
	plans, _ := plansWithin(t, nodes, mesh, map[string]float64{"S": 0})
	if len(plans) != 1 || plans[0].StoreyGlobalID != "S" {
		t.Fatalf("got %d plans, want one for S", len(plans))
	}
}

// Each element of a long parent chain climbs to the one storey at its top.
// Climbing the whole chain again for each element costs chain² steps.
func TestStoreyPlansClimbADeepChainOnce(t *testing.T) {
	const chain = 100_000
	nodes := []ImportNode{{GlobalID: "S", IFCClass: "IfcBuildingStorey"}}
	mesh := map[string]geometry.Element{}
	for i := 1; i <= chain; i++ {
		gid := fmt.Sprintf("E%d", i)
		nodes = append(nodes, ImportNode{GlobalID: gid, IFCClass: "IfcElementAssembly", ParentIndex: iptr(i - 1)})
		mesh[gid] = boxElem(gid, [3]float64{0, 0, 0}, [3]float64{1, 1, 3})
	}
	plans, _ := plansWithin(t, nodes, mesh, map[string]float64{"S": 0})
	if len(plans) != 1 || len(plans[0].Entities) != chain {
		t.Fatalf("got %d plans, want one holding all %d elements", len(plans), chain)
	}
}

// tallCount columns rise through tallCount storeys, so every storey cuts
// every column: tallCount² cuts from tallCount² × 2 small inputs. Past a
// budget that grows with the model's meshes, the plans leave elements out and
// say so.
func TestStoreyPlansOfTallElementsAreBounded(t *testing.T) {
	const tallCount = 1_000
	var nodes []ImportNode
	mesh := map[string]geometry.Element{}
	elev := map[string]float64{}
	for i := range tallCount {
		s, slab := fmt.Sprintf("S%d", i), fmt.Sprintf("F%d", i)
		z := 3 * float64(i)
		nodes = append(nodes, ImportNode{GlobalID: s, IFCClass: "IfcBuildingStorey"})
		nodes = append(nodes, ImportNode{GlobalID: slab, IFCClass: "IfcSlab", ParentIndex: iptr(2 * i)})
		mesh[slab] = boxElem(slab, [3]float64{-10, -10, z}, [3]float64{10, 10, z + 0.2})
		elev[s] = z
	}
	for i := range tallCount {
		c := fmt.Sprintf("C%d", i)
		x := float64(i % 40)
		y := float64(i / 40)
		nodes = append(nodes, ImportNode{GlobalID: c, IFCClass: "IfcColumn", ParentIndex: iptr(0)})
		mesh[c] = boxElem(c, [3]float64{x, y, 0}, [3]float64{x + 0.3, y + 0.3, 3 * tallCount})
	}

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	plans, warnings := plansWithin(t, nodes, mesh, elev)
	runtime.ReadMemStats(&after)
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 512<<20 {
		t.Fatalf("storey plans allocated %d MiB, want under 512", alloc>>20)
	}
	if len(plans) != tallCount {
		t.Fatalf("got %d plans, want %d", len(plans), tallCount)
	}
	if got := len(plans[0].Entities); got != tallCount+1 {
		t.Fatalf("the lowest storey holds %d elements, want its slab and all %d columns", got, tallCount)
	}
	if len(warnings) == 0 || !strings.Contains(warnings[0], "storey plans") {
		t.Fatalf("warnings = %q, want one that says the storey plans left elements out", warnings)
	}
}
