package step

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
)

// parseArgs consumes a parenthesized STEP argument list, assuming the opening '('
// has already been read, and pushes its values onto p.stack. Nested lists and
// typed values recurse and collapse to one handle each (see closeList), so on
// return the stack holds exactly this list's top-level values.
func (p *parser) parseArgs() error {
	for {
		tok := p.s.Next()
		switch tok.Kind {
		case TokRParen:
			return nil
		case TokComma:
			continue
		case TokEOF:
			return fmt.Errorf("step: unexpected EOF in argument list")
		default:
			if err := p.pushValue(tok); err != nil {
				return err
			}
		}
	}
}

// closeList moves p.stack[mark:] into the value slab and returns a handle to it.
// A list is closed before its parent, so the parent's handle can point at it.
func (p *parser) closeList(mark int, kind Kind) Value {
	start := len(p.vals)
	p.vals = append(p.vals, p.stack[mark:]...)
	n := len(p.vals) - start
	p.stack = p.stack[:mark]
	return Value{kind: kind, x: slabRef(p.slab, start), n: uint32(n), f: p.f}
}

// str appends text to the string arena and returns its handle.
func (p *parser) str(kind Kind, text []byte) Value {
	off := len(p.strs)
	p.strs = append(p.strs, text...)
	return Value{kind: kind, x: slabRef(p.slab, off), n: uint32(len(text)), f: p.f}
}

// pushValue builds a Value from a leading token onto p.stack, recursing for
// lists and typed values.
func (p *parser) pushValue(tok Token) error {
	v := Value{f: p.f}
	switch tok.Kind {
	case TokDollar:
		v.kind = KindNull
	case TokStar:
		v.kind = KindDerived
	case TokRef:
		id, err := parseUint32(tok.Text)
		if err != nil {
			return fmt.Errorf("step: bad ref #%s: %w", tok.Text, err)
		}
		v.kind, v.x = KindRef, uint64(id)
	case TokEnum:
		v = p.str(KindEnum, tok.Text)
	case TokBool:
		// .T./.F. are BOOLEAN; .U. is the LOGICAL "unknown" — a distinct value, NOT
		// false (matches ifcopenshell, which surfaces .U. as "UNKNOWN").
		if len(tok.Text) == 1 && tok.Text[0] == 'U' {
			v.kind = KindLogical
			break
		}
		v.kind = KindBool
		if len(tok.Text) == 1 && tok.Text[0] == 'T' {
			v.x = 1
		}
	case TokInt:
		n, err := strconv.ParseInt(string(tok.Text), 10, 64)
		if err != nil {
			return fmt.Errorf("step: bad integer %q: %w", tok.Text, err)
		}
		v.kind, v.x = KindInt, uint64(n)
	case TokFloat:
		f, err := strconv.ParseFloat(string(tok.Text), 64)
		if err != nil {
			return fmt.Errorf("step: bad real %q: %w", tok.Text, err)
		}
		v.kind, v.x = KindFloat, math.Float64bits(f)
	case TokString:
		// Most strings carry no escapes and go into the arena as they are.
		if bytes.IndexByte(tok.Text, '\'') < 0 && bytes.IndexByte(tok.Text, '\\') < 0 {
			v = p.str(KindString, tok.Text)
			break
		}
		s, err := decodeString(tok.Text)
		if err != nil {
			return err
		}
		v = p.str(KindString, []byte(s))
	case TokBinary:
		v = p.str(KindBinary, tok.Text)
	case TokLParen:
		mark := len(p.stack)
		if err := p.parseArgs(); err != nil {
			return err
		}
		v = p.closeList(mark, KindList)
	case TokKeyword:
		// typed / simple value: KEYWORD ( inner ). The keyword is stored as the
		// list's first member; Value.Str and Value.List split it back out.
		if open := p.s.Next(); open.Kind != TokLParen {
			return fmt.Errorf("step: expected '(' after typed value %q, got %v", tok.Text, open.Kind)
		}
		mark := len(p.stack)
		p.stack = append(p.stack, p.str(KindString, tok.Text))
		if err := p.parseArgs(); err != nil {
			return err
		}
		v = p.closeList(mark, KindTyped)
	default:
		return fmt.Errorf("step: unexpected token %v in argument list", tok.Kind)
	}
	p.stack = append(p.stack, v)
	return nil
}

// parseUint32 parses a decimal instance id without the string conversion
// strconv needs; ids are on every reference, so this is the hottest number
// parse in a file.
func parseUint32(b []byte) (uint32, error) {
	if len(b) == 0 || len(b) > 10 {
		v, err := strconv.ParseUint(string(b), 10, 32)
		return uint32(v), err
	}
	var n uint64
	for _, c := range b {
		if c < '0' || c > '9' {
			v, err := strconv.ParseUint(string(b), 10, 32)
			return uint32(v), err
		}
		n = n*10 + uint64(c-'0')
	}
	if n > math.MaxUint32 {
		v, err := strconv.ParseUint(string(b), 10, 32)
		return uint32(v), err
	}
	return uint32(n), nil
}
