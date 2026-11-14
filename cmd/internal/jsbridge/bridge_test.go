//go:build js && wasm

package jsbridge

import (
	"slices"
	"strings"
	"syscall/js"
	"testing"
)

func TestInclude(t *testing.T) {
	array := func(items ...any) js.Value { return js.ValueOf(items) }
	for _, c := range []struct {
		name string
		v    js.Value
		want []string
	}{
		{"undefined means the default", js.Undefined(), nil},
		{"null means the default", js.Null(), nil},
		{"an array of names", array("nodes", "psets"), []string{"nodes", "psets"}},
		{"an empty array stays empty", array(), []string{}},
		{"a string", js.ValueOf("psets"), []string{notParts}},
		{"an object", js.ValueOf(map[string]any{}), []string{notParts}},
		{"an array holding a number", array("nodes", 1), []string{notParts}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := Include(c.v)
			if (got == nil) != (c.want == nil) || !slices.Equal(got, c.want) {
				t.Errorf("Include = %q, want %q", got, c.want)
			}
		})
	}
}

func TestGuard(t *testing.T) {
	out := js.ValueOf(Guard("v0.0.0-test", func() any { panic("boom") }))
	answer := out.Get("answer").String()
	if !strings.Contains(answer, `"ok":false`) || !strings.Contains(answer, `"code":"invalid_input"`) || !strings.Contains(answer, "boom") {
		t.Errorf("answer %s, want an invalid_input refusal naming the panic", answer)
	}
	if !out.Get("glb").IsNull() {
		t.Error("a GLB came back from a panic")
	}
}
