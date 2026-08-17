package geometry

import (
	"math"
	"testing"

	"github.com/blox-eng/goifc/model"
)

func TestLayerAxisMapsAxisAndSense(t *testing.T) {
	e := elemBox(v3{0, 0, 0}, v3{10, 0.3, 3})
	cases := []struct {
		name string
		ls   model.LayerSet
		want [3]float64
	}{
		{"AXIS2 positive", model.LayerSet{Direction: "AXIS2", Sense: "POSITIVE"}, [3]float64{0, 1, 0}},
		{"AXIS2 negative", model.LayerSet{Direction: "AXIS2", Sense: "NEGATIVE"}, [3]float64{0, -1, 0}},
		{"AXIS1 positive", model.LayerSet{Direction: "AXIS1", Sense: "POSITIVE"}, [3]float64{1, 0, 0}},
		{"AXIS3 positive", model.LayerSet{Direction: "AXIS3", Sense: "POSITIVE"}, [3]float64{0, 0, 1}},
		{"absent sense defaults positive", model.LayerSet{Direction: "AXIS2"}, [3]float64{0, 1, 0}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := LayerAxis(e, c.ls)
			if !ok {
				t.Fatal("LayerAxis declined a usable layer set")
			}
			for i := range got {
				if math.Abs(got[i]-c.want[i]) > 1e-9 {
					t.Fatalf("LayerAxis = %v, want %v", got, c.want)
				}
			}
		})
	}
}

func TestLayerAxisSurvivesExtremePlacementScale(t *testing.T) {
	// Placements come from untrusted files. At 1e200 the squared length
	// overflows to +Inf, and dividing a finite vector by an infinite length
	// gives a ZERO vector — returned with ok=true, which a caller comparing it
	// against Facing.Normal reads as "perpendicular" rather than "no direction".
	//
	// The mirror case: at 1e-200 the squared length underflows to 0, and a
	// perfectly usable direction is refused as degenerate.
	for _, s := range []float64{1e200, 1e-200} {
		e := elemBox(v3{0, 0, 0}, v3{10, 0.3, 3})
		e.Placement = model.Mat4{s, 0, 0, 0, 0, s, 0, 0, 0, 0, s, 0, 0, 0, 0, 1}

		got, ok := LayerAxis(e, model.LayerSet{Direction: "AXIS2", Sense: "POSITIVE"})
		if !ok {
			t.Fatalf("scale %g: LayerAxis declined a usable direction", s)
		}
		l := math.Sqrt(got[0]*got[0] + got[1]*got[1] + got[2]*got[2])
		if math.Abs(l-1) > 1e-9 {
			t.Fatalf("scale %g: LayerAxis = %v has length %v, want unit", s, got, l)
		}
		if math.Abs(got[1]-1) > 1e-9 {
			t.Fatalf("scale %g: LayerAxis = %v, want +Y", s, got)
		}
	}
}

func TestLayerAxisDeclinesWithoutDirection(t *testing.T) {
	e := elemBox(v3{0, 0, 0}, v3{10, 0.3, 3})
	for _, ls := range []model.LayerSet{
		{},                   // bare layer set, no usage
		{Direction: "AXIS9"}, // not a direction this schema defines
		{Sense: "POSITIVE"},  // sense without an axis says nothing
	} {
		if _, ok := LayerAxis(e, ls); ok {
			t.Fatalf("LayerAxis accepted %+v", ls)
		}
	}
}

func TestLayerAxisFollowsPlacementRotation(t *testing.T) {
	e := elemBox(v3{0, 0, 0}, v3{10, 0.3, 3})
	// A large, nonzero translation (100, 200, 300) is deliberate: LayerAxis
	// reports a direction, and a direction must ignore translation entirely.
	// The classic bug here is applying the full Placement — translation
	// included — to the local direction; with a huge translation, that bug
	// would blow the result far outside a unit vector and be unmistakable,
	// not a rounding-scale wobble a small offset might hide.
	e.Placement = model.Mat4{0, 1, 0, 0, -1, 0, 0, 0, 0, 0, 1, 0, 100, 200, 300, 1}
	got, ok := LayerAxis(e, model.LayerSet{Direction: "AXIS2", Sense: "POSITIVE"})
	if !ok {
		t.Fatal("LayerAxis declined the rotated element")
	}
	// Local +Y rotated 90° about Z is world -X, translation notwithstanding.
	if math.Abs(got[0]+1) > 1e-9 || math.Abs(got[1]) > 1e-9 {
		t.Fatalf("LayerAxis = %v, want world -X", got)
	}
}

func TestLayerAxisAgainstFacingDecidesOrder(t *testing.T) {
	// The consumer's question: does the DECLARED order already run from the
	// exposed face inward? A negative dot means the first layer is outermost.
	e := elemBox(v3{0, 0, 0}, v3{10, 0.3, 3})
	axis, ok := LayerAxis(e, model.LayerSet{Direction: "AXIS2", Sense: "POSITIVE"})
	if !ok {
		t.Fatal("LayerAxis declined")
	}
	outward := Facing{Normal: [3]float64{0, -1, 0}}
	dot := axis[0]*outward.Normal[0] + axis[1]*outward.Normal[1] + axis[2]*outward.Normal[2]
	if dot >= 0 {
		t.Fatalf("dot = %v, want negative: the stack runs away from the exposed face", dot)
	}
}
