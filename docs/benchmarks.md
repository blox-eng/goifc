# Benchmarks

goifc against the two IFC readers people most often pick instead of it —
[IfcOpenShell](https://ifcopenshell.org) and
[web-ifc](https://github.com/ThatOpen/engine_web-ifc) — on the same files, on the
same machine, losses included.

## In short

**Where goifc is better**

- **The whole job is fastest on every model.** Parse, walk and tessellate
  together run 1.7–4.3× faster than web-ifc on the real models, 5–26× faster
  than IfcOpenShell on all 26 cores, and 17–140× faster than IfcOpenShell on one.
- **Reading properties is 7–14× faster than either.** That is the walk —
  every product's property sets, quantity sets and container — and goifc's
  does more work there than the others' (it also resolves materials and
  derives quantities).
- **It starts in 5 ms and ships as a 3 MB static binary**, against about 300 ms
  and a 24 MB (web-ifc) or 230 MB (IfcOpenShell) install. Called once per file,
  from a CLI or a request handler, that startup is most of the bill on small
  models.
- **Less memory on small models:** 2.5–3× less on the 2–3 MB houses.

**Where goifc is worse**

- **Parsing is 1.3–2.4× slower than web-ifc on every real model**, and 7–25%
  slower than IfcOpenShell from 13 MB up. web-ifc defers part of its parse to
  first access, so part of that gap moves into its walk, but not all of it.
- **More memory on large models:** 1.1–2× the lowest peak from 13 MB up —
  1.6 GiB against about 0.8–1.0 GiB on the 56 MB plumbing model.
- **Tessellating the MEP model is 1.5× slower than web-ifc**, and level with it
  on the private model. goifc's lead in tessellation shrinks as models grow.
- **Its geometry is simpler.** It does not cut openings out of walls, which both
  others do; the agreement check below cannot see that. Where its boxes do
  differ from IfcOpenShell's by more than 1 cm — 2 to 5 elements per model,
  almost all walls, and one site with no mesh on two models — goifc's box is always the larger one,
  never the smaller: a loose bound, as [coverage](coverage.md) describes, not
  an under-report.

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
(single-digit milliseconds) move by tens of percent between runs. The larger
models are where the ratios are stable.

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
Linux 6.17.0-29-generic. goifc v0.14.0-4-g2bf894d (go1.25.12), IfcOpenShell 0.9.0
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
| `ifcopenhouse` | 3.0 ms | 149 ms | 100 ms | 39 ms | **12.7× faster** |
| `duplex_a` | 54 ms | 2,979 ms | 511 ms | 233 ms | **4.3× faster** |
| `fzk_haus` | 61 ms | 2,786 ms | 1,098 ms | 234 ms | **3.8× faster** |
| `clinic_arch` | 340 ms | 15.8 s | 3,225 ms | 933 ms | **2.7× faster** |
| `schependomlaan` | 972 ms | 16.8 s | 4,506 ms | 2,509 ms | **2.6× faster** |
| `clinic_plumbing` | 2,452 ms | 347.0 s | 63.0 s | 4,064 ms | **1.7× faster** |
| private model (30 MB) | 960 ms | 88.2 s | 18.1 s | 1,785 ms | **1.9× faster** |

### Parse

| Model | goifc | IfcOpenShell | web-ifc | goifc vs best other |
|---|---|---|---|---|
| `ifcopenhouse` | 2.6 ms | 4.4 ms | 7.0 ms | **1.7× faster** |
| `duplex_a` | 38 ms | 54 ms | 30 ms | 1.3× slower |
| `fzk_haus` | 49 ms | 62 ms | 25 ms | 1.9× slower |
| `clinic_arch` | 210 ms | 184 ms | 89 ms | 2.4× slower |
| `schependomlaan` | 776 ms | 698 ms | 353 ms | 2.2× slower |
| `clinic_plumbing` | 1,033 ms | 830 ms | 494 ms | 2.1× slower |
| private model (30 MB) | 534 ms | 499 ms | 255 ms | 2.1× slower |

### Walk

Every product's identity, property and quantity sets and spatial container.

| Model | goifc | IfcOpenShell | web-ifc | goifc vs best other |
|---|---|---|---|---|
| `ifcopenhouse` | 0.2 ms | 11 ms | 5.8 ms | **27.4× faster** |
| `duplex_a` | 8.7 ms | 127 ms | 121 ms | **13.9× faster** |
| `fzk_haus` | 3.8 ms | 33 ms | 71 ms | **8.7× faster** |
| `clinic_arch` | 87 ms | 850 ms | 647 ms | **7.4× faster** |
| `schependomlaan` | 106 ms | 750 ms | 1,911 ms | **7.1× faster** |
| `clinic_plumbing` | 326 ms | 3,613 ms | 2,865 ms | **8.8× faster** |
| private model (30 MB) | 31 ms | 260 ms | 1,120 ms | **8.4× faster** |

### Tessellate

| Model | goifc | IfcOpenShell | IfcOpenShell, 26 threads | web-ifc | goifc vs best other |
|---|---|---|---|---|---|
| `ifcopenhouse` | 0.2 ms | 134 ms | 84 ms | 26 ms | **139.0× faster** |
| `duplex_a` | 7.5 ms | 2,798 ms | 395 ms | 82 ms | **11.0× faster** |
| `fzk_haus` | 8.7 ms | 2,692 ms | 1,007 ms | 137 ms | **15.7× faster** |
| `clinic_arch` | 43 ms | 14.8 s | 2,407 ms | 197 ms | **4.6× faster** |
| `schependomlaan` | 90 ms | 15.3 s | 3,039 ms | 245 ms | **2.7× faster** |
| `clinic_plumbing` | 1,093 ms | 342.5 s | 59.4 s | 706 ms | 1.5× slower |
| private model (30 MB) | 396 ms | 87.5 s | 17.4 s | 410 ms | even |

### Peak memory

Peak resident set (`VmHWM`) of the whole process, runtime included.

| Model | goifc | IfcOpenShell | web-ifc | goifc vs best other |
|---|---|---|---|---|
| `ifcopenhouse` | 6 MiB | 95 MiB | 159 MiB | **16.8× less** |
| `duplex_a` | 45 MiB | 111 MiB | 221 MiB | **2.5× less** |
| `fzk_haus` | 45 MiB | 137 MiB | 190 MiB | **3.1× less** |
| `clinic_arch` | 270 MiB | 252 MiB | 358 MiB | even |
| `schependomlaan` | 725 MiB | 462 MiB | 583 MiB | 1.6× more |
| `clinic_plumbing` | 1,648 MiB | 972 MiB | 832 MiB | 2.0× more |
| private model (30 MB) | 666 MiB | 496 MiB | 498 MiB | 1.3× more |

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
| goifc | 4.9 ms |
| IfcOpenShell | 314 ms |
| web-ifc | 294 ms |

### Footprint

| Tool | What you install | Size |
|---|---|---|
| goifc | static binary, cgo off | 3.2 MB |
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
