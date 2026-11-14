//go:build js && wasm

// Command goifc-read is goifc-read.wasm. It sets one global function,
// goifcRead(ifc: Uint8Array, include: string[] | null) →
// {answer: string, glb: Uint8Array | null}, and waits for calls. See
// docs/guides/wasm.md.
package main

import (
	"syscall/js"

	"github.com/blox-eng/goifc/cmd/internal/jsbridge"
	"github.com/blox-eng/goifc/internal/wasmapi"
)

func main() {
	version := jsbridge.Version()
	js.Global().Set("goifcRead", js.FuncOf(func(_ js.Value, args []js.Value) any {
		var src []byte
		var include []string
		if len(args) > 0 {
			src = jsbridge.Bytes(args[0])
		}
		if len(args) > 1 && args[1].Type() == js.TypeObject {
			n := args[1].Get("length").Int()
			include = make([]string, n)
			for i := range n {
				include[i] = args[1].Index(i).String()
			}
		}
		return jsbridge.Result(wasmapi.Read(version, src, include))
	}))
	select {}
}
