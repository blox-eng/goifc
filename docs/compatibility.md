# Compatibility

## The API is unstable pre-1.0

Expect breaking changes on minor versions. Pin a version.

```
require github.com/blox-eng/goifc v0.2.0
```

Breaks are kept to the smallest shape that works, but that is a practice, not a
promise. There is no support SLA.

### What a break tends to look like

v0.2.0 renamed `geometry.LoopBelow` to `geometry.LoopSilhouette`. That was a
**source-only** break: the serialized string value stayed `"below"`, so stored
drawing data was unaffected and renderers matching the literal needed no change.
The fix was renaming the identifier at the call site.

Where a change would otherwise touch both the API and data already on disk, the
wire format is what gets preserved. Source you can fix with a compiler error to
guide you; a silent data migration you cannot.

Every break is written down in the [changelog](changelog.md).

## Serialization contracts

Some string values are load-bearing beyond the Go API, because consumers persist
them and match them as literals:

| Values | Where |
|---|---|
| `"qto"`, `"geometry"`, `"none"` | `QuantitySource` — see [quantities](concepts/quantities.md) |
| `"cut"`, `"below"` | `LoopRole` — see [sections](guides/sections.md) |

These do not change without a coordinated consumer migration, independently of
what the Go identifiers are called.

## Schema support

IFC2X3 and IFC4 core entities, via positional attribute indices that are stable
across both. There is no EXPRESS schema — see [limitations](limitations.md).

## Go version

The supported Go version is whatever `go.mod` declares. CI builds and tests with
`CGO_ENABLED=0`: if a dependency ever introduces a cgo requirement, the build
fails loudly instead of silently linking against system libraries.

## "Used in production by Blox" — what that scopes to

Blox's import pipeline is the only consumer this library has been hardened
against. Concretely, that means:

- **`BuildImport` on architectural IFC exports** is the exercised path.
- Formats, entity types and code paths outside that see far less real-world
  input, regardless of whether they have unit tests.

It is a statement about which bugs have already been found, not a general
assurance. Treat it as a map of where the tested ground is.

## Reporting a break

Open an issue with the IFC file if you can share it, or a reduced one that
reproduces. The [`step` package](step.md) parses any STEP file, so a minimal
repro is often only a few entities.
