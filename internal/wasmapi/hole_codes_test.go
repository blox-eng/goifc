package wasmapi

import (
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
