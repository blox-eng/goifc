package wasmapi

import (
	"encoding/json"
	"errors"
	"strconv"

	"github.com/blox-eng/goifc/geometry"
)

// Caps on one request.
const (
	maxRequestBytes = 16 << 20 // a build request's JSON
	maxItems        = 10_000   // walls + rooms + slabs in one build request
	maxReadBytes    = 48 << 20 // an IFC file given to Read: about 1.7 GiB of wasm memory, measured
)

// Error is one refusal in an answer. Code is what a program matches on;
// Message is for a person.
type Error struct {
	Code    string `json:"code"`
	Item    string `json:"item,omitempty"`
	Wall    *int   `json:"wall,omitempty"`
	Opening *int   `json:"opening,omitempty"`
	Message string `json:"message"`
}

// failed is the answer to any refused request.
type failed struct {
	GoifcVersion string  `json:"goifcVersion"`
	OK           bool    `json:"ok"`
	Errors       []Error `json:"errors"`
}

func failAnswer(version string, errs []Error) []byte {
	return mustJSON(failed{GoifcVersion: version, Errors: errs})
}

// mustJSON encodes v. Every contract type holds only finite numbers, strings
// and slices, so encoding cannot fail; a failure is a bug in this package.
func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic("wasmapi: " + err.Error())
	}
	return b
}

// codeOf gives the contract code of one error that is not a PlacementError.
func codeOf(err error) string {
	switch {
	case errors.Is(err, geometry.ErrTooLarge):
		return "too_large"
	case errors.Is(err, geometry.ErrNothingToDraw):
		return "nothing_to_draw"
	}
	return "invalid_input"
}

// flatten returns the leaves of err's errors.Join tree, depth first, so every
// joined refusal becomes its own Error.
func flatten(err error) []error {
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		var out []error
		for _, e := range j.Unwrap() {
			out = append(out, flatten(e)...)
		}
		return out
	}
	return []error{err}
}

// toErrors turns one item's error into contract errors.
func toErrors(item string, err error) []Error {
	var out []Error
	for _, leaf := range flatten(err) {
		e := Error{Code: codeOf(leaf), Item: item, Message: leaf.Error()}
		var pe *geometry.PlacementError
		if errors.As(leaf, &pe) {
			e.Code = string(pe.Reason)
			e.Wall, e.Opening = &pe.Wall, &pe.Opening
		}
		out = append(out, e)
	}
	return out
}

// thousands writes n with comma separators, as the geometry messages do.
func thousands(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
