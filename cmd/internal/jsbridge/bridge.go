//go:build js && wasm

// Package jsbridge moves bytes between JavaScript and the wasmapi contract.
// It holds no contract logic: that lives in internal/wasmapi.
package jsbridge

import (
	"runtime/debug"
	"syscall/js"
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
