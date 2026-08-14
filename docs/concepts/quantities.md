# Quantities and provenance

Every element reports where its numbers came from, because the two sources are
not equally trustworthy.

| `QuantitySource` | Means |
|------------------|-------|
| `"qto"`          | Authored `IfcElementQuantity`. Net, exact, from the modeller. |
| `"geometry"`     | Derived from the proxy mesh. Gross — a bounding estimate. |
| `"none"`         | Neither available. Never a fabricated `0.0`. |

Authored quantities always win. A missing quantity stays missing. If you only
trust one tier, filter on the tag.

## How the tiers get applied

`Assemble` chains four stages, and the last two are what produce this tagging:

```
model.Extract                    authored Qto, where the modeller wrote one
geometry.Build                   proxy meshes per element
Scene.DerivedQuantities()        tier-2 GROSS quantities from those meshes
Result.ApplyDerivedQuantities()  back-fills ONLY where tier 1 is absent
```

Back-filling never overwrites. An element that arrived with an authored volume
keeps it and stays `"qto"`; an element with no authored volume takes the derived
one and becomes `"geometry"`; an element with no mesh either stays `"none"`.

## Why tier 2 is gross

On the `"geometry"` tier the extrude path reports the solid, un-subtracted
volume. A wall with a window and a door over-reports against IfcOpenShell's net
figure.

Netting openings out of extruded geometry needs a real B-rep kernel; the honest
answer there is IfcOpenShell. The tag exists so you can tell the two apart and
treat tier 2 as a bound.

!!! warning "Do not sum across tiers silently"
    Adding a `"qto"` volume to a `"geometry"` volume produces a number that is
    neither net nor gross, and nothing downstream can tell that it happened.
    If you aggregate, either filter to one tier or carry the split through to
    whatever reads the total.

## Checking a quantity before you use it

Quantities are pointers precisely so that "absent" is distinguishable from zero:

```go
for i := range a.Result.Elements {
	e := a.Result.Elements[i]
	if e.Qto.Volume == nil {
		continue // genuinely absent — not 0.0
	}
	if e.QuantitySource == "geometry" {
		// a gross bound; fine for sizing, wrong for a bill of quantities
	}
	fmt.Printf("%s\t%.3f m³\t(%s)\n", e.Name, *e.Qto.Volume, e.QuantitySource)
}
```
