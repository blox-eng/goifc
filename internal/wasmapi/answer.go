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
	maxElements     = 100_000  // elements one build request builds, layers and room walls counted
	maxReadBytes    = 48 << 20 // an IFC file given to Read: about 1.7 GiB of wasm memory, measured
)

// Error is one refusal in an answer. Code is what a program matches on;
// Message is for a person.
type Error struct {
	Code    string `json:"code"`
	Item    string `json:"item,omitempty"`
	Wall    *int   `json:"wall,omitempty"`
	Opening *int   `json:"opening,omitempty"`
	Join    *int   `json:"join,omitempty"`
	Message string `json:"message"`
}

// failed is the answer to any refused request.
type failed struct {
	GoifcVersion string  `json:"goifcVersion"`
	OK           bool    `json:"ok"`
	Errors       []Error `json:"errors"`
}

// Failure is a refused answer with one error, for a caller outside this
// package that must answer without calling Build or Read.
func Failure(version, code, message string) []byte {
	return failAnswer(version, []Error{{Code: code, Message: message}})
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

// toErrors turns BuildPlan's refusals into contract errors, one per refusal.
func toErrors(err error) []Error {
	var out []Error
	for _, leaf := range flatten(err) {
		var je *geometry.JoinError
		var ie *geometry.ItemError
		switch {
		case errors.As(leaf, &je):
			out = append(out, Error{Code: string(je.Reason), Item: je.Item, Join: &je.Join, Message: je.Error()})
		case errors.As(leaf, &ie):
			for _, inner := range flatten(ie.Err) {
				out = append(out, itemError(ie.Item, inner))
			}
		default:
			out = append(out, Error{Code: codeOf(leaf), Message: leaf.Error()})
		}
	}
	return out
}

func itemError(item string, err error) Error {
	e := Error{Code: codeOf(err), Item: item, Message: err.Error()}
	var pe *geometry.PlacementError
	if errors.As(err, &pe) {
		e.Code = string(pe.Reason)
		e.Wall, e.Opening = &pe.Wall, &pe.Opening
	}
	return e
}

// thousands writes n with comma separators, as the geometry messages do.
func thousands(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
