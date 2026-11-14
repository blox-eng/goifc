//go:build js && wasm

// Package jsbridge moves bytes between JavaScript and the wasmapi contract.
// It holds no contract logic: that lives in internal/wasmapi.
package jsbridge

import (
	"fmt"
	"runtime/debug"
	"syscall/js"

	"github.com/blox-eng/goifc/internal/wasmapi"
)

// Version is the goifc version this file was built from: the tag on a
// release build, a pseudo-version, or "(devel)".
func Version() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		return bi.Main.Version
	}
	return "(devel)"
}

// Bytes copies a Uint8Array into Go. Any other value gives nil.
func Bytes(v js.Value) []byte {
	if v.Type() != js.TypeObject || !v.InstanceOf(js.Global().Get("Uint8Array")) {
		return nil
	}
	b := make([]byte, v.Get("length").Int())
	js.CopyBytesToGo(b, v)
	return b
}

// Result is the JavaScript value every exported function returns:
// {answer: string, glb: Uint8Array | null}.
func Result(answer, glb []byte) any {
	out := map[string]any{"answer": string(answer), "glb": nil}
	if glb != nil {
		u := js.Global().Get("Uint8Array").New(len(glb))
		js.CopyBytesToJS(u, glb)
		out["glb"] = u
	}
	return js.ValueOf(out)
}

// notParts stands in for an include that is not an array of strings, so
// wasmapi.Read refuses it as invalid_input instead of reading the default.
const notParts = "(include must be an array of part names)"

// Include turns the include argument into Go. undefined and null mean the
// default (nil); an array of strings is copied; anything else becomes a
// list Read refuses.
func Include(v js.Value) []string {
	if v.IsUndefined() || v.IsNull() {
		return nil
	}
	if !v.InstanceOf(js.Global().Get("Array")) {
		return []string{notParts}
	}
	out := make([]string, v.Length())
	for i := range out {
		e := v.Index(i)
		if e.Type() != js.TypeString {
			return []string{notParts}
		}
		out[i] = e.String()
	}
	return out
}

// Guard runs f and turns a panic into an invalid_input answer, so a bug in
// goifc refuses one request instead of stopping the Go runtime for good.
func Guard(version string, f func() any) (out any) {
	defer func() {
		if r := recover(); r != nil {
			out = Result(wasmapi.Failure(version, "invalid_input",
				fmt.Sprintf("goifc stopped on this input, which is a bug: please report it with the input. %v", r)), nil)
		}
	}()
	return f()
}
