package step

import (
	"fmt"
	"strconv"
)

// parseArgs consumes a parenthesized STEP argument list, assuming the opening '('
// has already been read from s. It returns the ordered argument values and stops
// after consuming the matching ')'. Nested lists and typed values (KEYWORD(...))
// recurse. Refs are captured as KindRef with RefID set and Ref left nil for the
// resolution pass. An unexpected EOF or token is a hard error.
func parseArgs(s *Scanner) ([]Value, error) {
	var out []Value
	for {
		tok := s.Next()
		switch tok.Kind {
		case TokRParen:
			return out, nil
		case TokComma:
			continue
		case TokEOF:
			return nil, fmt.Errorf("step: unexpected EOF in argument list")
		default:
			v, err := valueFromToken(s, tok)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
	}
}

// valueFromToken builds a Value from a leading token, recursing for lists and
// typed values. s is used only when recursion is needed.
func valueFromToken(s *Scanner, tok Token) (Value, error) {
	switch tok.Kind {
	case TokDollar:
		return Value{Kind: KindNull}, nil
	case TokStar:
		return Value{Kind: KindDerived}, nil
	case TokRef:
		id, err := strconv.ParseUint(string(tok.Text), 10, 32)
		if err != nil {
			return Value{}, fmt.Errorf("step: bad ref #%s: %w", tok.Text, err)
		}
		return Value{Kind: KindRef, RefID: uint32(id)}, nil
	case TokEnum:
		return Value{Kind: KindEnum, Str: string(tok.Text)}, nil
	case TokBool:
		// .T./.F. are BOOLEAN; .U. is the LOGICAL "unknown" — a distinct value, NOT
		// false (matches ifcopenshell, which surfaces .U. as "UNKNOWN").
		if len(tok.Text) == 1 && tok.Text[0] == 'U' {
			return Value{Kind: KindLogical}, nil
		}
		return Value{Kind: KindBool, B: len(tok.Text) == 1 && tok.Text[0] == 'T'}, nil
	case TokInt:
		n, err := strconv.ParseInt(string(tok.Text), 10, 64)
		if err != nil {
			return Value{}, fmt.Errorf("step: bad integer %q: %w", tok.Text, err)
		}
		return Value{Kind: KindInt, I: n}, nil
	case TokFloat:
		f, err := strconv.ParseFloat(string(tok.Text), 64)
		if err != nil {
			return Value{}, fmt.Errorf("step: bad real %q: %w", tok.Text, err)
		}
		return Value{Kind: KindFloat, F: f}, nil
	case TokString:
		str, err := decodeString(tok.Text)
		if err != nil {
			return Value{}, err
		}
		return Value{Kind: KindString, Str: str}, nil
	case TokBinary:
		return Value{Kind: KindBinary, Str: string(tok.Text)}, nil
	case TokLParen:
		inner, err := parseArgs(s)
		if err != nil {
			return Value{}, err
		}
		return Value{Kind: KindList, List: inner}, nil
	case TokKeyword:
		// typed / simple value: KEYWORD ( inner )
		open := s.Next()
		if open.Kind != TokLParen {
			return Value{}, fmt.Errorf("step: expected '(' after typed value %q, got %v", tok.Text, open.Kind)
		}
		inner, err := parseArgs(s)
		if err != nil {
			return Value{}, err
		}
		return Value{Kind: KindTyped, Str: string(tok.Text), List: inner}, nil
	default:
		return Value{}, fmt.Errorf("step: unexpected token %v in argument list", tok.Kind)
	}
}
