package ifc

import (
	"fmt"

	"github.com/blox-eng/goifc/geometry"
	"github.com/blox-eng/goifc/model"
	"github.com/blox-eng/goifc/step"
)

// Assembled is the end-to-end output of [Assemble]: the quantity-back-filled
// semantic Result — the []model.Element that downstream consumers golden-diff
// and import — paired with the proxy-geometry Scene it was derived from,
// for WriteGLB and mesh Stats. Elements in the two share identity via GlobalID.
type Assembled struct {
	Result *model.Result
	Scene  *geometry.Scene
}

// Assemble runs the full ifc pipeline over a parsed STEP file, in order:
//
//	model.Extract          semantic elements ([]model.Element)
//	geometry.Build         proxy geometry per element
//	Scene.DerivedQuantities tier-2 GROSS quantities from the meshes
//	Result.ApplyDerivedQuantities back-fills them onto un-authored elements
//
// It is the single production entry point for the engine: it chains the four
// stages so callers never have to reinvent the chain.
//
// Quantity tiering follows model.ApplyDerivedQuantities: authored Qto (tier-1,
// NET) always wins; where absent, the GROSS geometry-derived quantities back-fill
// and the element is tagged quantity_source="geometry"; an element with neither
// stays "none" — never a fabricated 0.0.
func Assemble(f *step.File) (*Assembled, error) {
	if f == nil {
		return nil, fmt.Errorf("ifc: nil step file")
	}
	r, err := model.Extract(f)
	if err != nil {
		return nil, fmt.Errorf("ifc: extract: %w", err)
	}
	s, err := geometry.Build(f, r)
	if err != nil {
		return nil, fmt.Errorf("ifc: build geometry: %w", err)
	}
	r.ApplyDerivedQuantities(s.DerivedQuantities())
	return &Assembled{Result: r, Scene: s}, nil
}
