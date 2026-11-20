package wasmapi

import (
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/blox-eng/goifc/geometry"
)

func TestEveryHoleReasonHasACode(t *testing.T) {
	for _, r := range []geometry.HoleReason{
		geometry.ReasonHoleOutside, geometry.ReasonHolesOverlap, geometry.ReasonHoleSelfCrossing,
	} {
		if holeCodes[r] == "" {
			t.Errorf("hole reason %q has no wasm code", r)
		}
	}
}

// A roof's code is its reason, so the contract must list every reason, and
// Build must answer with it.
func TestEveryRoofReasonIsAContractCode(t *testing.T) {
	b, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Defs struct {
			Error struct {
				Properties struct {
					Code struct{ Enum []string } `json:"code"`
				} `json:"properties"`
			} `json:"error"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	for _, r := range []geometry.RoofReason{
		geometry.ReasonPitchOutOfRange, geometry.ReasonGableOutOfRange, geometry.ReasonGableRepeated,
		geometry.ReasonNoSlopedEdge, geometry.ReasonRoofStep, geometry.ReasonGableCollinear,
		geometry.ReasonOverhangOutOfRange, geometry.ReasonOverhangClosesOutline, geometry.ReasonRoofTooComplex,
	} {
		if !slices.Contains(s.Defs.Error.Properties.Code.Enum, string(r)) {
			t.Errorf("roof reason %q is not a code in the schema", r)
		}
		e := itemError("r1", &geometry.RoofError{Reason: r, Edge: 1, Other: -1})
		if e.Code != string(r) || e.Edge == nil || *e.Edge != 1 || e.Other != nil {
			t.Errorf("reason %q gave %+v, want its own code, edge 1, no other", r, e)
		}
	}
	e := itemError("r1", &geometry.RoofError{Reason: geometry.ReasonRoofStep, Edge: 2, Other: 4})
	if e.Edge == nil || *e.Edge != 2 || e.Other == nil || *e.Other != 4 {
		t.Errorf("error %+v, want edge 2 and other 4", e)
	}
	if e := itemError("r1", &geometry.RoofError{Reason: geometry.ReasonNoSlopedEdge, Edge: -1, Other: -1}); e.Edge != nil || e.Other != nil {
		t.Errorf("error %+v, want no edge and no other", e)
	}
}
