package step

import (
	"errors"
	"testing"
)

// Coverage added from the /review pass: the library-polish API surface (ParseError,
// Traverse BreadthFirst, the Stringers, ByID bounds) that the feature tests didn't
// exercise.

func TestParseError_AsAndOffset(t *testing.T) {
	src := []byte("ISO-10303-21;\nDATA;\n#1= IFCWALL('oops);\nENDSEC;")
	_, err := ParseBytes(src)
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("want *ParseError, got %T: %v", err, err)
	}
	if pe.Offset <= 0 || pe.Offset > len(src) {
		t.Fatalf("Offset = %d, want a byte position within [1,%d]", pe.Offset, len(src))
	}
	if pe.Unwrap() == nil {
		t.Fatal("Unwrap returned nil")
	}
}

func TestTraverse_BreadthFirst(t *testing.T) {
	// #1 -> #4 (long path to #2) and #1 -> #2 (direct, depth 1).
	src := "ISO-10303-21;\nDATA;\n" +
		"#1= IFCX(#4,#2);\n#4= IFCX(#3,$);\n#3= IFCX(#2,$);\n#2= IFCX($,$);\n" +
		"ENDSEC;\nEND-ISO-10303-21;"
	f, err := ParseBytes([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	one, _ := f.ByID(1)

	// bounded BFS at depth 1 includes the directly-reachable #2, excludes #3 (depth 2)
	seen := map[int]bool{}
	for _, inst := range f.Traverse(one, 1, BreadthFirst) {
		seen[inst.ID()] = true
	}
	if !seen[2] || seen[3] {
		t.Fatalf("BFS depth-1 from #1 = %v, want #2 included and #3 excluded", seen)
	}

	// unbounded BFS and DFS reach the same closure
	bf := f.Traverse(one, Unbounded, BreadthFirst)
	df := f.Traverse(one, Unbounded, DepthFirst)
	if len(bf) != len(df) {
		t.Fatalf("BFS closure %d != DFS closure %d", len(bf), len(df))
	}
}

func TestKind_String(t *testing.T) {
	all := []Kind{KindNull, KindDerived, KindInt, KindFloat, KindString, KindEnum,
		KindBool, KindLogical, KindBinary, KindRef, KindList, KindTyped}
	for _, k := range all {
		if k.String() == "" {
			t.Fatalf("Kind(%d).String() is empty", k)
		}
	}
	if got := Kind(255).String(); got == "" {
		t.Fatal("out-of-range Kind.String() should return a non-empty fallback")
	}
}

func TestTokenKind_String(t *testing.T) {
	all := []TokenKind{TokEOF, TokLParen, TokRParen, TokComma, TokEquals, TokSemi,
		TokDollar, TokStar, TokRef, TokString, TokEnum, TokBool, TokBinary, TokInt,
		TokFloat, TokKeyword}
	for _, k := range all {
		if k.String() == "" {
			t.Fatalf("TokenKind(%d).String() is empty", k)
		}
	}
	if got := TokenKind(255).String(); got == "" {
		t.Fatal("out-of-range TokenKind.String() should return a non-empty fallback")
	}
}

func TestByID_OutOfRange(t *testing.T) {
	f, err := ParseBytes(readTestdata(t, "minimal.ifc"))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{-1, 0, 1 << 40} {
		if _, ok := f.ByID(id); ok {
			t.Fatalf("ByID(%d) = ok, want not found", id)
		}
	}
}
