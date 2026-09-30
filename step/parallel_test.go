package step

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// dump renders everything a File exposes, so two parses can be compared whole.
func dump(f *File) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%q %d %q\n", f.Head, f.Len(), f.Warnings())
	var val func(Value)
	val = func(v Value) {
		fmt.Fprintf(&b, "%d:%q/%d/%v/%d/%v/%d[", v.kind, v.Str(), v.RefID(), v.Ref() != nil, v.Int(), v.Float(), bool2int(v.Bool()))
		for _, c := range v.List() {
			val(c)
		}
		b.WriteString("]")
	}
	for inst := range f.All() {
		fmt.Fprintf(&b, "#%d=%s", inst.ID(), inst.Type())
		for _, a := range inst.Args() {
			val(a)
		}
		for _, r := range f.InverseIndices(inst) {
			fmt.Fprintf(&b, "<%d.%d", r.From.ID(), r.AttrIndex)
		}
		b.WriteString("\n")
	}
	for _, t := range []string{"IFCWALL", "IFCLABEL", "IFCA", "IFCB", "IFCSLAB"} {
		fmt.Fprintf(&b, "%s:", t)
		for _, inst := range f.ByType(t) {
			fmt.Fprintf(&b, "%d,", inst.ID())
		}
	}
	return b.String()
}

func bool2int(b bool) int {
	if b {
		return 1
	}
	return 0
}

// bigFile builds a DATA section of at least n bytes of well-formed records:
// multi-line records, nested and typed lists, complex instances, duplicate ids
// and dangling references. With traps set, most records also hold ";#" inside
// a string or a comment, where a split guess can land.
func bigFile(n int, traps bool) []byte {
	var b strings.Builder
	b.WriteString("ISO-10303-21;\nHEADER;\nFILE_DESCRIPTION(('x'),'2;1');\nFILE_NAME('a','b',('c'),('d'),'e','f','g');\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n")
	for i := 1; b.Len() < n; i += 6 {
		fmt.Fprintf(&b, "#%d=IFCWALL('g%d',#%d,$,*,.T.,.U.,(1,2.5,-3.E-2),IFCLABEL('l'),((#%d,#%d),()));\n", i, i, i+1, i+2, i+99999999)
		fmt.Fprintf(&b, "#%d=IFCSLAB(\n'multi-line',\n#%d);\n", i+1, i)
		fmt.Fprintf(&b, "#%d=(IFCA(#%d)IFCB('\\X2\\00E9\\X0\\',.ENUM.));\n", i+2, i+1)
		fmt.Fprintf(&b, "#%d=IFCWALL(#%d,\"0FF\");#%d=IFCWALL(#%d);\n", i+3, i+2, i+4, i+3)
		if traps {
			fmt.Fprintf(&b, "#%d=IFCSLAB('line\n;\n#%d=IFCSLAB($);\n');\n", i+5, i+3)
			fmt.Fprintf(&b, "/* comment ;\n#%d=IFCWALL(); */\n", i+4)
		}
		if i%601 == 1 {
			fmt.Fprintf(&b, "#%d=IFCSLAB('duplicate id');\n", i)
		}
	}
	b.WriteString("ENDSEC;\nEND-ISO-10303-21;\n")
	return []byte(b.String())
}

func TestParseParallelMatchesSerial(t *testing.T) {
	src := bigFile(6*parallelChunk, false)
	ser, err := parseSerial(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, workers := range []int{2, 3, 5, 16} {
		par, ok := parseParallelN(src, workers, parallelChunk)
		if !ok {
			t.Fatalf("%d workers: declined a well-formed file", workers)
		}
		if len(par.slabs) < 3 {
			t.Fatalf("%d workers: parsed in %d slabs; want a real split", workers, len(par.slabs))
		}
		if dump(par) != dump(ser) {
			t.Fatalf("%d workers: parallel and serial parses differ", workers)
		}
	}
}

// A file dense with ";#" inside strings and comments: whether a given split
// verifies depends on where the guesses land, but the result must be the
// serial one either way.
func TestParseParallelTrapsMatchSerial(t *testing.T) {
	src := bigFile(6*parallelChunk, true)
	ser, err := parseSerial(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, workers := range []int{2, 3, 5, 16} {
		if par, ok := parseParallelN(src, workers, parallelChunk); ok && dump(par) != dump(ser) {
			t.Fatalf("%d workers: parallel and serial parses differ", workers)
		}
	}
	f, err := ParseBytes(src)
	if err != nil {
		t.Fatal(err)
	}
	if dump(f) != dump(ser) {
		t.Fatal("ParseBytes and the serial parse differ")
	}
}

// Every candidate boundary sits inside a string, so no split can verify and
// parseParallel must decline rather than join chunks cut mid-record.
func TestParseParallelDeclinesSplitsInsideStrings(t *testing.T) {
	var b strings.Builder
	b.WriteString("ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n#1=IFCWALL('")
	for b.Len() < 4*parallelChunk {
		b.WriteString(";\n#7=IFCWALL($);\n")
	}
	b.WriteString("');\nENDSEC;\nEND-ISO-10303-21;\n")
	src := []byte(b.String())
	if _, ok := parseParallelN(src, 8, parallelChunk); ok {
		t.Fatal("parseParallel joined chunks split inside a string")
	}
	f, err := ParseBytes(src)
	if err != nil {
		t.Fatal(err)
	}
	if f.Len() != 1 {
		t.Fatalf("Len = %d; want the one instance holding the string", f.Len())
	}
}

func TestParseParallelErrorsMatchSerial(t *testing.T) {
	src := bigFile(4*parallelChunk, false)
	mid := len(src)/2 + strings.Index(string(src[len(src)/2:]), "IFCWALL(") + len("IFCWALL")
	src[mid] = '='
	_, perr := ParseBytes(src)
	_, serr := parseSerial(src)
	var pe, se *ParseError
	if !errors.As(perr, &pe) || !errors.As(serr, &se) {
		t.Fatalf("want ParseErrors, got %v and %v", perr, serr)
	}
	if pe.Offset != se.Offset || pe.Error() != se.Error() {
		t.Fatalf("ParseBytes error %v at %d; serial %v at %d", pe, pe.Offset, se, se.Offset)
	}
}
