package step

import (
	"strings"
	"testing"
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
	if a0.Kind != KindList || len(a0.List) != 3 || a0.List[2].F != 0. {
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
	if a7.Kind != KindString || a7.Str != "tag" {
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
