"""Render results/results.json into the generated block of docs/benchmarks.md.

    python report.py [--check]

--check exits non-zero when the committed page does not match the results,
instead of rewriting it.
"""
import json
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
PAGE = HERE.parent.parent / "docs" / "benchmarks.md"
BEGIN, END = "<!-- bench:begin -->", "<!-- bench:end -->"

LABEL = {"goifc": "goifc", "ifcopenshell": "IfcOpenShell", "web-ifc": "web-ifc"}


def label(tool):
    name = tool
    suffix = []
    if name.endswith("t") and name.rsplit("-", 1)[-1][:-1].isdigit():
        name, threads = name.rsplit("-", 1)
        n = threads[:-1]
        suffix.append(f"{n} thread" + ("" if n == "1" else "s"))
    if name.endswith("-noopen"):
        name = name.removesuffix("-noopen")
        suffix.insert(0, "no openings")
    return LABEL[name] + (f", {', '.join(suffix)}" if suffix else "")


def dur(ms):
    if ms < 10:
        return f"{ms:.1f} ms"
    if ms < 10_000:
        return f"{ms:,.0f} ms"
    return f"{ms / 1000:,.1f} s"


def ratio(goifc, others, better="faster", worse="slower"):
    """goifc against the best of the others, as '12× faster' or '1.4× slower'."""
    if not others:
        return "—"
    best = min(others)
    if goifc <= 0 or max(goifc, best) / min(goifc, best) < 1.1:
        return "even"
    if goifc < best:
        return f"**{best / goifc:,.1f}× {better}**"
    return f"{goifc / best:,.1f}× {worse}"


def model_name(name, m):
    if m["source"] == "private":
        return f"private model ({m['mb']:.0f} MB)"
    return f"`{name}`"


def table(header, rows):
    out = ["| " + " | ".join(header) + " |", "|" + "|".join("---" for _ in header) + "|"]
    out += ["| " + " | ".join(r) + " |" for r in rows]
    return "\n".join(out)


def metric_table(results, tools, key, fmt, words=("faster", "slower")):
    """A model-by-tool table of key; tools[0] is the goifc variant compared."""
    me = tools[0]
    header = ["Model"] + [label(t) for t in tools] + ["goifc vs best other"]
    rows = []
    for name, m in results["models"].items():
        cells, vals = [model_name(name, m)], {}
        for t in tools:
            s = m["tools"].get(t, {})
            if "error" in s:
                cells.append("failed")
            elif key in s:
                vals[t] = s[key]
                cells.append(fmt(s[key]))
            else:
                cells.append("—")
        # Only other tools: a second goifc column is a variant, not a rival.
        others = [v for t, v in vals.items() if not t.startswith("goifc")]
        cells.append(ratio(vals[me], others, *words) if me in vals else "—")
        rows.append(cells)
    return table(header, rows)


def agreement_table(results, tools, key="agreement"):
    header = ["Model", "Reference elements"] + [label(t) for t in tools]
    rows = []
    for name, m in results["models"].items():
        ref = next((s[key]["reference"] for s in m["tools"].values() if key in s), None)
        cells = [model_name(name, m), f"{ref:,}" if ref is not None else "—"]
        for t in tools:
            s = m["tools"].get(t, {})
            a = s.get(key)
            if "error" in s:
                cells.append("failed")
            elif not a or not a["reference"]:
                cells.append("—")
            else:
                cells.append(f"{a['agree']:,} of {a['reference']:,}" + (f" ({a['covered']:,} meshed)" if a["covered"] != a["reference"] else ""))
        rows.append(cells)
    return table(header, rows)


def render(results):
    env = results["env"]
    present = {t for m in results["models"].values() for t in m["tools"]}
    cores = env["cores"]
    all_ios = f"ifcopenshell-{cores}t"
    all_ios_no = f"ifcopenshell-noopen-{cores}t"
    one = [t for t in ("goifc-1t", "ifcopenshell-noopen", "web-ifc-noopen") if t in present]
    many = [t for t in ("goifc", all_ios_no) if t in present]
    default = [t for t in ("goifc", "goifc-1t", "ifcopenshell", all_ios, "web-ifc") if t in present]
    corpus = table(
        ["Model", "Size", "Schema", "Exporter", "What it is", "Licence"],
        [[model_name(n, m), f"{m['mb']:.1f} MB", m["schema"], m["exporter"], m["shape"], m["licence"]]
         for n, m in results["models"].items()],
    )
    ifcopenhouse = results["models"].get("ifcopenhouse", {}).get("tools", {})
    cold = table(
        ["Tool", "Process start to exit, smallest model"],
        [[label(t), dur(ifcopenhouse[t]["wall_ms"])] for t in ("goifc", "ifcopenshell", "web-ifc")
         if "wall_ms" in ifcopenhouse.get(t, {})],
    )
    fp = table(
        ["Tool", "What you install", "Size"],
        [[label(t), f["what"], f"{f['mib']:.1f} MB"] for t, f in results["footprint"].items()],
    )
    mem = lambda v: f"{v:,.0f} MiB"
    tris = lambda v: f"{v:,}"
    return f"""Measured {env['date']} on {env['cpu']} ({cores} cores, {env['memory_gib']} GiB),
{env['os']}. goifc {env['goifc']} ({env['go']}), IfcOpenShell {env['ifcopenshell']}
(Python {env['python']}), web-ifc {env['web-ifc']} (Node {env['node']}). Medians of
five runs below 10 MB and three above, each in a fresh process, with nothing
else running on the host.

### Corpus

{corpus}

### The same job, one core

Openings are not cut by any tool, and every tool gets one core. This is the
like-for-like comparison.

#### Parse + walk + tessellate

The whole job: file bytes in, every element's properties and a mesh out.

{metric_table(results, one, "total_ms", dur)}

#### Parse

{metric_table(results, one, "parse_ms", dur)}

#### Walk

Every product's identity, property and quantity sets and spatial container.

{metric_table(results, one, "walk_ms", dur)}

#### Tessellate

{metric_table(results, one, "geom_ms", dur)}

#### Triangles

How much mesh each tool produced, over every element it meshed. A flat wall is
12 triangles however it is built; the differences are curves, how finely each
tool divides them, and which elements get a mesh at all.

{metric_table(results, one, "tris", tris, ("fewer", "more"))}

#### Peak memory

Peak resident set (`VmHWM`) of the whole process, runtime included.

{metric_table(results, one, "peak_rss_mib", mem, ("less", "more"))}

#### Geometry agreement

How many of the building elements IfcOpenShell meshes, with openings not cut,
the tool reproduces within 1 cm on every side of the world bounding box, and in
brackets how many it meshed at all when that is fewer.

{agreement_table(results, [t for t in ("goifc", "web-ifc-noopen", "ifcopenshell-noopen") if t in present], "agreement_noopen")}

### The same job, all cores

goifc parses and tessellates on every core; IfcOpenShell tessellates on every
core. web-ifc has no column: under Node its multi-threaded build runs geometry
on one thread, measured at the same speed as the single-threaded one.

#### Parse + walk + tessellate

{metric_table(results, many, "total_ms", dur)}

#### Tessellate

{metric_table(results, many, "geom_ms", dur)}

#### Peak memory

{metric_table(results, many, "peak_rss_mib", mem, ("less", "more"))}

### Each tool's default job

What a caller gets without changing any setting: IfcOpenShell and web-ifc cut
openings out of walls, goifc does not. Faster here partly means doing less.

#### Parse + walk + tessellate

{metric_table(results, default, "total_ms", dur)}

#### Tessellate

{metric_table(results, default, "geom_ms", dur)}

#### Triangles

{metric_table(results, [t for t in default if t not in ("goifc-1t", "ifcopenshell")], "tris", tris, ("fewer", "more"))}

### Geometry agreement with IfcOpenShell's default job

How many of the building elements IfcOpenShell meshes (openings cut) the tool
reproduces within 1 cm on every side of the world bounding box, and in brackets
how many it meshed at all when that is fewer. IfcOpenShell is the reference, so
its own column is complete by construction.

{agreement_table(results, [t for t in ("goifc", "ifcopenshell", "web-ifc") if t in present])}

### Cold start

{cold}

### Footprint

{fp}"""


def main():
    results = json.loads((HERE / "results" / "results.json").read_text())
    order = [m["name"] for m in json.loads((HERE / "corpus.json").read_text())]
    results["models"] = {n: results["models"][n] for n in order if n in results["models"]}
    page = PAGE.read_text()
    head, rest = page.split(BEGIN, 1)
    _, tail = rest.split(END, 1)
    new = f"{head}{BEGIN}\n\n{render(results)}\n\n{END}{tail}"
    if "--check" in sys.argv[1:]:
        if new != page:
            sys.exit(f"{PAGE} is stale: run `python parity/bench/report.py`")
        return
    PAGE.write_text(new)
    print(f"wrote {PAGE}")


if __name__ == "__main__":
    main()
