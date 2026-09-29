# Benchmarks

goifc against the two IFC readers people most often pick instead of it —
[IfcOpenShell](https://ifcopenshell.org) and
[web-ifc](https://github.com/ThatOpen/engine_web-ifc) — on the same files, on the
same machine, losses included.

## In short

On the real models (everything but the 0.1 MB toy), against the best of the
other two in each column:

- **The whole job is 7.7–14.5× faster** than web-ifc, 18–126× faster than
  IfcOpenShell on all 26 cores, and 67–700× faster than IfcOpenShell on one.
- **Parse is 1.8–4.0× faster**: the DATA section is parsed across all cores
  into flat slabs, where it used to be one thread building a tree of small
  objects.
- **Reading properties is 6–11× faster**, although goifc's walk does more work
  than the others' (it also resolves materials and derives quantities).
- **Tessellation is 13–58× faster than web-ifc.** It runs in parallel and
  meshes a shape mapped into many places once. Its geometry is also simpler:
  see the last point.
- **Peak memory is 1.3–5× lower** than the leaner of the other two, on every
  model.
- **It starts in 4 ms and ships as a 3 MB static binary**, against about 300 ms
  and a 24 MB (web-ifc) or 230 MB (IfcOpenShell) install.

**Where goifc is still worse: its geometry is simpler.** It does not cut
openings out of walls, which both others do, and the agreement check below
cannot see that. Where its boxes do differ from IfcOpenShell's by more than
1 cm — 2 to 5 elements per model, almost all walls, and one site with no mesh
on two models — goifc's box is always the larger one, never the smaller: a
loose bound, as [coverage](coverage.md) describes, not an under-report. Part
of the tessellation lead is this simplicity, not speed.

## Method

Each tool runs the same three stages on one file, in a fresh process, and times
each stage from inside that process:

1. **Parse** — file bytes to an in-memory model. `step.ParseBytes`,
   `ifcopenshell.open`, `IfcAPI.OpenModel`.
2. **Walk** — for every product, its identity, its property and quantity sets and
   its spatial container: what a quantity reader needs before it measures
   anything. goifc runs `model.Extract`, which does this and more (it also
   resolves materials and derives quantities), so its walk does strictly more
   work than the other two.
3. **Tessellate** — a mesh for every element that has one. `geometry.Build`,
   IfcOpenShell's geometry iterator in world coordinates, and web-ifc's
   `StreamAllMeshes` with each vertex buffer read out of the WebAssembly heap.

Whole-process peak memory (`VmHWM`) and the process's wall time are recorded
alongside. Runners live in
[`parity/bench`](https://github.com/blox-eng/goifc/tree/main/parity/bench); each
is short enough to read in a minute, and a runner that looks unfair is a bug
worth an issue.

### What makes it fair, and where it cannot be

**The tools do not do the same geometric work, so speed is never reported
without agreement.** IfcOpenShell builds solids with OpenCASCADE, its default
kernel, and subtracts openings from walls; web-ifc subtracts them too, with its
own mesh booleans; goifc produces [proxy meshes](concepts/quantities.md) and
deliberately does not ([limitations](limitations.md)). A reader that skips work
is faster for it. So every model also reports how many of the building elements
IfcOpenShell meshes each tool meshed, and how many of those it placed within
1 cm of IfcOpenShell's world bounding box. Openings and spaces are
left out of that reference set: IfcOpenShell meshes them, the other two do not,
and neither is a building element anyone takes off.

**Bounding-box agreement is not shape agreement.** A wall with its window cut out
and a wall without have the same box. The agreement column shows that a tool
found the element and put it in the right place at the right size, not that its
mesh matches.

**Single-threaded is the headline.** goifc and web-ifc (under Node) tessellate on
one thread. IfcOpenShell can use every core, so it gets a second column with
all of them.

**web-ifc parses lazily.** `OpenModel` tokenizes the file and decodes entities on
first access, so part of what goifc and IfcOpenShell count as parse, web-ifc
pays for during the walk. Compare the whole-job table before the per-stage ones.

**Its walk is hand-written.** web-ifc's `properties` helpers rescan every
relationship per element and would lose by orders of magnitude on the larger
models, so the runner resolves each relationship once, the way a
performance-minded caller would.

**IfcOpenShell runs from Python.** Parse and tessellation are C++ underneath and
the Python overhead there is small; the walk is Python through and through,
which is how most IfcOpenShell users write it.

**One machine, not a lab.** The host was not otherwise idle, so small models
(single-digit milliseconds) move by tens of percent between runs, and web-ifc's
timings moved by up to a third between two runs on the larger ones. The ratios
above are ranges for that reason. goifc and web-ifc were last measured together;
the IfcOpenShell columns come from an earlier run the same day on the same
host, since nothing about IfcOpenShell changed and a full run of it takes an
hour.

### Models

The three smallest models are the public [coverage](coverage.md) corpus. The
three larger ones are
[buildingSMART community sample files](https://github.com/buildingsmart-community/Community-Sample-Test-Files),
CC-BY 4.0, fetched by URL and pinned by SHA-256. The seventh is a private
production model; it is described but not named, it is not redistributable, and
only its aggregate numbers are published.

## Results

<!-- bench:begin -->

Measured 2026-09-29 on Intel(R) Core(TM) i7-14700K (26 cores, 63 GiB),
Linux 6.17.0-29-generic. goifc v0.14.0-9-gb872dfe (go1.25.12), IfcOpenShell 0.9.0
(Python 3.12.3), web-ifc 0.0.78 (Node v24.17.0). Medians of
five runs below 10 MB and three above, each in a fresh process.

### Corpus

| Model | Size | Schema | Exporter | What it is | Licence |
|---|---|---|---|---|---|
| `ifcopenhouse` | 0.1 MB | IFC2X3 | IfcOpenShell 0.5 | IfcOpenShell's sample house | redistributable published sample, committed under parity/testdata |
| `duplex_a` | 2.4 MB | IFC2X3 | Revit 2011 | buildingSMART Duplex Apartment, architectural | redistributable published sample, committed under parity/testdata |
| `fzk_haus` | 2.6 MB | IFC4 | ArchiCAD 20 | KIT FZK-Haus, detached house | redistributable published sample, committed under parity/testdata |
| `clinic_arch` | 13.0 MB | IFC2X3 | Revit 2011 | Medical-Dental Clinic, architectural | CC-BY-4.0, buildingSMART community sample files |
| `schependomlaan` | 49.3 MB | IFC2X3 | ArchiCAD | Schependomlaan design model, housing block | CC-BY-4.0, buildingSMART community sample files |
| `clinic_plumbing` | 55.8 MB | IFC2X3 | Revit 2013 | Medical-Dental Clinic, plumbing (MEP) | CC-BY-4.0, buildingSMART community sample files |
| private model (30 MB) | 29.6 MB | IFC2X3 | ArchiCAD 25 | private seven-storey building, one block | private, not redistributable; published as aggregates only |

### Parse + walk + tessellate

The whole job: file bytes in, every element's properties and a mesh out.

| Model | goifc | IfcOpenShell | IfcOpenShell, 26 threads | web-ifc | goifc vs best other |
|---|---|---|---|---|---|
| `ifcopenhouse` | 1.8 ms | 149 ms | 100 ms | 35 ms | **19.8× faster** |
| `duplex_a` | 23 ms | 2,979 ms | 511 ms | 177 ms | **7.7× faster** |
| `fzk_haus` | 20 ms | 2,786 ms | 1,098 ms | 293 ms | **14.5× faster** |
| `clinic_arch` | 130 ms | 15.8 s | 3,225 ms | 1,083 ms | **8.3× faster** |
| `schependomlaan` | 251 ms | 16.8 s | 4,506 ms | 3,289 ms | **13.1× faster** |
| `clinic_plumbing` | 499 ms | 347.0 s | 63.0 s | 4,166 ms | **8.4× faster** |
| private model (30 MB) | 150 ms | 88.2 s | 18.1 s | 2,017 ms | **13.4× faster** |

### Parse

| Model | goifc | IfcOpenShell | web-ifc | goifc vs best other |
|---|---|---|---|---|
| `ifcopenhouse` | 1.4 ms | 4.4 ms | 6.2 ms | **3.1× faster** |
| `duplex_a` | 12 ms | 54 ms | 23 ms | **1.9× faster** |
| `fzk_haus` | 14 ms | 62 ms | 25 ms | **1.8× faster** |
| `clinic_arch` | 33 ms | 184 ms | 89 ms | **2.7× faster** |
| `schependomlaan` | 121 ms | 698 ms | 480 ms | **4.0× faster** |
| `clinic_plumbing` | 137 ms | 830 ms | 466 ms | **3.4× faster** |
| private model (30 MB) | 84 ms | 499 ms | 283 ms | **3.4× faster** |

### Walk

Every product's identity, property and quantity sets and spatial container.

| Model | goifc | IfcOpenShell | web-ifc | goifc vs best other |
|---|---|---|---|---|
| `ifcopenhouse` | 0.2 ms | 11 ms | 6.9 ms | **38.2× faster** |
| `duplex_a` | 8.6 ms | 127 ms | 96 ms | **11.2× faster** |
| `fzk_haus` | 3.6 ms | 33 ms | 91 ms | **9.3× faster** |
| `clinic_arch` | 90 ms | 850 ms | 768 ms | **8.5× faster** |
| `schependomlaan` | 118 ms | 750 ms | 2,394 ms | **6.3× faster** |
| `clinic_plumbing` | 302 ms | 3,613 ms | 2,936 ms | **9.7× faster** |
| private model (30 MB) | 43 ms | 260 ms | 1,298 ms | **6.0× faster** |

### Tessellate

| Model | goifc | IfcOpenShell | IfcOpenShell, 26 threads | web-ifc | goifc vs best other |
|---|---|---|---|---|---|
| `ifcopenhouse` | 0.2 ms | 134 ms | 84 ms | 22 ms | **117.6× faster** |
| `duplex_a` | 2.3 ms | 2,798 ms | 395 ms | 58 ms | **25.0× faster** |
| `fzk_haus` | 3.1 ms | 2,692 ms | 1,007 ms | 177 ms | **58.0× faster** |
| `clinic_arch` | 7.3 ms | 14.8 s | 2,407 ms | 227 ms | **31.0× faster** |
| `schependomlaan` | 12 ms | 15.3 s | 3,039 ms | 416 ms | **34.5× faster** |
| `clinic_plumbing` | 60 ms | 342.5 s | 59.4 s | 764 ms | **12.8× faster** |
| private model (30 MB) | 24 ms | 87.5 s | 17.4 s | 436 ms | **18.4× faster** |

### Peak memory

Peak resident set (`VmHWM`) of the whole process, runtime included.

| Model | goifc | IfcOpenShell | web-ifc | goifc vs best other |
|---|---|---|---|---|
| `ifcopenhouse` | 5 MiB | 95 MiB | 148 MiB | **19.9× less** |
| `duplex_a` | 28 MiB | 111 MiB | 213 MiB | **3.9× less** |
| `fzk_haus` | 28 MiB | 137 MiB | 187 MiB | **5.0× less** |
| `clinic_arch` | 128 MiB | 252 MiB | 352 MiB | **2.0× less** |
| `schependomlaan` | 337 MiB | 462 MiB | 580 MiB | **1.4× less** |
| `clinic_plumbing` | 636 MiB | 972 MiB | 833 MiB | **1.3× less** |
| private model (30 MB) | 282 MiB | 496 MiB | 501 MiB | **1.8× less** |

### Geometry agreement with IfcOpenShell

How many of the building elements IfcOpenShell meshes the tool reproduces within
1 cm on every side of the world bounding box, and in brackets how many it meshed
at all when that is fewer. IfcOpenShell is the reference, so its own column is
complete by construction.

| Model | Reference elements | goifc | IfcOpenShell | web-ifc |
|---|---|---|---|---|
| `ifcopenhouse` | 35 | 34 of 35 (34 meshed) | 35 of 35 | 35 of 35 |
| `duplex_a` | 215 | 213 of 215 | 215 of 215 | 215 of 215 |
| `fzk_haus` | 83 | 80 of 83 (82 meshed) | 83 of 83 | 83 of 83 |
| `clinic_arch` | 2,585 | 2,580 of 2,585 | 2,585 of 2,585 | 2,585 of 2,585 |
| `schependomlaan` | 3,504 | 3,501 of 3,504 | 3,504 of 3,504 | 3,504 of 3,504 |
| `clinic_plumbing` | 6,587 | 6,587 of 6,587 | 6,587 of 6,587 | 6,587 of 6,587 |
| private model (30 MB) | 1,878 | 1,876 of 1,878 | 1,878 of 1,878 | 1,878 of 1,878 |

### Cold start

| Tool | Process start to exit, smallest model |
|---|---|
| goifc | 4.2 ms |
| IfcOpenShell | 314 ms |
| web-ifc | 367 ms |

### Footprint

| Tool | What you install | Size |
|---|---|---|
| goifc | static binary, cgo off | 3.3 MB |
| IfcOpenShell | installed Python package | 230.4 MB |
| web-ifc | installed npm package | 23.6 MB |

<!-- bench:end -->

## Reproduce

```bash
cd parity/bench
npm ci                                   # web-ifc, pinned
python3 -m venv .venv && .venv/bin/pip install -r requirements.txt
.venv/bin/python run.py                  # --models, --tools, --repeats
.venv/bin/python report.py               # rewrites the tables above
```

`run.py` downloads the larger models into `parity/bench/.cache` on first use and
refuses any whose hash has changed. Set `GOIFC_PRIVATE_CORPUS` to include
private models; without it they are skipped and the public rows are unchanged.
