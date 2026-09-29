# `ifc/step` — Go STEP/EXPRESS tokenizer + entity graph

Schema-agnostic STEP/SPF (ISO 10303-21) parser for IFC files, ported from
ifcopenshell's parser + `entity_instance` model into idiomatic Go. Parses a `.ifc`
file in-process into a navigable entity graph with **forward and inverse**
references. No CAD kernel, no Python, no EXPRESS schema.

## Scope: pure SPF, not the schema

A STEP file is purely positional — `#5=IFCWALL('guid',#6,'name',...)` stores
attributes by position, never by name. This package exposes exactly what the raw
stream yields; naming and type-hierarchy features are a separate schema layer
built on top.

| In scope (pure SPF, no schema) | Out of scope (needs the EXPRESS schema) |
|---|---|
| attribute by **index** — `inst.Get(i)`, `inst.Args()` | attribute by **name** — `inst.GlobalId` |
| type keyword — `inst.Type()`, `inst.IsA()` (exact) | `is_a(supertype)`, `ByType` subtype expansion |
| forward refs — `Value.Ref()` (resolved `#id`) | named inverse attrs — `.IsDecomposedBy` |
| inverse graph — `File.Inverse` / `InverseIndices` / `TotalInverses` | derived-attribute formulas |
| `Traverse`, `ByID`, `ByType` (exact), `All` | `by_guid`, `create_entity` by name |

`IsA`/`ByType` are exact-type only. `Inverse` exposes the **raw referrer graph**;
projecting it into named IFC inverse attributes is the schema layer's job.

## API

```go
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

Grammar covered: `#id` refs, typed values (`IFCLABEL(...)`), enums (`.MILLI.`),
booleans (`.T./.F.`) and logical unknown (`.U.`, distinct from false), `$` (unset)
/ `*` (derived), integers/reals (including
non-conformant leading-dot reals like `.5`), binary, nested lists, complex
instances (`#id=(TYPEA(...)TYPEB(...))`), and ISO-10303-21 string escapes (`\X2\`,
`\X4\`, `\X\`, `\S\`, `\P`, `''`). Tokenization is a character stream (not
line-based), so multi-line records and `/* */` comments parse correctly.

## Design

Eager, two-pass, in-memory, laid out as a few flat slabs rather than a tree of
small objects:

```
ParseBytes(src)
  pass 1  scan HEADER + every #id=KEYWORD(args);
          -> Instance{id, type, args range}   appended to one []Instance
          -> every value, list member and arg  appended to one []Value
          -> every string, enum and binary     appended to one string arena
  pass 2  -> id index: a slice when ids are compact, a map when they are not
          -> type index, sized by a counting pass
          -> inverse index as one flat []InverseRef plus per-instance offsets
          -> dangling ref = non-fatal warning (ifcopenshell SYN 28 parity)
```

A `Value` is a 24-byte handle into its `File`: a kind, a payload (an int, a
float's bits, a reference id, or an offset into the string arena or value slab)
and a length. References resolve on access through the id index rather than
being patched in place. Read values through `Str`, `List`, `Ref`, `RefID`,
`Float`, `Int` and `Bool`.

## Measured — a 28 MB IFC2X3 ArchiCAD export

| Metric | Value |
|---|---|
| File size | ~28 MB |
| Instances | 528,228 |
| Inverse edges | 858,642 |
| **Parse time** | **~0.18 s** (i7-14700K, best of five) |
| **Live heap after parse** | **~72 MiB**, excluding the source bytes |
| Allocations | ~166 MiB in ~2,300 allocations per parse |

Before the slab layout the same file took ~0.55 s and held ~255 MiB in 3.7 M
allocations, most of it 72-byte `Value` structs carrying three pointers each
that the garbage collector had to trace. The allocations that remain are the
slabs growing and the string arena being frozen.

## Not in this package

Semantic model (`IFCElement[]`), geometry, quantities, and the EXPRESS schema layer
are built on top of this package. This package stops at the navigable graph.
