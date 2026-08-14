# Storey plans

`BuildImport` pre-bakes one `StoreyPlan` per `IfcBuildingStorey`, so a renderable
floor plan comes out of the import with no second pass over the geometry.

```go
m, err := ifc.BuildImport(f)
if err != nil {
	panic(err)
}
for _, plan := range m.StoreyPlans {
	fmt.Printf("storey %s at %.2f m — %d entities\n",
		plan.StoreyGlobalID, plan.Elevation, len(plan.Entities))

	for _, ent := range plan.Entities {
		for _, loop := range ent.Loops {
			_ = ent.GlobalID // your own domain object id
			_ = ent.IFCClass // "IfcWall", "IfcDoor", ...
			_ = loop.Role    // LoopCut (poché) or LoopSilhouette (context)
			_ = loop.Points  // [][2]float64, world XY metres
		}
	}
}
```

## The shape

```go
type StoreyPlan struct {
	StoreyGlobalID string
	Elevation      float64 // metres, for UI ordering
	Entities       []StoreyEntity
}

type StoreyEntity struct {
	GlobalID string
	IFCClass string
	Loops    []geometry.Loop
}
```

Loops are the same `geometry.Loop` described in
[sections and floor plans](sections.md) — same roles, same winding, same
hole convention.

## The cut height

Each storey is cut at **`floorZ + 1.2 m`**, the architectural convention for a
plan cut: above the door handles, below the window heads.

`floorZ` is *seeded* from the minimum world Z of the mesh elements spatially
contained in that storey. Seeded, not decided — spatial containment picks the
band, and then **geometry decides membership**. An element assigned to storey 2
in the IFC hierarchy but physically sitting on storey 1 draws where it actually
is. This matters on real exports, where containment is frequently wrong.

## Storeys that get skipped

A storey with no contained mesh element is **omitted from `StoreyPlans`
entirely** — there is no reliable way to place its cut plane, so it produces no
plan rather than an empty or arbitrarily-placed one.

Do not assume `len(m.StoreyPlans)` equals the number of `IfcBuildingStorey`
nodes in the tree. Iterate the plans, or join back on `StoreyGlobalID`.

`Elevation` comes from the storey's own `IfcBuildingStorey` elevation attribute
and is `0` when absent — it is for ordering the storeys in a UI, not for
positioning geometry. The geometry is already in world coordinates.

## Determinism

Identical input yields an identical slice — same storeys, same order, same
entities, same loops. You can golden-diff storey plans across releases and a
change in the output means a change in behaviour.
