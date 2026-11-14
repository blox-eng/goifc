package wasmapi

import (
	"bytes"
	"flag"
	"os"
	"testing"
)

var update = flag.Bool("update", false, "rewrite internal/wasmapi/testdata golden files")

// testVersion stands in for the goifc version in every golden answer.
const testVersion = "v0.0.0-test"

// golden compares got with the file at path, or rewrites it under -update.
func golden(t *testing.T, path string, got []byte) {
	t.Helper()
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from the golden file (run with -update after checking the change)", path)
	}
}
