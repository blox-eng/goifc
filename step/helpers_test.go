package step

import (
	"os"
	"path/filepath"
	"testing"
)

// readTestdata loads a fixture file from testdata/ and fails the test if absent.
func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read testdata %s: %v", name, err)
	}
	return b
}
