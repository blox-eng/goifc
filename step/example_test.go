package step_test

import (
	"fmt"
	"sort"

	"github.com/blox-eng/goifc/step"
)

// A tiny self-contained STEP/SPF document used by the examples.
const sampleIFC = `ISO-10303-21;
HEADER;
FILE_DESCRIPTION((''),'2;1');
FILE_NAME('demo.ifc','2026-07-21T00:00:00',(''),(''),'','demo','');
FILE_SCHEMA(('IFC4'));
ENDSEC;
DATA;
#1= IFCPROJECT('proj-guid',$,'Demo',$,$,$,$,$,#2);
#2= IFCUNITASSIGNMENT((#3));
#3= IFCSIUNIT(*,.LENGTHUNIT.,.MILLI.,.METRE.);
#10= IFCWALL('wall-guid',$,'Wall A',$,$,#11,$,'tag');
#11= IFCLOCALPLACEMENT($,#12);
#12= IFCAXIS2PLACEMENT3D(#13,$,$);
#13= IFCCARTESIANPOINT((0.,0.,0.));
ENDSEC;
END-ISO-10303-21;`

func ExampleParseBytes() {
	f, err := step.ParseBytes([]byte(sampleIFC))
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("schema:", f.SchemaID())
	fmt.Println("instances:", f.Len())
	// Output:
	// schema: IFC4
	// instances: 7
}

func ExampleFile_ByType() {
	f, _ := step.ParseBytes([]byte(sampleIFC))
	for _, wall := range f.ByType("IfcWall") { // exact type, case-insensitive
		name, _ := wall.Get(2) // positional attribute (schema-agnostic)
		placement, _ := wall.Ref(5)
		fmt.Printf("#%d %s name=%q placement=#%d\n",
			wall.ID(), wall.Type(), name.Str, placement.ID())
	}
	// Output:
	// #10 IFCWALL name="Wall A" placement=#11
}

func ExampleFile_Inverse() {
	f, _ := step.ParseBytes([]byte(sampleIFC))
	unit := f.ByType("IfcSiUnit")[0] // #3
	for _, referrer := range f.Inverse(unit) {
		fmt.Printf("#%d is referenced by #%d (%s)\n",
			unit.ID(), referrer.ID(), referrer.Type())
	}
	// Output:
	// #3 is referenced by #2 (IFCUNITASSIGNMENT)
}

func ExampleFile_Traverse() {
	f, _ := step.ParseBytes([]byte(sampleIFC))
	wall, _ := f.ByID(10)
	var ids []int
	for _, inst := range f.Traverse(wall, step.Unbounded, step.DepthFirst) {
		ids = append(ids, inst.ID())
	}
	sort.Ints(ids)
	fmt.Println("closure of #10:", ids)
	// Output:
	// closure of #10: [10 11 12 13]
}

func ExampleFile_All() {
	f, _ := step.ParseBytes([]byte(sampleIFC))
	count := 0
	for range f.All() { // no allocation; supports break
		count++
	}
	fmt.Println("total:", count)
	// Output:
	// total: 7
}
