package step

import "testing"

// args parses a bare "(...)" argument list from src (after locating the '(').
func argsOf(t *testing.T, src string) []Value {
	t.Helper()
	s := NewScanner([]byte(src))
	tok := s.Next()
	if tok.Kind != TokLParen {
		t.Fatalf("want '(' got %v", tok.Kind)
	}
	vs, err := parseArgs(s)
	if err != nil {
		t.Fatal(err)
	}
	return vs
}

func TestParseArgs_Scalars(t *testing.T) {
	vs := argsOf(t, "(.MILLI.,$,*,#28,'x',1,2.5,.T.)")
	want := []Kind{KindEnum, KindNull, KindDerived, KindRef, KindString, KindInt, KindFloat, KindBool}
	if len(vs) != len(want) {
		t.Fatalf("len %d want %d (%+v)", len(vs), len(want), vs)
	}
	for i := range vs {
		if vs[i].Kind != want[i] {
			t.Fatalf("arg %d kind %v want %v", i, vs[i].Kind, want[i])
		}
	}
	if vs[0].Str != "MILLI" {
		t.Fatalf("enum label %q", vs[0].Str)
	}
	if vs[3].RefID != 28 {
		t.Fatalf("ref %d", vs[3].RefID)
	}
	if vs[4].Str != "x" || vs[5].I != 1 || vs[6].F != 2.5 || vs[7].B != true {
		t.Fatalf("scalar payloads wrong: %+v", vs)
	}
}

func TestParseArgs_NestedAndTyped(t *testing.T) {
	vs := argsOf(t, "((0.,0.,0.),IFCLABEL('n'),(#1,#2))")
	if vs[0].Kind != KindList || len(vs[0].List) != 3 || vs[0].List[0].F != 0. {
		t.Fatalf("nested list wrong: %+v", vs[0])
	}
	if vs[1].Kind != KindTyped || vs[1].Str != "IFCLABEL" || vs[1].List[0].Str != "n" {
		t.Fatalf("typed wrong: %+v", vs[1])
	}
	if vs[2].Kind != KindList || vs[2].List[1].RefID != 2 {
		t.Fatalf("ref list wrong: %+v", vs[2])
	}
}

func TestParseArgs_LogicalUnknown(t *testing.T) {
	// EXPRESS LOGICAL: .T./.F. are booleans, .U. is "unknown" (distinct from false).
	vs := argsOf(t, "(.T.,.F.,.U.)")
	if vs[0].Kind != KindBool || vs[0].B != true {
		t.Fatalf(".T. = %+v want bool true", vs[0])
	}
	if vs[1].Kind != KindBool || vs[1].B != false {
		t.Fatalf(".F. = %+v want bool false", vs[1])
	}
	if vs[2].Kind != KindLogical {
		t.Fatalf(".U. = %+v want KindLogical (not bool false)", vs[2])
	}
}

func TestValue_Walk(t *testing.T) {
	vs := argsOf(t, "((#1,#2),IFCLABEL('n'))")
	var refs int
	for _, v := range vs {
		v.Walk(func(x Value) {
			if x.Kind == KindRef {
				refs++
			}
		})
	}
	if refs != 2 {
		t.Fatalf("walk found %d refs want 2", refs)
	}
}
