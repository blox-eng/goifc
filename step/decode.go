package step

import (
	"fmt"
	"strings"
	"unicode/utf16"
)

// decodeString unescapes the bytes between a STEP string literal's quotes
// (ISO 10303-21 §7.3.4) into a Go UTF-8 string. Handled escapes:
//
//	''            -> a single apostrophe
//	\X2\HHHH..\X0\ -> UTF-16BE code units (4 hex digits each) until \X0\
//	\X4\HHHHHHHH..\X0\ -> UTF-32 code points (8 hex digits each) until \X0\
//	\X\HH         -> one byte HH, interpreted as a Latin-1 code point
//	\S\c          -> the character c with 0x80 added (Latin-1 upper half)
//	\Pc\          -> code-page directive; consumed and ignored (best-effort)
//
// Text outside escapes is assumed ASCII/UTF-8 and copied through. Returns an
// error on malformed \X2\/\X4\/\X\ runs so the caller can reject the file.
func decodeString(raw []byte) (string, error) {
	var b strings.Builder
	b.Grow(len(raw))
	i := 0
	for i < len(raw) {
		c := raw[i]
		switch {
		case c == '\'' && i+1 < len(raw) && raw[i+1] == '\'':
			b.WriteByte('\'')
			i += 2

		case c == '\\' && i+3 < len(raw) && raw[i+1] == 'X' && raw[i+2] == '2' && raw[i+3] == '\\':
			n, err := decodeXRun(raw, i+4, 4, &b)
			if err != nil {
				return "", err
			}
			i = n

		case c == '\\' && i+3 < len(raw) && raw[i+1] == 'X' && raw[i+2] == '4' && raw[i+3] == '\\':
			n, err := decodeXRun(raw, i+4, 8, &b)
			if err != nil {
				return "", err
			}
			i = n

		case c == '\\' && i+2 < len(raw) && raw[i+1] == 'X' && raw[i+2] == '\\':
			// \X\HH — single byte as a Latin-1 code point
			if i+5 > len(raw) {
				return "", fmt.Errorf("step: \\X\\ needs 2 hex digits at offset %d", i)
			}
			v, ok := hex2(raw[i+3], raw[i+4])
			if !ok {
				return "", fmt.Errorf("step: bad \\X\\ hex at offset %d", i)
			}
			b.WriteRune(rune(v))
			i += 5

		case c == '\\' && i+2 < len(raw) && raw[i+1] == 'S' && raw[i+2] == '\\':
			// \S\c — c + 0x80, decoded as a Latin-1 code point
			if i+3 >= len(raw) {
				return "", fmt.Errorf("step: \\S\\ needs a following char at offset %d", i)
			}
			b.WriteRune(rune(raw[i+3]) + 0x80)
			i += 4

		case c == '\\' && i+1 < len(raw) && raw[i+1] == 'P':
			// \Pc\ — code page directive; consume up to and including the next '\'
			j := i + 2
			for j < len(raw) && raw[j] != '\\' {
				j++
			}
			if j < len(raw) {
				j++ // consume trailing '\'
			}
			i = j

		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String(), nil
}

// decodeXRun decodes a \X2\ (width=4) or \X4\ (width=8) hex run starting at
// off, appending decoded runes to b, until the \X0\ terminator. Returns the
// index just past \X0\.
func decodeXRun(raw []byte, off, width int, b *strings.Builder) (int, error) {
	var units []uint16 // only used for width==4 (UTF-16)
	i := off
	for {
		// terminator \X0\
		if i+4 <= len(raw) && raw[i] == '\\' && raw[i+1] == 'X' && raw[i+2] == '0' && raw[i+3] == '\\' {
			if width == 4 {
				for _, r := range utf16.Decode(units) {
					b.WriteRune(r)
				}
			}
			return i + 4, nil
		}
		if i+width > len(raw) {
			marker := "X2"
			if width == 8 {
				marker = "X4"
			}
			return 0, fmt.Errorf("step: unterminated \\%s\\ run at offset %d", marker, off)
		}
		v, err := parseHex(raw[i : i+width])
		if err != nil {
			return 0, err
		}
		if width == 4 {
			units = append(units, uint16(v))
		} else {
			b.WriteRune(rune(v))
		}
		i += width
	}
}

func parseHex(h []byte) (uint32, error) {
	var v uint32
	for _, c := range h {
		d, ok := hexDigit(c)
		if !ok {
			return 0, fmt.Errorf("step: bad hex digit %q", string(c))
		}
		v = v<<4 | uint32(d)
	}
	return v, nil
}

func hex2(a, b byte) (uint8, bool) {
	da, oka := hexDigit(a)
	db, okb := hexDigit(b)
	if !oka || !okb {
		return 0, false
	}
	return da<<4 | db, true
}

func hexDigit(c byte) (uint8, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	}
	return 0, false
}
