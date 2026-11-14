package geometry

import (
	"errors"
	"fmt"
)

// ErrTooLarge is wrapped by every error that refuses input for breaking a
// limit: a dimension over 1,000 m, a point over 1,000 m from the first, a
// position over 1e7 m from the origin, more than 256 openings or 16 layers,
// and the WritePreviewSVG caps. errors.Is tells it from bad input.
var ErrTooLarge = errors.New("too large")

// ErrNothingToDraw is wrapped by the WritePreviewSVG error for input with
// nothing to draw.
var ErrNothingToDraw = errors.New("nothing to draw")

// limitError keeps a limit error's message exactly as written and wraps the
// kind it belongs to.
type limitError struct {
	msg  string
	kind error
}

func (e limitError) Error() string { return e.msg }
func (e limitError) Unwrap() error { return e.kind }

func tooLarge(format string, a ...any) error {
	return limitError{fmt.Sprintf(format, a...), ErrTooLarge}
}
