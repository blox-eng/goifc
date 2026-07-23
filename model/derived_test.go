package model

import "testing"

func fp(v float64) *float64 { return &v }

// TestApplyDerivedQuantities proves the tiering contract: "none" elements with a
// derived entry become "geometry"; "qto" is never overwritten (prefer-net);
// missing/empty derived entries stay "none" (no phantom fabrication).
func TestApplyDerivedQuantities(t *testing.T) {
	qtoVol := 3.0
	r := &Result{Elements: []Element{
		{GlobalID: "A_none_withderived", QuantitySource: QuantitySourceNone},
		{GlobalID: "B_qto_keep", QuantitySource: QuantitySourceQto, Qto: Quantities{Volume: fp(qtoVol)}},
		{GlobalID: "C_none_noderived", QuantitySource: QuantitySourceNone},
		{GlobalID: "D_none_emptyderived", QuantitySource: QuantitySourceNone},
	}}
	derived := map[string]Quantities{
		"A_none_withderived":  {Volume: fp(9.9), Height: fp(2.5)},
		"B_qto_keep":          {Volume: fp(999)}, // must be ignored — element is already qto
		"D_none_emptyderived": {},                // empty → must not upgrade
	}
	r.ApplyDerivedQuantities(derived)

	a := r.Elements[0]
	if a.QuantitySource != QuantitySourceGeometry {
		t.Errorf("A source = %q, want geometry", a.QuantitySource)
	}
	if a.Qto.Volume == nil || *a.Qto.Volume != 9.9 || a.Qto.Height == nil || *a.Qto.Height != 2.5 {
		t.Errorf("A quantities not back-filled from derived: %+v", a.Qto)
	}

	b := r.Elements[1]
	if b.QuantitySource != QuantitySourceQto || b.Qto.Volume == nil || *b.Qto.Volume != qtoVol {
		t.Errorf("B (qto) was overwritten by geometry — prefer-net violated: src=%q vol=%v", b.QuantitySource, b.Qto.Volume)
	}

	c := r.Elements[2]
	if c.QuantitySource != QuantitySourceNone || !c.Qto.IsEmpty() {
		t.Errorf("C (no derived) must stay none/empty, got src=%q qto=%+v", c.QuantitySource, c.Qto)
	}

	d := r.Elements[3]
	if d.QuantitySource != QuantitySourceNone || !d.Qto.IsEmpty() {
		t.Errorf("D (empty derived) must stay none — no phantom upgrade, got src=%q qto=%+v", d.QuantitySource, d.Qto)
	}
}

func TestQuantities_IsEmpty(t *testing.T) {
	if !(Quantities{}).IsEmpty() {
		t.Error("zero Quantities should be empty")
	}
	if (Quantities{Height: fp(1)}).IsEmpty() {
		t.Error("Quantities with a Height should not be empty")
	}
}
