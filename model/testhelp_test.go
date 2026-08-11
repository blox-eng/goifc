package model

import (
	"os"
	"strings"
	"testing"

	"github.com/blox-eng/goifc/step"
)

func parseString(t *testing.T, spf string) *step.File {
	t.Helper()
	f, err := step.Parse(strings.NewReader(spf))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return f
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
