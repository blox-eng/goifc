package wasmapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// checkKeys refuses any object key in raw that is not exactly a json tag of
// the struct it decodes into, or that appears twice. encoding/json matches
// keys case-insensitively and keeps the last of a repeat, so "Thickness" or
// a second "id" would otherwise build something the caller did not write.
func checkKeys(raw json.RawMessage, t reflect.Type, path string) error {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	switch t.Kind() {
	case reflect.Struct:
		dec := json.NewDecoder(bytes.NewReader(raw))
		if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
			return nil // not an object: json.Unmarshal reports the type error
		}
		seen := map[string]bool{}
		for dec.More() {
			tok, err := dec.Token()
			if err != nil {
				return nil
			}
			key, _ := tok.(string)
			var val json.RawMessage
			if err := dec.Decode(&val); err != nil {
				return nil
			}
			f, ok := fieldByTag(t, key)
			if !ok {
				return fmt.Errorf("%sunknown field %q", path, key)
			}
			if seen[key] {
				return fmt.Errorf("%sfield %q is given twice", path, key)
			}
			seen[key] = true
			if err := checkKeys(val, f.Type, path+key+"."); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		var items []json.RawMessage
		if json.Unmarshal(raw, &items) != nil {
			return nil
		}
		for i, it := range items {
			if err := checkKeys(it, t.Elem(), fmt.Sprintf("%s%d.", path, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

func fieldByTag(t reflect.Type, key string) (reflect.StructField, bool) {
	for i := range t.NumField() {
		f := t.Field(i)
		if name, _, _ := strings.Cut(f.Tag.Get("json"), ","); name == key {
			return f, true
		}
	}
	return reflect.StructField{}, false
}
