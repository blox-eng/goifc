package step

import "fmt"

// ParseError reports a STEP/SPF syntax error together with the byte offset into the
// source where parsing stopped. Callers can match it with errors.As:
//
//	f, err := step.ParseBytes(src)
//	var pe *step.ParseError
//	if errors.As(err, &pe) {
//		log.Printf("bad IFC at byte %d: %v", pe.Offset, pe.Err)
//	}
//
// I/O errors from ParseFile (e.g. a missing file) are returned unwrapped, so
// errors.Is(err, os.ErrNotExist) still works — only syntax errors are ParseErrors.
type ParseError struct {
	Offset int   // byte offset into the source where the scanner stopped
	Err    error // the underlying syntax error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s (at offset %d)", e.Err.Error(), e.Offset)
}

func (e *ParseError) Unwrap() error { return e.Err }
