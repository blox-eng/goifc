//go:build js && wasm

// Command goifc-build is goifc-build.wasm. It sets one global function,
// goifcBuild(request: string) → {answer: string, glb: Uint8Array | null},
// and waits for calls. See docs/guides/wasm.md.
package main

import (
	"syscall/js"

	"github.com/blox-eng/goifc/cmd/internal/jsbridge"
	"github.com/blox-eng/goifc/internal/wasmapi"
)

func main() {
	version := jsbridge.Version()
	js.Global().Set("goifcBuild", js.FuncOf(func(_ js.Value, args []js.Value) any {
		var request []byte
		if len(args) > 0 && args[0].Type() == js.TypeString {
			request = []byte(args[0].String())
		}
		return jsbridge.Result(wasmapi.Build(version, request))
	}))
	select {}
}
