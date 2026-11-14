package wasmapi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const schemaPath = "../../wasm/goifc-contract.schema.json"

// validator checks a JSON value against the subset of JSON Schema the
// contract file uses. A keyword it does not know fails the check, so the
// schema cannot use one this test would skip.
type validator struct{ defs map[string]any }

var schemaAnnotations = []string{"$schema", "$id", "title", "description", "$defs"}

func (v validator) check(schema any, x any, path string) error {
	s, ok := schema.(map[string]any)
	if !ok {
		return fmt.Errorf("%s: schema is not an object", path)
	}
	for k := range s {
		switch k {
		case "$ref", "type", "properties", "required", "additionalProperties", "items",
			"minItems", "maxItems", "enum", "const", "pattern", "oneOf":
		default:
			if !slices.Contains(schemaAnnotations, k) {
				return fmt.Errorf("%s: the test validator does not know keyword %q", path, k)
			}
		}
	}
	if ref, ok := s["$ref"].(string); ok {
		name, found := strings.CutPrefix(ref, "#/$defs/")
		def, ok := v.defs[name]
		if !found || !ok {
			return fmt.Errorf("%s: unknown $ref %q", path, ref)
		}
		return v.check(def, x, path)
	}
	if t, ok := s["type"]; ok {
		var types []string
		switch t := t.(type) {
		case string:
			types = []string{t}
		case []any:
			for _, e := range t {
				types = append(types, e.(string))
			}
		}
		if !slices.ContainsFunc(types, func(t string) bool { return typeIs(t, x) }) {
			return fmt.Errorf("%s: %v is not of type %v", path, x, types)
		}
	}
	if e, ok := s["enum"].([]any); ok && !slices.Contains(e, x) {
		return fmt.Errorf("%s: %v is not one of %v", path, x, e)
	}
	if c, ok := s["const"]; ok && c != x {
		return fmt.Errorf("%s: %v is not %v", path, x, c)
	}
	if p, ok := s["pattern"].(string); ok {
		if str, ok := x.(string); ok && !regexp.MustCompile(p).MatchString(str) {
			return fmt.Errorf("%s: %q does not match %s", path, str, p)
		}
	}
	if one, ok := s["oneOf"].([]any); ok {
		n, last := 0, error(nil)
		for _, sub := range one {
			if err := v.check(sub, x, path); err == nil {
				n++
			} else {
				last = err
			}
		}
		if n != 1 {
			return fmt.Errorf("%s: matches %d of oneOf, want 1 (last mismatch: %v)", path, n, last)
		}
	}
	if obj, ok := x.(map[string]any); ok {
		props, _ := s["properties"].(map[string]any)
		if req, ok := s["required"].([]any); ok {
			for _, r := range req {
				if _, ok := obj[r.(string)]; !ok {
					return fmt.Errorf("%s: %q is required", path, r)
				}
			}
		}
		for k, val := range obj {
			if sub, ok := props[k]; ok {
				if err := v.check(sub, val, path+"."+k); err != nil {
					return err
				}
				continue
			}
			switch ap := s["additionalProperties"].(type) {
			case bool:
				if !ap {
					return fmt.Errorf("%s: %q is not allowed", path, k)
				}
			case map[string]any:
				if err := v.check(ap, val, path+"."+k); err != nil {
					return err
				}
			}
		}
	}
	if arr, ok := x.([]any); ok {
		if n, ok := s["minItems"].(float64); ok && float64(len(arr)) < n {
			return fmt.Errorf("%s: %d items, want at least %v", path, len(arr), n)
		}
		if n, ok := s["maxItems"].(float64); ok && float64(len(arr)) > n {
			return fmt.Errorf("%s: %d items, want at most %v", path, len(arr), n)
		}
		if items, ok := s["items"]; ok {
			for i, e := range arr {
				if err := v.check(items, e, fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func typeIs(t string, x any) bool {
	switch t {
	case "null":
		return x == nil
	case "boolean":
		_, ok := x.(bool)
		return ok
	case "string":
		_, ok := x.(string)
		return ok
	case "number":
		_, ok := x.(float64)
		return ok
	case "integer":
		f, ok := x.(float64)
		return ok && f == float64(int64(f))
	case "array":
		_, ok := x.([]any)
		return ok
	case "object":
		_, ok := x.(map[string]any)
		return ok
	}
	return false
}

func loadSchema(t *testing.T) validator {
	t.Helper()
	b, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Defs map[string]any `json:"$defs"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	return validator{defs: s.Defs}
}

func validate(t *testing.T, v validator, def string, doc []byte) error {
	t.Helper()
	var x any
	if err := json.Unmarshal(doc, &x); err != nil {
		t.Fatal(err)
	}
	return v.check(map[string]any{"$ref": "#/$defs/" + def}, x, def)
}

func TestSchema_Goldens(t *testing.T) {
	v := loadSchema(t)
	for _, c := range []struct{ glob, def string }{
		{"testdata/build/*.request.json", "buildRequest"},
		{"testdata/build/*.answer.json", "buildAnswer"},
		{"testdata/read/*.answer.json", "readAnswer"},
	} {
		files, _ := filepath.Glob(c.glob)
		if len(files) == 0 {
			t.Fatalf("no files for %s", c.glob)
		}
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if err := validate(t, v, c.def, b); err != nil {
				t.Errorf("%s: %v", f, err)
			}
		}
	}
	// Refusals Read gives, which no golden holds.
	for _, include := range [][]string{{}, {"meshes"}} {
		answer, _ := Read(testVersion, []byte("x"), include)
		if err := validate(t, v, "readAnswer", answer); err != nil {
			t.Errorf("read refusal %s: %v", answer, err)
		}
	}
}

func TestSchema_RejectsDrift(t *testing.T) {
	v := loadSchema(t)
	for _, c := range []struct{ name, def, doc string }{
		{"extra answer key", "buildAnswer", `{"goifcVersion":"v","ok":true,"walls":[],"slabs":[],"Extra":1}`},
		{"misspelt request field", "buildRequest", `{"walls":[{"id":"w","edge":[[0,0],[1,0]],"base":0,"height":1,"thikness":0.2,"side":"left"}]}`},
		{"unknown code", "buildAnswer", `{"goifcVersion":"v","ok":false,"errors":[{"code":"nope","message":"m"}]}`},
		{"thickness and layers", "buildRequest", `{"slabs":[{"id":"s","outline":[[0,0],[1,0],[1,1]],"z":0,"thickness":0.2,"layers":[0.2]}]}`},
		{"id with a slash", "buildRequest", `{"slabs":[{"id":"a/b","outline":[[0,0],[1,0],[1,1]],"z":0,"thickness":0.2}]}`},
		{"unknown part", "readAnswer", `{"goifcVersion":"v","ok":true,"parts":["meshes"],"warnings":[]}`},
	} {
		if err := validate(t, v, c.def, []byte(c.doc)); err == nil {
			t.Errorf("%s: the schema accepts %s", c.name, c.doc)
		}
	}
}
