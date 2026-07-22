package step

import "testing"

func kinds(src string) []TokenKind {
	s := NewScanner([]byte(src))
	var out []TokenKind
	for {
		t := s.Next()
		if t.Kind == TokEOF {
			return out
		}
		out = append(out, t.Kind)
	}
}

func TestScanner_Constructs(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []TokenKind
	}{
		{"ref+keyword+parens", "#33= IFCOWNERHISTORY(#28,#32);", []TokenKind{
			TokRef, TokEquals, TokKeyword, TokLParen, TokRef, TokComma, TokRef, TokRParen, TokSemi}},
		{"enum+dollar+star", "(*,.LENGTHUNIT.,$)", []TokenKind{
			TokLParen, TokStar, TokComma, TokEnum, TokComma, TokDollar, TokRParen}},
		{"bool", "(.T.,.F.,.U.)", []TokenKind{
			TokLParen, TokBool, TokComma, TokBool, TokComma, TokBool, TokRParen}},
		{"numbers", "(1,-4,1.,-2.5E-3,+7)", []TokenKind{
			TokLParen, TokInt, TokComma, TokInt, TokComma, TokFloat, TokComma, TokFloat, TokComma, TokInt, TokRParen}},
		{"string with escapes stays one token", `'a''b\X2\0410\X0\'`, []TokenKind{TokString}},
		{"comment skipped", "/* hi */ #1=A();", []TokenKind{
			TokRef, TokEquals, TokKeyword, TokLParen, TokRParen, TokSemi}},
		{"binary", `"012 AF"`, []TokenKind{TokBinary}},
		{"schema keyword with dashes", "ISO-10303-21;", []TokenKind{TokKeyword, TokSemi}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := kinds(c.src)
			if len(got) != len(c.want) {
				t.Fatalf("len=%d want %d (%v)", len(got), len(c.want), got)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("tok %d = %v want %v", i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestScanner_TokenText(t *testing.T) {
	s := NewScanner([]byte("#926049= IFCWALL(.T.,'x''y');"))
	exp := []struct {
		k TokenKind
		s string
	}{{TokRef, "926049"}, {TokEquals, "="}, {TokKeyword, "IFCWALL"}, {TokLParen, "("},
		{TokBool, "T"}, {TokComma, ","}, {TokString, "x''y"}, {TokRParen, ")"}, {TokSemi, ";"}}
	for i, e := range exp {
		tok := s.Next()
		if tok.Kind != e.k || string(tok.Text) != e.s {
			t.Fatalf("tok %d = (%v,%q) want (%v,%q)", i, tok.Kind, tok.Text, e.k, e.s)
		}
	}
}
