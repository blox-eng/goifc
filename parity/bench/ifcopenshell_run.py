"""IfcOpenShell runner for the reader benchmark: same stages, same record as goifc/main.go."""
import argparse
import json
import sys
import time

import ifcopenshell
import ifcopenshell.geom
import ifcopenshell.util.element


def peak_rss_mib():
    with open("/proc/self/status") as fh:
        for line in fh:
            if line.startswith("VmHWM:"):
                return int(line.split()[1]) / 1024
    return 0.0


def ms(t):
    return (time.perf_counter() - t) * 1000


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("model")
    ap.add_argument("--boxes")
    ap.add_argument("--threads", type=int, default=1)
    ap.add_argument("--no-openings", action="store_true", help="skip opening subtraction, as goifc does")
    args = ap.parse_args()
    tool = "ifcopenshell-noopen" if args.no_openings else "ifcopenshell"
    rec = {"tool": tool if args.threads == 1 else f"{tool}-{args.threads}t"}

    t = time.perf_counter()
    f = ifcopenshell.open(args.model)
    rec["parse_ms"] = ms(t)

    # Walk: what a quantity reader needs per product — identity, property and
    # quantity sets, and the spatial container.
    t = time.perf_counter()
    products = 0
    for p in f.by_type("IfcProduct"):
        _ = (p.GlobalId, p.Name, p.is_a())
        ifcopenshell.util.element.get_psets(p)
        ifcopenshell.util.element.get_container(p)
        products += 1
    rec["walk_ms"] = ms(t)
    rec["products"] = products

    settings = ifcopenshell.geom.settings()
    settings.set("use-world-coords", True)
    settings.set("disable-opening-subtractions", args.no_openings)
    t = time.perf_counter()
    shapes = []
    tris = 0
    it = ifcopenshell.geom.iterator(settings, f, args.threads)
    if it.initialize():
        while True:
            shape = it.get()
            shapes.append((shape.guid, shape.geometry.verts))
            tris += len(shape.geometry.faces) // 3
            if not it.next():
                break
    rec["geom_ms"] = ms(t)

    boxes = {}
    for guid, verts in shapes:
        if not verts:
            continue
        xs, ys, zs = verts[0::3], verts[1::3], verts[2::3]
        boxes[guid] = {
            "min": [min(xs), min(ys), min(zs)],
            "max": [max(xs), max(ys), max(zs)],
            "type": f.by_guid(guid).is_a(),
        }
    rec["meshes"] = len(boxes)
    rec["tris"] = tris
    rec["peak_rss_mib"] = peak_rss_mib()

    if args.boxes:
        with open(args.boxes, "w") as fh:
            json.dump(boxes, fh)
    json.dump(rec, sys.stdout)
    print()


if __name__ == "__main__":
    main()
