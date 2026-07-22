package step

import "testing"

func TestDecodeString(t *testing.T) {
	cases := []struct{ in, want string }{
		{`plain`, "plain"},
		{`a''b`, "a'b"},                         // doubled apostrophe -> one quote
		{`\X2\0410\X0\`, "А"},                   // UTF-16BE 0x0410 = Cyrillic A
		{`\X2\04100411\X0\`, "АБ"},              // two UTF-16 code units
		{`ул. \X2\0421043E0444\X0\`, "ул. Соф"}, // ASCII-ish prefix + \X2\ run
		{`\X\C4`, "Ä"},                          // single byte 0xC4 (Latin-1) = U+00C4
		{`\S\A`, "Á"},                           // 'A'(0x41)+0x80 = 0xC1 = U+00C1
		{`a\X2\0410\X0\b`, "aАb"},               // \X2\ run bounded on both sides
	}
	for _, c := range cases {
		got, err := decodeString([]byte(c.in))
		if err != nil {
			t.Fatalf("%q: unexpected err %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("decode(%q) = %q want %q", c.in, got, c.want)
		}
	}
}

func TestDecodeString_Errors(t *testing.T) {
	bad := []string{
		`\X2\041\X0\`, // 3 hex digits: not a multiple of 4
		`\X2\0410`,    // unterminated \X2\ run (no \X0\)
		`\X\C`,        // \X\ needs 2 hex digits
	}
	for _, in := range bad {
		if _, err := decodeString([]byte(in)); err == nil {
			t.Fatalf("decode(%q) = nil err, want error", in)
		}
	}
}
