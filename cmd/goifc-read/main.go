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
		return jsbridge.Guard(version, func() any {
			var src []byte
			var include []string
			if len(args) > 0 {
				src = jsbridge.Bytes(args[0])
			}
			if len(args) > 1 {
				include = jsbridge.Include(args[1])
			}
			return jsbridge.Result(wasmapi.Read(version, src, include))
		})
	}))
	select {}
}
