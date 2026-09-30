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
    if tool.startswith("ifcopenshell-"):
        return f"IfcOpenShell, {tool.removeprefix('ifcopenshell-').removesuffix('t')} threads"
    return LABEL[tool]


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
        others = [v for t, v in vals.items() if t != "goifc"]
        cells.append(ratio(vals["goifc"], others, *words) if "goifc" in vals else "—")
        rows.append(cells)
    return table(header, rows)


def agreement_table(results, tools):
    header = ["Model", "Reference elements"] + [label(t) for t in tools]
    rows = []
    for name, m in results["models"].items():
        ref = next((s["agreement"]["reference"] for s in m["tools"].values() if "agreement" in s), None)
        cells = [model_name(name, m), f"{ref:,}" if ref is not None else "—"]
        for t in tools:
            s = m["tools"].get(t, {})
            a = s.get("agreement")
            if "error" in s:
                cells.append("failed")
            elif not a or not a["reference"]:
                cells.append("—")
            else:
                cells.append(f"{a['agree']:,} of {a['reference']:,}" + (f" ({a['covered']:,} meshed)" if a["covered"] != a["reference"] else ""))
        rows.append(cells)
    return table(header, rows)


def render(results):
    tools = ["goifc", "ifcopenshell", "web-ifc"]
    mt = next((t for m in results["models"].values() for t in m["tools"] if t.startswith("ifcopenshell-")), None)
    geom_tools = tools[:2] + ([mt] if mt else []) + tools[2:]
    env = results["env"]
    corpus = table(
        ["Model", "Size", "Schema", "Exporter", "What it is", "Licence"],
        [[model_name(n, m), f"{m['mb']:.1f} MB", m["schema"], m["exporter"], m["shape"], m["licence"]]
         for n, m in results["models"].items()],
    )
    ifcopenhouse = results["models"].get("ifcopenhouse", {}).get("tools", {})
    cold = table(
        ["Tool", "Process start to exit, smallest model"],
        [[label(t), dur(ifcopenhouse[t]["wall_ms"])] for t in tools if "wall_ms" in ifcopenhouse.get(t, {})],
    )
    fp = table(
        ["Tool", "What you install", "Size"],
        [[label(t), f["what"], f"{f['mib']:.1f} MB"] for t, f in results["footprint"].items()],
    )
    return f"""Measured {env['date']} on {env['cpu']} ({env['cores']} cores, {env['memory_gib']} GiB),
{env['os']}. goifc {env['goifc']} ({env['go']}), IfcOpenShell {env['ifcopenshell']}
(Python {env['python']}), web-ifc {env['web-ifc']} (Node {env['node']}). Medians of
five runs below 10 MB and three above, each in a fresh process.

### Corpus

{corpus}

### Parse + walk + tessellate

The whole job: file bytes in, every element's properties and a mesh out.

{metric_table(results, geom_tools, "total_ms", dur)}

### Parse

{metric_table(results, tools, "parse_ms", dur)}

### Walk

Every product's identity, property and quantity sets and spatial container.

{metric_table(results, tools, "walk_ms", dur)}

### Tessellate

{metric_table(results, geom_tools, "geom_ms", dur)}

### Peak memory

Peak resident set (`VmHWM`) of the whole process, runtime included.

{metric_table(results, tools, "peak_rss_mib", lambda v: f"{v:,.0f} MiB", ("less", "more"))}

### Geometry agreement with IfcOpenShell

How many of the building elements IfcOpenShell meshes the tool reproduces within
1 cm on every side of the world bounding box, and in brackets how many it meshed
at all when that is fewer. IfcOpenShell is the reference, so its own column is
complete by construction.

{agreement_table(results, tools)}

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
