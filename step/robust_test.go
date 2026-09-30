package step

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
)

func TestParse_MultilineRecord(t *testing.T) {
	f, err := ParseBytes(readTestdata(t, "multiline.ifc"))
	if err != nil {
		t.Fatal(err)
	}
	if f.Len() != 2 {
		t.Fatalf("instances %d want 2", f.Len())
	}
	p, _ := f.ByID(1)
	a0, _ := p.Get(0)
	if a0.kind != KindList || len(a0.List()) != 3 || a0.List()[2].Float() != 0. {
		t.Fatalf("multiline nested list mis-parsed: %+v", a0)
	}
}

func TestParse_CommentBetweenAttrs(t *testing.T) {
	f, err := ParseBytes(readTestdata(t, "comment_in_data.ifc"))
	if err != nil {
		t.Fatal(err)
	}
	w, _ := f.ByID(1)
	if w.Len() != 8 {
		t.Fatalf("wall args %d want 8", w.Len())
	}
	a7, _ := w.Get(7)
	if a7.kind != KindString || a7.Str() != "tag" {
		t.Fatalf("comment between attrs broke parse: %+v", a7)
	}
}

func TestParse_ErrorPaths(t *testing.T) {
	// unterminated string -> hard error, not panic
	if _, err := ParseBytes([]byte("ISO-10303-21;\nDATA;\n#1= IFCWALL('oops);\nENDSEC;")); err == nil {
		t.Fatal("want error for unterminated string")
	}
	// EOF mid-record -> hard error, not panic
	if _, err := ParseBytes([]byte("ISO-10303-21;\nDATA;\n#1= IFCWALL('g',$,")); err == nil {
		t.Fatal("want error for EOF mid-record")
	}
	// malformed \X2\ hex length -> decode error surfaces
	if _, err := decodeString([]byte(`\X2\041\X0\`)); err == nil {
		t.Fatal("want decode error for bad \\X2\\ hex length")
	}
}

func TestParse_MissingRefNonFatal(t *testing.T) {
	f, err := ParseBytes([]byte("ISO-10303-21;\nDATA;\n#1= IFCWALL('g',$,$,$,$,#999,$,$);\nENDSEC;\nEND-ISO-10303-21;"))
	if err != nil {
		t.Fatalf("dangling ref must not be fatal: %v", err)
	}
	if len(f.Warnings()) == 0 || !strings.Contains(f.Warnings()[0], "999") {
		t.Fatalf("expected dangling-ref warning, got %v", f.Warnings())
	}
}

// Every way a record can be malformed fails with its own message, never a
// panic, and the parallel parse declines it rather than guess.
func TestParse_MalformedRecords(t *testing.T) {
	const head = "ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\n"
	for _, c := range []struct{ name, src, want string }{
		{"header without (", "ISO-10303-21;\nHEADER;\nFILE_SCHEMA 'x';", "expected '(' after header keyword"},
		{"header bad value", "ISO-10303-21;\nHEADER;\nFILE_SCHEMA((=));", "unexpected token"},
		{"header without ;", "ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'))\nENDSEC;", "expected ';' after header record"},
		{"stray ) at top level", "ISO-10303-21;\n);", "unexpected token"},
		{"DATA without ;", head + "DATA\n#1=IFCWALL();\nENDSEC;", "expected ';' after DATA"},
		{"keyword in DATA", head + "DATA;\nFOO;\nENDSEC;", "unexpected keyword"},
		{"EOF in DATA", head + "DATA;\n#1=IFCWALL();\n", "unexpected EOF in DATA section"},
		{"id too long", head + "DATA;\n#99999999999=IFCWALL();\nENDSEC;", "bad instance id"},
		{"id past uint32", head + "DATA;\n#4294967296=IFCWALL();\nENDSEC;", "bad instance id"},
		{"no =", head + "DATA;\n#1 IFCWALL();\nENDSEC;", "expected '=' after #1"},
		{"no type", head + "DATA;\n#1=$;\nENDSEC;", "expected entity keyword or '('"},
		{"simple without (", head + "DATA;\n#1=IFCWALL;\nENDSEC;", "expected '(' for #1"},
		{"simple without ;", head + "DATA;\n#1=IFCWALL()\n#2=IFCWALL();\nENDSEC;", "expected ';' after #1"},
		{"complex part not a type", head + "DATA;\n#1=($);\nENDSEC;", "expected part type in complex #1"},
		{"complex part without (", head + "DATA;\n#1=(IFCA IFCB());\nENDSEC;", "expected '(' after IFCA"},
		{"complex part bad value", head + "DATA;\n#1=(IFCA(=));\nENDSEC;", "complex #1 IFCA"},
		{"empty complex", head + "DATA;\n#1=();\nENDSEC;", "empty complex instance #1"},
		{"complex without ;", head + "DATA;\n#1=(IFCA())\nENDSEC;", "expected ';' after complex #1"},
		{"ref past uint32", head + "DATA;\n#1=IFCWALL(#4294967296);\nENDSEC;", "bad ref"},
		{"integer past int64", head + "DATA;\n#1=IFCWALL(99999999999999999999);\nENDSEC;", "bad integer"},
		{"real past float64", head + "DATA;\n#1=IFCWALL(1.0E999);\nENDSEC;", "bad real"},
		{"bad escape", head + "DATA;\n#1=IFCWALL('\\X2\\041\\X0\\');\nENDSEC;", "bad hex digit"},
		{"bad value in nested list", head + "DATA;\n#1=IFCWALL((1,=));\nENDSEC;", "unexpected token"},
		{"typed value without (", head + "DATA;\n#1=IFCWALL(IFCLABEL 'x');\nENDSEC;", "expected '(' after typed value"},
		{"bad value in typed value", head + "DATA;\n#1=IFCWALL(IFCLABEL(=));\nENDSEC;", "unexpected token"},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := []byte(c.src)
			_, err := ParseBytes(src)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v; want one containing %q", err, c.want)
			}
			if _, ok := parseParallelN(src, 4, 8); ok {
				t.Fatal("the parallel parse accepted a malformed file")
			}
		})
	}
}

func TestParseUint32(t *testing.T) {
	for _, c := range []struct {
		in   string
		want uint32
		ok   bool
	}{
		{"0", 0, true},
		{"4294967295", 4294967295, true},
		{"4294967296", 0, false},
		{"99999999999", 0, false},
		{"", 0, false},
		{"12a", 0, false},
	} {
		got, err := parseUint32([]byte(c.in))
		if (err == nil) != c.ok || (c.ok && got != c.want) {
			t.Errorf("parseUint32(%q) = %d, %v; want %d, ok=%v", c.in, got, err, c.want, c.ok)
		}
	}
}

func TestParse_ReadErrors(t *testing.T) {
	if _, err := ParseFile(filepath.Join(t.TempDir(), "missing.ifc")); err == nil {
		t.Fatal("ParseFile of a missing file: want an error")
	}
	if _, err := Parse(iotest.ErrReader(errors.New("boom"))); err == nil || err.Error() != "boom" {
		t.Fatalf("Parse of a failing reader: err = %v; want boom", err)
	}
}

// Header records in the shapes real exporters write: nested and non-string
// description members, a bare schema string, and a schema that is no string.
func TestParse_HeaderShapes(t *testing.T) {
	f, err := ParseBytes([]byte("ISO-10303-21;\nHEADER;\nFILE_DESCRIPTION(('a',('b','c'),$),'2;1');\nFILE_SCHEMA('IFC4');\nENDSEC;\nDATA;\nENDSEC;\nEND-ISO-10303-21;"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.Head.Description, "|"); got != "a|b|c|" {
		t.Errorf("Description = %q; want a|b|c|", got)
	}
	if f.SchemaID() != "IFC4" {
		t.Errorf("SchemaID = %q; want IFC4", f.SchemaID())
	}
	f, err = ParseBytes([]byte("ISO-10303-21;\nHEADER;\nFILE_SCHEMA(5);\nENDSEC;\nEND-ISO-10303-21;"))
	if err != nil {
		t.Fatal(err)
	}
	if f.SchemaID() != "" {
		t.Errorf("SchemaID = %q; want none", f.SchemaID())
	}
}

// Ids too far apart for the dense index, a type spelled two ways, and a
// referrer that names its target twice.
func TestParse_IndexEdges(t *testing.T) {
	f, err := ParseBytes([]byte("ISO-10303-21;\nDATA;\n#1=IFCWALL(#100000000,#100000000);\n#100000000=IfcWall();\nENDSEC;\nEND-ISO-10303-21;"))
	if err != nil {
		t.Fatal(err)
	}
	if f.sparse == nil {
		t.Fatal("ids 1 and 100000000 built a dense index")
	}
	if _, ok := f.ByID(5); ok {
		t.Error("ByID(5) found an instance that does not exist")
	}
	if n := len(f.ByType("IFCWALL")); n != 2 {
		t.Errorf("ByType(IFCWALL) = %d instances; want 2 (one spelled IfcWall)", n)
	}
	target, _ := f.ByID(100000000)
	if n := len(f.InverseIndices(target)); n != 2 {
		t.Errorf("InverseIndices = %d; want 2", n)
	}
	if n := len(f.Inverse(target)); n != 1 {
		t.Errorf("Inverse = %d referrers; want 1", n)
	}
	if f.InverseIndices(&Instance{id: 7}) != nil {
		t.Error("InverseIndices of an instance from no file: want nil")
	}
	var zero Instance
	if zero.Args() != nil {
		t.Error("zero Instance Args: want nil")
	}
}
