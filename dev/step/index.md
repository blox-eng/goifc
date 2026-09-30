Source

This page is [`step/README.md`](https://github.com/blox-eng/goifc/blob/main/step/README.md) included verbatim — edit that file, not this page.

# `ifc/step` — Go STEP/EXPRESS tokenizer + entity graph

Schema-agnostic STEP/SPF (ISO 10303-21) parser for IFC files, ported from ifcopenshell's parser + `entity_instance` model into idiomatic Go. Parses a `.ifc` file in-process into a navigable entity graph with **forward and inverse** references. No CAD kernel, no Python, no EXPRESS schema.

## Scope: pure SPF, not the schema

A STEP file is purely positional — `#5=IFCWALL('guid',#6,'name',...)` stores attributes by position, never by name. This package exposes exactly what the raw stream yields; naming and type-hierarchy features are a separate schema layer built on top.

| In scope (pure SPF, no schema)                                      | Out of scope (needs the EXPRESS schema)       |
| ------------------------------------------------------------------- | --------------------------------------------- |
| attribute by **index** — `inst.Get(i)`, `inst.Args()`               | attribute by **name** — `inst.GlobalId`       |
| type keyword — `inst.Type()`, `inst.IsA()` (exact)                  | `is_a(supertype)`, `ByType` subtype expansion |
| forward refs — `Value.Ref()` (resolved `#id`)                       | named inverse attrs — `.IsDecomposedBy`       |
| inverse graph — `File.Inverse` / `InverseIndices` / `TotalInverses` | derived-attribute formulas                    |
| `Traverse`, `ByID`, `ByType` (exact), `All`                         | `by_guid`, `create_entity` by name            |

`IsA`/`ByType` are exact-type only. `Inverse` exposes the **raw referrer graph**; projecting it into named IFC inverse attributes is the schema layer's job.

## API

```
f, err := step.ParseFile("model.ifc")   // or ParseBytes([]byte) / Parse(io.Reader)
f.SchemaID()                             // "IFC2X3"
f.Len()                                  // instance count
wall, ok := f.ByID(42)                   // lookup by #id
walls := f.ByType("IfcWall")             // exact type (case-insensitive)
placement, ok := wall.Ref(5)             // resolved #id -> *Instance
referrers := f.Inverse(unit)             // who references this instance
closure := f.Traverse(project, step.Unbounded, step.DepthFirst) // forward closure
for inst := range f.All() { _ = inst }   // iterate all instances (no alloc)
f.Warnings()                             // non-fatal issues (e.g. dangling refs)
```

Grammar covered: `#id` refs, typed values (`IFCLABEL(...)`), enums (`.MILLI.`), booleans (`.T./.F.`) and logical unknown (`.U.`, distinct from false), `$` (unset) / `*` (derived), integers/reals (including non-conformant leading-dot reals like `.5`), binary, nested lists, complex instances (`#id=(TYPEA(...)TYPEB(...))`), and ISO-10303-21 string escapes (`\X2\`, `\X4\`, `\X\`, `\S\`, `\P`, `''`). Tokenization is a character stream (not line-based), so multi-line records and `/* */` comments parse correctly.

## Design

Eager, two-pass, in-memory, laid out as a few flat slabs rather than a tree of small objects:

```
ParseBytes(src)
  pass 1  scan HEADER + every #id=KEYWORD(args); split across cores (below)
          -> Instance{id, type, args range}   appended to one []Instance
          -> every value, list member and arg  appended to one []Value
          -> every string, enum and binary     appended to one string arena
  pass 2  -> id index: a slice when ids are compact, a map when they are not
          -> type index, sized by a counting pass
          -> inverse index as one flat []InverseRef plus per-instance offsets
          -> dangling ref = non-fatal warning (ifcopenshell SYN 28 parity)
```

Pass 1 runs in parallel on files of 2 MB and up. The header is read serially to `DATA;`, the records are cut into GOMAXPROCS chunks at a `;` followed by `#`, and each chunk parses into its own slab. A cut can land inside a string or comment that happens to hold `;#`, so each chunk must end exactly where the next one begins; if any does not, or any chunk fails, the whole file is parsed again serially. The result, and every error with its offset, is the serial parser's. Pass 2 builds the type and inverse indexes over instance ranges in parallel, keeping source order.

A `Value` is a 24-byte handle into its `File`: a kind, a payload (an int, a float's bits, a reference id, or an offset into the string arena or value slab) and a length. References resolve on access through the id index rather than being patched in place. Read values through `Str`, `List`, `Ref`, `RefID`, `Float`, `Int` and `Bool`.

## Measured — a 28 MB IFC2X3 ArchiCAD export

| Metric                    | Value                                                             |
| ------------------------- | ----------------------------------------------------------------- |
| File size                 | ~28 MB                                                            |
| Instances                 | 528,228                                                           |
| Inverse edges             | 858,642                                                           |
| **Parse time**            | **~0.05 s** on 26 cores, ~0.18 s on one (i7-14700K, best of five) |
| **Live heap after parse** | **~74 MiB**, excluding the source bytes                           |
| Allocations               | ~166 MiB in ~5,600 allocations per parse                          |

Before the slab layout the same file took ~0.55 s and held ~255 MiB in 3.7 M allocations, most of it 72-byte `Value` structs carrying three pointers each that the garbage collector had to trace. The allocations that remain are the per-chunk slabs, the frozen string arenas and the index-building scratch.

## Not in this package

Semantic model (`IFCElement[]`), geometry, quantities, and the EXPRESS schema layer are built on top of this package. This package stops at the navigable graph.

## Also see

- [The pipeline](https://docs.goifc.org/latest/concepts/pipeline/index.md) — where `step` sits relative to `model` and `geometry`.
- [`step` on pkg.go.dev](https://pkg.go.dev/github.com/blox-eng/goifc/step) — the generated API reference.
