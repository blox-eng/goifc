package step

import "strconv"

// TokenKind classifies a STEP/SPF (ISO 10303-21) lexical token. The set mirrors
// ifcopenshell's IfcSpfLexer token types, ported to Go.
type TokenKind uint8

const (
	TokEOF     TokenKind = iota
	TokLParen            // (
	TokRParen            // )
	TokComma             // ,
	TokEquals            // =
	TokSemi              // ;
	TokDollar            // $  unset / null
	TokStar              // *  derived-in-supertype placeholder
	TokRef               // #123  (Text excludes the '#')
	TokString            // '...' (Text = raw bytes between the quotes, still escaped)
	TokEnum              // .FOO. (Text = label without the dots)
	TokBool              // .T. / .F. / .U. (Text = T/F/U)
	TokBinary            // "0..." (Text = bytes between the quotes)
	TokInt               // 123 / -4 / +7
	TokFloat             // 1. / -2.5E-3
	TokKeyword           // IFCWALL, ISO-10303-21, HEADER, DATA, ENDSEC ...
)

// String returns the token kind name, so kinds render readably in error messages.
func (k TokenKind) String() string {
	switch k {
	case TokEOF:
		return "EOF"
	case TokLParen:
		return "'('"
	case TokRParen:
		return "')'"
	case TokComma:
		return "','"
	case TokEquals:
		return "'='"
	case TokSemi:
		return "';'"
	case TokDollar:
		return "'$'"
	case TokStar:
		return "'*'"
	case TokRef:
		return "reference"
	case TokString:
		return "string"
	case TokEnum:
		return "enumeration"
	case TokBool:
		return "boolean"
	case TokBinary:
		return "binary"
	case TokInt:
		return "integer"
	case TokFloat:
		return "real"
	case TokKeyword:
		return "keyword"
	default:
		return "TokenKind(" + strconv.Itoa(int(k)) + ")"
	}
}

// Token is one lexical unit. Text is a sub-slice of the source buffer (zero-copy);
// it is only meaningful for the value-bearing kinds (Ref, String, Enum, Bool,
// Binary, Int, Float, Keyword) — for operators it holds the operator byte(s).
type Token struct {
	Kind TokenKind
	Text []byte
}

// Scanner is a character-stream lexer over an in-memory STEP source buffer. It is
// deliberately not line-based: STEP permits a single entity record to span many
// lines, so tokenization walks the byte stream directly.
type Scanner struct {
	src []byte
	pos int
}

// NewScanner returns a Scanner over src. src is retained (not copied); Token.Text
// values alias it, so callers must not mutate src while scanning.
func NewScanner(src []byte) *Scanner {
	return &Scanner{src: src}
}

// Pos reports the scanner's current byte offset into the source, used to attach
// positions to parse errors.
func (s *Scanner) Pos() int { return s.pos }

// isDelim reports whether c terminates an unquoted token (keyword or enum) — a
// structural operator or whitespace. Shared by scanKeyword and scanEnum so the two
// scanners cannot drift out of sync.
func isDelim(c byte) bool {
	switch c {
	case '(', ')', '=', ',', ';', ' ', '\t', '\r', '\n':
		return true
	}
	return false
}

// Next returns the next token, or a Token with Kind==TokEOF once the input is
// exhausted (repeatedly). It never panics on malformed input; an unterminated
// string or binary literal yields a token up to end-of-input for the parser layer
// to reject.
func (s *Scanner) Next() Token {
	s.skipTrivia()
	if s.pos >= len(s.src) {
		return Token{Kind: TokEOF}
	}
	c := s.src[s.pos]
	switch c {
	case '(':
		return s.op(TokLParen)
	case ')':
		return s.op(TokRParen)
	case ',':
		return s.op(TokComma)
	case '=':
		return s.op(TokEquals)
	case ';':
		return s.op(TokSemi)
	case '$':
		return s.op(TokDollar)
	case '*':
		return s.op(TokStar)
	case '#':
		return s.scanRef()
	case '\'':
		return s.scanString()
	case '"':
		return s.scanBinary()
	case '.':
		// A leading-dot real (.5, -.25 handled via the sign path) is non-conformant
		// per ISO 10303-21 but some exporters emit it. Route ".<digit>" to the
		// number scanner so it is not silently swallowed as an enumeration.
		if s.pos+1 < len(s.src) && s.src[s.pos+1] >= '0' && s.src[s.pos+1] <= '9' {
			return s.scanNumber()
		}
		return s.scanEnum()
	}
	if c == '+' || c == '-' || (c >= '0' && c <= '9') {
		return s.scanNumber()
	}
	return s.scanKeyword()
}

// op consumes a single-character operator token.
func (s *Scanner) op(k TokenKind) Token {
	t := Token{Kind: k, Text: s.src[s.pos : s.pos+1]}
	s.pos++
	return t
}

// skipTrivia advances past whitespace and /* ... */ comments (which may nest STEP
// content but never nest each other in ISO 10303-21).
func (s *Scanner) skipTrivia() {
	for s.pos < len(s.src) {
		c := s.src[s.pos]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			s.pos++
			continue
		}
		if c == '/' && s.pos+1 < len(s.src) && s.src[s.pos+1] == '*' {
			s.pos += 2
			for s.pos+1 < len(s.src) && (s.src[s.pos] != '*' || s.src[s.pos+1] != '/') {
				s.pos++
			}
			if s.pos+1 < len(s.src) {
				s.pos += 2 // consume closing */
			} else {
				s.pos = len(s.src) // unterminated comment: consume to EOF
			}
			continue
		}
		return
	}
}

func (s *Scanner) scanRef() Token {
	s.pos++ // consume '#'
	start := s.pos
	for s.pos < len(s.src) && s.src[s.pos] >= '0' && s.src[s.pos] <= '9' {
		s.pos++
	}
	return Token{Kind: TokRef, Text: s.src[start:s.pos]}
}

// scanString consumes a '...' literal. A doubled apostrophe (”) is an escaped
// quote and stays inside the token; the closing quote is a single ' not followed
// by another '. Escape sequences like \X2\..\X0\ are copied verbatim (decoded
// later). Text excludes the surrounding quotes.
func (s *Scanner) scanString() Token {
	s.pos++ // consume opening '
	start := s.pos
	for s.pos < len(s.src) {
		if s.src[s.pos] == '\'' {
			if s.pos+1 < len(s.src) && s.src[s.pos+1] == '\'' {
				s.pos += 2 // escaped '' — keep scanning
				continue
			}
			t := Token{Kind: TokString, Text: s.src[start:s.pos]}
			s.pos++ // consume closing '
			return t
		}
		s.pos++
	}
	// unterminated: return what we have; parser layer rejects on EOF
	return Token{Kind: TokString, Text: s.src[start:s.pos]}
}

func (s *Scanner) scanBinary() Token {
	s.pos++ // consume opening "
	start := s.pos
	for s.pos < len(s.src) && s.src[s.pos] != '"' {
		s.pos++
	}
	t := Token{Kind: TokBinary, Text: s.src[start:s.pos]}
	if s.pos < len(s.src) {
		s.pos++ // consume closing "
	}
	return t
}

// scanEnum consumes a .LABEL. token. .T./.F./.U. classify as TokBool. The scan is
// bounded by structural delimiters so a malformed, unterminated enum cannot swallow
// subsequent tokens (it stops at the first delimiter rather than running to EOF).
func (s *Scanner) scanEnum() Token {
	s.pos++ // consume opening .
	start := s.pos
	for s.pos < len(s.src) {
		c := s.src[s.pos]
		if c == '.' {
			break
		}
		if isDelim(c) {
			break // unterminated enum: stop, do not consume the delimiter
		}
		s.pos++
	}
	label := s.src[start:s.pos]
	if s.pos < len(s.src) && s.src[s.pos] == '.' {
		s.pos++ // consume closing . (only if present — a delimiter is left for Next)
	}
	if len(label) == 1 && (label[0] == 'T' || label[0] == 'F' || label[0] == 'U') {
		return Token{Kind: TokBool, Text: label}
	}
	return Token{Kind: TokEnum, Text: label}
}

// scanNumber consumes an integer or real. A '.' or exponent marker makes it a float.
func (s *Scanner) scanNumber() Token {
	start := s.pos
	if s.src[s.pos] == '+' || s.src[s.pos] == '-' {
		s.pos++
	}
	isFloat := false
	for s.pos < len(s.src) {
		c := s.src[s.pos]
		switch {
		case c >= '0' && c <= '9':
			s.pos++
		case c == '.':
			isFloat = true
			s.pos++
		case c == 'e' || c == 'E':
			isFloat = true
			s.pos++
			if s.pos < len(s.src) && (s.src[s.pos] == '+' || s.src[s.pos] == '-') {
				s.pos++
			}
		default:
			goto done
		}
	}
done:
	kind := TokInt
	if isFloat {
		kind = TokFloat
	}
	return Token{Kind: kind, Text: s.src[start:s.pos]}
}

// scanKeyword consumes an unquoted identifier: entity type names (IFCWALL),
// section markers (HEADER, DATA, ENDSEC), and hyphenated markers (ISO-10303-21,
// END-ISO-10303-21). Terminated by whitespace or a structural delimiter.
func (s *Scanner) scanKeyword() Token {
	start := s.pos
	for s.pos < len(s.src) {
		if isDelim(s.src[s.pos]) {
			break
		}
		s.pos++
	}
	return Token{Kind: TokKeyword, Text: s.src[start:s.pos]}
}
