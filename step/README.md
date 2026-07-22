# `ifc/step` — Go STEP/EXPRESS tokenizer + entity graph

Schema-agnostic STEP/SPF (ISO 10303-21) parser for IFC files, ported from
ifcopenshell's parser + `entity_instance` model into idiomatic Go. Parses a `.ifc`
file in-process into a navigable entity graph with **forward and inverse**
references. No CAD kernel, no Python, no EXPRESS schema.

Part of the Go-native IFC engine (epic #2206), child 1 (#2207).

## Scope: pure SPF, not the schema

A STEP file is purely positional — `#5=IFCWALL('guid',#6,'name',...)` stores
attributes by position, never by name. This package exposes exactly what the raw
stream yields; naming and type-hierarchy features are a separate schema layer
(child 2) built on top.

| In scope (pure SPF, no schema) | Out of scope (needs the EXPRESS schema) |
|---|---|
| attribute by **index** — `inst.Get(i)`, `inst.Args()` | attribute by **name** — `inst.GlobalId` |
| type keyword — `inst.Type()`, `inst.IsA()` (exact) | `is_a(supertype)`, `ByType` subtype expansion |
| forward refs — `Value.Ref` (resolved `#id`) | named inverse attrs — `.IsDecomposedBy` |
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

Eager, two-pass, in-memory (ported from ifcopenshell's default in-memory variant):

```
ParseBytes(src)
  pass 1  scan HEADER + every #id=KEYWORD(args); record
          -> Instance{id, type, []Value}   (refs captured, unresolved)
          -> byID map, byType index, insertion order
  pass 2  walk every attribute
          -> resolve #ref -> *Instance (in place)
          -> build inverse index (target id -> []{referrer, attrIndex})
          -> dangling ref = non-fatal warning (ifcopenshell SYN 28 parity)
```

## Measured — kb645.ifc (28 MB IFC2X3 ArchiCAD export)

| Metric | Value |
|---|---|
| File size | 29,558,941 B (~28 MB) |
| Instances | 528,228 |
| Entity types | 93 |
| Inverse edges | 857,962 |
| **Parse time** | **~0.48–0.66 s** (i7-14700K) |
| **Peak heap** | **~306 MB** |
| Allocations | ~500 MB / 3.7 M allocs per parse |

**Memory driver (for #2206 child 6 worker bounding):** peak is dominated by the
~3.7 M `Value` structs (**72 B each ≈ 266 MB**, after field-order packing from 80 B),
not string data — so string interning would not move peak. At ~30% of a 1 GB budget
there's ample headroom; a columnar/arena `Value` rework is deferred behind this
measurement and only worth it if a future input class blows the budget. Size worker
limits against ~310 MB peak per 28 MB IFC, scaling roughly linearly with instance
count.

## Not in this package

Semantic model (`IFCElement[]`), geometry, quantities, and the EXPRESS schema layer
are later children of #2206. This package stops at the navigable graph.
