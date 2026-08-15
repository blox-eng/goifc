# The pipeline

`Assemble` runs three stages. Each one is a package you can use on its own.

| Package    | Does |
|------------|------|
| `step`     | STEP/SPF (ISO 10303-21) tokenizer and entity graph. Schema-agnostic — no EXPRESS. |
| `model`    | Entity graph → semantic `[]Element`. Ports ifcopenshell's `util.element`, `util.placement`, `util.unit`. |
| `geometry` | Proxy tessellation, derived quantities, plane sections, and a Y-up GLB whose node names are `GlobalId`s. |

```
bytes ──▶ step.ParseBytes ──▶ *step.File          entity graph, forward + inverse refs
                                  │
                                  ▼
                            model.Extract   ──▶ *model.Result     semantic elements
                                  │
                                  ▼
                            geometry.Build  ──▶ *geometry.Scene   meshes, bboxes, quantities
                                  │
                                  ▼
                            ifc.Assemble    ──▶ *Assembled        elements + scene
                                  │
                                  ▼
                            ifc.BuildImport ──▶ *ImportModel      tree, layers, storey plans
```

## Why the split matters

**`step` is genuinely standalone.** It parses any STEP file, IFC or not — it
knows nothing about walls or storeys, only `#id=KEYWORD(args)` and the reference
graph between them. If you have a STEP file from a mechanical CAD tool, this
package still works. See [the `step` package](../step.md).

**`model` is where IFC semantics enter.** It reads positional attribute indices
that are stable across IFC2X3 and IFC4 for core entities, resolves placements,
and normalises units. This is also the layer that carries the schema assumption
noted in [limitations](../limitations.md) — there is no EXPRESS schema behind it.

**`geometry` is the only stage that approximates.** Everything above it is
lossless with respect to the file. Tessellation is where proxy meshes and
derived, gross quantities come from, and it is why quantities carry a
[provenance tag](quantities.md).
