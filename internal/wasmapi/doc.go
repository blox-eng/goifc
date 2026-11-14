// Package wasmapi is the JSON contract of goifc-build.wasm and
// goifc-read.wasm, in plain Go so it is tested like the rest of goifc.
//
// Build turns one request of walls, rooms and slabs into an answer and a
// GLB; Read turns an IFC file into an answer, a GLB and the parts the caller
// asks for. The JSON shapes are defined here and only here: no geometry,
// model or ifc type is encoded directly, so a Go-side rename cannot change
// the contract without a failing golden test. The design is
// docs/superpowers/specs/2026-11-14-wasm-design.md.
package wasmapi
