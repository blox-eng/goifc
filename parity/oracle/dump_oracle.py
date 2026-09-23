"""Write GlobalID -> world AABB for every element IfcOpenShell can build.

Run via `make oracle`, never in CI. The output is committed data.
"""
import json
import sys

import ifcopenshell
import ifcopenshell.geom


def aabb(verts):
    xs, ys, zs = verts[0::3], verts[1::3], verts[2::3]
    return {"min": [min(xs), min(ys), min(zs)], "max": [max(xs), max(ys), max(zs)]}


def main(path, out):
    f = ifcopenshell.open(path)
    settings = ifcopenshell.geom.settings()
    settings.set(settings.USE_WORLD_COORDS, True)
    boxes = {}
    it = ifcopenshell.geom.iterator(settings, f)
    if it.initialize():
        while True:
            shape = it.get()
            verts = shape.geometry.verts
            if verts:
                boxes[shape.guid] = aabb(verts)
            if not it.next():
                break
    with open(out, "w") as fh:
        json.dump(boxes, fh, indent=1, sort_keys=True)
    print(f"{out}: {len(boxes)} elements")


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
