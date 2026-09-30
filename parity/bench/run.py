"""Run the reader benchmark: every tool on every model, in fresh processes.

    python run.py [--models a,b] [--tools goifc,web-ifc] [--repeats N]

Needs Go, Node with `npm ci` done here, and a Python that has
requirements.txt installed (run this script with that Python). Writes
results/results.json; report.py turns it into docs/benchmarks.md.
"""
import argparse
import gzip
import hashlib
import json
import os
import platform
import shutil
import statistics
import subprocess
import sys
import time
import urllib.request
from pathlib import Path

HERE = Path(__file__).resolve().parent
CACHE = HERE / ".cache"
PARITY = HERE.parent

# IfcOpenShell also meshes openings and spaces; goifc and web-ifc deliberately
# do not, so they are not part of the reference set either tool is scored on.
NOT_BUILDING_ELEMENTS = {"IfcOpeningElement", "IfcOpeningStandardCase", "IfcSpace"}

# A bound further than this from IfcOpenShell's counts as disagreeing.
AGREE_M = 0.01

TIMEOUT_S = 1800


def model_path(m):
    name = m["name"]
    if m["source"] == "committed":
        out = CACHE / f"{name}.ifc"
        if not out.exists():
            with gzip.open(PARITY / "testdata" / f"{name}.ifc.gz") as src, open(out, "wb") as dst:
                shutil.copyfileobj(src, dst)
        return out
    if m["source"] == "private":
        root = os.environ.get("GOIFC_PRIVATE_CORPUS")
        p = Path(root) / f"{name}.ifc" if root else None
        return p if p and p.exists() else None
    out = CACHE / f"{name}.ifc"
    if not out.exists():
        print(f"fetching {name}...", file=sys.stderr)
        tmp = out.with_suffix(".part")
        urllib.request.urlretrieve(m["url"], tmp)
        tmp.rename(out)
    digest = hashlib.sha256(out.read_bytes()).hexdigest()
    if digest != m["sha256"]:
        raise SystemExit(f"{name}: sha256 {digest} does not match corpus.json")
    return out


def tools():
    goifc = CACHE / "goifc-bench"
    subprocess.run(
        ["go", "build", "-trimpath", "-o", str(goifc), "./bench/goifc"],
        cwd=PARITY, check=True, env={**os.environ, "CGO_ENABLED": "0"},
    )
    ios = [sys.executable, str(HERE / "ifcopenshell_run.py")]
    threads = os.cpu_count()
    return {
        "goifc": lambda model, boxes: [str(goifc), "-boxes", boxes, model],
        "goifc-1t": lambda model, boxes: [str(goifc), "-procs", "1", "-boxes", boxes, model],
        "ifcopenshell": lambda model, boxes: ios + [model, "--boxes", boxes],
        f"ifcopenshell-{threads}t": lambda model, boxes: ios + [model, "--boxes", boxes, "--threads", str(threads)],
        "web-ifc": lambda model, boxes: ["node", str(HERE / "webifc_run.mjs"), model, "--boxes", boxes],
        # The same job goifc does: no opening subtraction.
        "ifcopenshell-noopen": lambda model, boxes: ios + [model, "--boxes", boxes, "--no-openings"],
        f"ifcopenshell-noopen-{threads}t": lambda model, boxes: ios + [
            model, "--boxes", boxes, "--no-openings", "--threads", str(threads)],
        "web-ifc-noopen": lambda model, boxes: [
            "node", str(HERE / "webifc_run.mjs"), model, "--boxes", boxes, "--no-openings"],
    }


def run_once(argv):
    t = time.perf_counter()
    try:
        p = subprocess.run(argv, capture_output=True, text=True, timeout=TIMEOUT_S)
    except subprocess.TimeoutExpired:
        return {"error": f"timeout after {TIMEOUT_S}s"}
    wall = (time.perf_counter() - t) * 1000
    if p.returncode != 0:
        return {"error": (p.stderr.strip().splitlines() or ["exit %d" % p.returncode])[-1][:300]}
    rec = json.loads(p.stdout.strip().splitlines()[-1])
    rec["wall_ms"] = wall
    return rec


def agreement(ref_boxes, boxes):
    ref = {g: b for g, b in ref_boxes.items() if b.get("type") not in NOT_BUILDING_ELEMENTS}
    covered = agree = 0
    for g, r in ref.items():
        b = boxes.get(g)
        if b is None:
            continue
        covered += 1
        if all(abs(b[k][i] - r[k][i]) <= AGREE_M for k in ("min", "max") for i in range(3)):
            agree += 1
    return {"reference": len(ref), "covered": covered, "agree": agree}


def summarize(runs):
    ok = [r for r in runs if "error" not in r]
    if not ok:
        return {"error": runs[0]["error"]}
    out = {k: statistics.median(r[k] for r in ok)
           for k in ("parse_ms", "walk_ms", "geom_ms", "wall_ms", "peak_rss_mib")}
    out["total_ms"] = out["parse_ms"] + out["walk_ms"] + out["geom_ms"]
    out["meshes"] = ok[0]["meshes"]
    out["tris"] = ok[0].get("tris")
    out["runs"] = len(ok)
    return out


def versions():
    def cmd(argv):
        return subprocess.run(argv, capture_output=True, text=True).stdout.strip()

    import ifcopenshell

    web_ifc = json.loads((HERE / "node_modules" / "web-ifc" / "package.json").read_text())["version"]
    cpu = next((l.split(":", 1)[1].strip() for l in open("/proc/cpuinfo") if l.startswith("model name")), "")
    mem_kb = next(int(l.split()[1]) for l in open("/proc/meminfo") if l.startswith("MemTotal"))
    goifc = cmd(["git", "-C", str(PARITY), "describe", "--tags", "--always"])
    return {
        "date": time.strftime("%Y-%m-%d"),
        "goifc": goifc,
        "go": cmd(["go", "env", "GOVERSION"]),
        "ifcopenshell": ifcopenshell.version,
        "python": platform.python_version(),
        "web-ifc": web_ifc,
        "node": cmd(["node", "--version"]),
        "cpu": cpu,
        "cores": os.cpu_count(),
        "memory_gib": round(mem_kb / 1024 / 1024),
        "os": f"{platform.system()} {platform.release()}",
    }


def footprint(goifc_bin):
    def du(p):
        return sum(f.stat().st_size for f in Path(p).rglob("*") if f.is_file()) / 1024 / 1024

    import ifcopenshell

    return {
        "goifc": {"what": "static binary, cgo off", "mib": goifc_bin.stat().st_size / 1024 / 1024},
        "ifcopenshell": {"what": "installed Python package", "mib": du(Path(ifcopenshell.__file__).parent)},
        "web-ifc": {"what": "installed npm package", "mib": du(HERE / "node_modules" / "web-ifc")},
    }


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--models")
    ap.add_argument("--tools")
    ap.add_argument("--repeats", type=int, help="default: 5 below 10 MB, 3 above")
    args = ap.parse_args()

    CACHE.mkdir(exist_ok=True)
    (CACHE / "boxes").mkdir(exist_ok=True)
    corpus = json.loads((HERE / "corpus.json").read_text())
    if args.models:
        corpus = [m for m in corpus if m["name"] in args.models.split(",")]
    all_tools = tools()
    names = args.tools.split(",") if args.tools else list(all_tools)

    out_path = HERE / "results" / "results.json"
    results = json.loads(out_path.read_text()) if out_path.exists() else {"models": {}}

    for m in corpus:
        path = model_path(m)
        if path is None:
            print(f"{m['name']}: not available, skipped", file=sys.stderr)
            continue
        size = path.stat().st_size
        repeats = args.repeats or (5 if size < 10 * 2**20 else 3)
        entry = results["models"].setdefault(m["name"], {"tools": {}})
        entry.update({k: m[k] for k in ("schema", "exporter", "shape", "licence", "source")})
        entry["mb"] = size / 1e6
        for tool in names:
            boxes = CACHE / "boxes" / f"{tool}-{m['name']}.json"
            runs = []
            for i in range(repeats):
                runs.append(run_once(all_tools[tool](str(path), str(boxes))))
                print(f"{m['name']:16} {tool:18} run {i + 1}/{repeats}: "
                      f"{runs[-1].get('error') or '%.0f ms' % runs[-1]['wall_ms']}", file=sys.stderr)
                if "error" in runs[-1]:
                    break
            entry["tools"][tool] = summarize(runs)

    # Score every tool against IfcOpenShell twice: doing its default job
    # (openings cut) and doing goifc's (openings not cut).
    for name, entry in results["models"].items():
        for ref_tool, key in (("ifcopenshell", "agreement"), ("ifcopenshell-noopen", "agreement_noopen")):
            ref = CACHE / "boxes" / f"{ref_tool}-{name}.json"
            if not ref.exists():
                continue
            ref_boxes = json.loads(ref.read_text())
            for tool, s in entry["tools"].items():
                b = CACHE / "boxes" / f"{tool}-{name}.json"
                if "error" not in s and b.exists():
                    s[key] = agreement(ref_boxes, json.loads(b.read_text()))

    results["env"] = versions()
    results["footprint"] = footprint(CACHE / "goifc-bench")
    out_path.parent.mkdir(exist_ok=True)
    out_path.write_text(json.dumps(results, indent=1, sort_keys=True) + "\n")
    print(f"wrote {out_path}", file=sys.stderr)


if __name__ == "__main__":
    main()
