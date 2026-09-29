# Benchmarks

goifc against the two IFC readers people most often pick instead of it —
[IfcOpenShell](https://ifcopenshell.org) and
[web-ifc](https://github.com/ThatOpen/engine_web-ifc) — on the same files, on the
same machine, losses included.

## In short

The fair comparison is the same job on the same cores: no tool cuts openings
out of walls, and every tool gets one core. On the six real models (everything
but the 0.1 MB toy), against the best of the other two in each column:

- **The whole job is 2.7–4.1× faster** than web-ifc and 25–270× faster than
  IfcOpenShell.
- **Parse is level with web-ifc** — from 1.2× slower to 1.3× faster depending
  on the model — and 1.8–3.0× faster than IfcOpenShell.
- **Reading properties is 3–11× faster** than the faster of the other two,
  although goifc's walk does more (it also resolves materials and derives
  quantities).
- **Tessellation is 1.5–6.7× faster** than web-ifc, producing within 9% of the
  same number of triangles.
- **Peak memory is 1.1–5× lower.**

On every core, goifc parses and tessellates in parallel and IfcOpenShell
tessellates in parallel; web-ifc cannot under Node. There the whole job is
17–138× faster than IfcOpenShell, with 2–10× less memory.

Each tool's default job — IfcOpenShell and web-ifc cutting openings, goifc on
all cores — is 7.6–11.4× faster end to end than web-ifc, but that figure mixes
three things: less geometric work, more cores, and speed. The two numbers above
separate them.

**Where goifc is worse**

- **It does not cut openings**, where both others do by default. Its meshes and
  derived quantities are gross ([quantities](concepts/quantities.md)).
- **Its geometry is less exact even on the same job.** Against IfcOpenShell
  with openings off, web-ifc reproduces every element's box within 1 cm on
  every model. goifc misses the site's mesh on two models and gets 2–4 walls
  per model loose on four: its box is larger than IfcOpenShell's, never smaller
  — a bound, as [coverage](coverage.md) describes, not an under-report.
- **On one core its parse is slightly slower than web-ifc's** on two of the
  six models (1.1× and 1.2×).

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

**The same job.** IfcOpenShell cuts openings out of walls with OpenCASCADE, its
default kernel, and web-ifc cuts them with its own mesh booleans; goifc produces
[proxy meshes](concepts/quantities.md) and does not
([limitations](limitations.md)). The headline tables switch that step off in
both: IfcOpenShell through its `disable-opening-subtractions` setting, web-ifc
by removing the `IfcRelVoidsElement` records that name the openings, before the
file is loaded — web-ifc has no setting, cuts only openings those records name,
and nothing references them. The triangle counts show the meshes that result
are comparable.

**The same cores.** goifc parses and tessellates on every core, and so the
headline tables limit it to one. IfcOpenShell tessellates on every core when
asked and appears in the all-cores tables. web-ifc has no column there: its
multi-threaded build, under Node, ran geometry on one thread at the same speed
as the single-threaded one.

**Agreement goes with every speed number.** For every building element
IfcOpenShell meshes, the tables count whether each tool meshed it and placed its
world bounding box within 1 cm of IfcOpenShell's, doing the same job. Openings
and spaces are left out of that reference set: IfcOpenShell meshes them, the
other two do not, and neither is a building element anyone takes off.

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

**One machine, not a lab.** Nothing else ran during a measurement, but small
models (single-digit milliseconds) still move by tens of percent between runs,
and web-ifc's timings moved by up to a third between runs on the larger ones —
hence ranges. goifc and web-ifc were measured together. The single-threaded
IfcOpenShell run with openings cut is from an earlier run the same day on the
same host: nothing about IfcOpenShell changed, and it takes an hour.

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
Linux 6.17.0-29-generic. goifc v0.14.0-10-gfbb7abe (go1.25.12), IfcOpenShell 0.9.0
(Python 3.12.3), web-ifc 0.0.78 (Node v24.17.0). Medians of
five runs below 10 MB and three above, each in a fresh process, with nothing
else running on the host.

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

### The same job, one core

Openings are not cut by any tool, and every tool gets one core. This is the
like-for-like comparison.

#### Parse + walk + tessellate

The whole job: file bytes in, every element's properties and a mesh out.

| Model | goifc, 1 thread | IfcOpenShell, no openings | web-ifc, no openings | goifc vs best other |
|---|---|---|---|---|
| `ifcopenhouse` | 2.0 ms | 115 ms | 23 ms | **11.7× faster** |
| `duplex_a` | 35 ms | 1,569 ms | 141 ms | **4.0× faster** |
| `fzk_haus` | 31 ms | 1,466 ms | 108 ms | **3.5× faster** |
| `clinic_arch` | 208 ms | 11.6 s | 853 ms | **4.1× faster** |
| `schependomlaan` | 682 ms | 17.1 s | 2,526 ms | **3.7× faster** |
| `clinic_plumbing` | 1,307 ms | 358.1 s | 3,691 ms | **2.8× faster** |
| private model (30 MB) | 574 ms | 86.0 s | 1,540 ms | **2.7× faster** |

#### Parse

| Model | goifc, 1 thread | IfcOpenShell, no openings | web-ifc, no openings | goifc vs best other |
|---|---|---|---|---|
| `ifcopenhouse` | 1.6 ms | 4.1 ms | 3.1 ms | **2.0× faster** |
| `duplex_a` | 18 ms | 34 ms | 20 ms | even |
| `fzk_haus` | 19 ms | 57 ms | 21 ms | **1.1× faster** |
| `clinic_arch` | 68 ms | 179 ms | 90 ms | **1.3× faster** |
| `schependomlaan` | 301 ms | 739 ms | 377 ms | **1.3× faster** |
| `clinic_plumbing` | 411 ms | 845 ms | 367 ms | 1.1× slower |
| private model (30 MB) | 265 ms | 489 ms | 215 ms | 1.2× slower |

#### Walk

Every product's identity, property and quantity sets and spatial container.

| Model | goifc, 1 thread | IfcOpenShell, no openings | web-ifc, no openings | goifc vs best other |
|---|---|---|---|---|
| `ifcopenhouse` | 0.2 ms | 10 ms | 5.3 ms | **24.8× faster** |
| `duplex_a` | 12 ms | 57 ms | 87 ms | **4.9× faster** |
| `fzk_haus` | 2.7 ms | 29 ms | 59 ms | **10.6× faster** |
| `clinic_arch` | 99 ms | 608 ms | 645 ms | **6.2× faster** |
| `schependomlaan` | 241 ms | 760 ms | 1,916 ms | **3.1× faster** |
| `clinic_plumbing` | 410 ms | 2,463 ms | 2,582 ms | **6.0× faster** |
| private model (30 MB) | 62 ms | 243 ms | 966 ms | **3.9× faster** |

#### Tessellate

| Model | goifc, 1 thread | IfcOpenShell, no openings | web-ifc, no openings | goifc vs best other |
|---|---|---|---|---|
| `ifcopenhouse` | 0.2 ms | 101 ms | 15 ms | **88.7× faster** |
| `duplex_a` | 5.0 ms | 1,478 ms | 33 ms | **6.7× faster** |
| `fzk_haus` | 8.9 ms | 1,380 ms | 29 ms | **3.2× faster** |
| `clinic_arch` | 41 ms | 10.9 s | 117 ms | **2.8× faster** |
| `schependomlaan` | 139 ms | 15.6 s | 232 ms | **1.7× faster** |
| `clinic_plumbing` | 486 ms | 354.8 s | 742 ms | **1.5× faster** |
| private model (30 MB) | 247 ms | 85.3 s | 359 ms | **1.5× faster** |

#### Triangles

How much mesh each tool produced, over every element it meshed. A flat wall is
12 triangles however it is built; the differences are curves, how finely each
tool divides them, and which elements get a mesh at all.

| Model | goifc, 1 thread | IfcOpenShell, no openings | web-ifc, no openings | goifc vs best other |
|---|---|---|---|---|
| `ifcopenhouse` | 468 | 1,078 | 1,030 | **2.2× fewer** |
| `duplex_a` | 24,827 | 26,928 | 25,868 | even |
| `fzk_haus` | 18,020 | 22,272 | 19,754 | even |
| `clinic_arch` | 157,093 | 184,977 | 167,949 | even |
| `schependomlaan` | 256,177 | 261,882 | 261,959 | even |
| `clinic_plumbing` | 3,329,982 | 3,354,132 | 3,178,306 | even |
| private model (30 MB) | 1,107,556 | 1,133,261 | 1,127,634 | even |

#### Peak memory

Peak resident set (`VmHWM`) of the whole process, runtime included.

| Model | goifc, 1 thread | IfcOpenShell, no openings | web-ifc, no openings | goifc vs best other |
|---|---|---|---|---|
| `ifcopenhouse` | 4 MiB | 92 MiB | 148 MiB | **21.0× less** |
| `duplex_a` | 23 MiB | 106 MiB | 218 MiB | **4.6× less** |
| `fzk_haus` | 26 MiB | 134 MiB | 211 MiB | **5.2× less** |
| `clinic_arch` | 138 MiB | 249 MiB | 392 MiB | **1.8× less** |
| `schependomlaan` | 398 MiB | 458 MiB | 627 MiB | **1.1× less** |
| `clinic_plumbing` | 694 MiB | 948 MiB | 837 MiB | **1.2× less** |
| private model (30 MB) | 246 MiB | 492 MiB | 572 MiB | **2.0× less** |

#### Geometry agreement

How many of the building elements IfcOpenShell meshes, with openings not cut,
the tool reproduces within 1 cm on every side of the world bounding box, and in
brackets how many it meshed at all when that is fewer.

| Model | Reference elements | goifc | web-ifc, no openings | IfcOpenShell, no openings |
|---|---|---|---|---|
| `ifcopenhouse` | 35 | 34 of 35 (34 meshed) | 35 of 35 | 35 of 35 |
| `duplex_a` | 215 | 213 of 215 | 215 of 215 | 215 of 215 |
| `fzk_haus` | 83 | 80 of 83 (82 meshed) | 83 of 83 | 83 of 83 |
| `clinic_arch` | 2,585 | 2,581 of 2,585 | 2,585 of 2,585 | 2,585 of 2,585 |
| `schependomlaan` | 3,569 | 3,569 of 3,569 | 3,569 of 3,569 | 3,569 of 3,569 |
| `clinic_plumbing` | 6,587 | 6,587 of 6,587 | 6,587 of 6,587 | 6,587 of 6,587 |
| private model (30 MB) | 1,878 | 1,876 of 1,878 | 1,878 of 1,878 | 1,878 of 1,878 |

### The same job, all cores

goifc parses and tessellates on every core; IfcOpenShell tessellates on every
core. web-ifc has no column: under Node its multi-threaded build runs geometry
on one thread, measured at the same speed as the single-threaded one.

#### Parse + walk + tessellate

| Model | goifc | IfcOpenShell, no openings, 26 threads | goifc vs best other |
|---|---|---|---|
| `ifcopenhouse` | 1.7 ms | 97 ms | **58.4× faster** |
| `duplex_a` | 22 ms | 384 ms | **17.4× faster** |
| `fzk_haus` | 19 ms | 494 ms | **25.4× faster** |
| `clinic_arch` | 116 ms | 3,015 ms | **26.1× faster** |
| `schependomlaan` | 216 ms | 4,288 ms | **19.8× faster** |
| `clinic_plumbing` | 457 ms | 63.2 s | **138.3× faster** |
| private model (30 MB) | 148 ms | 18.5 s | **125.2× faster** |

#### Tessellate

| Model | goifc | IfcOpenShell, no openings, 26 threads | goifc vs best other |
|---|---|---|---|
| `ifcopenhouse` | 0.2 ms | 83 ms | **494.8× faster** |
| `duplex_a` | 2.5 ms | 286 ms | **115.1× faster** |
| `fzk_haus` | 2.5 ms | 406 ms | **159.4× faster** |
| `clinic_arch` | 5.8 ms | 2,220 ms | **381.3× faster** |
| `schependomlaan` | 9.3 ms | 2,896 ms | **311.5× faster** |
| `clinic_plumbing` | 55 ms | 59.7 s | **1,075.8× faster** |
| private model (30 MB) | 26 ms | 17.5 s | **674.6× faster** |

#### Peak memory

| Model | goifc | IfcOpenShell, no openings, 26 threads | goifc vs best other |
|---|---|---|---|
| `ifcopenhouse` | 5 MiB | 96 MiB | **20.1× less** |
| `duplex_a` | 29 MiB | 189 MiB | **6.5× less** |
| `fzk_haus` | 28 MiB | 194 MiB | **7.0× less** |
| `clinic_arch` | 129 MiB | 851 MiB | **6.6× less** |
| `schependomlaan` | 395 MiB | 790 MiB | **2.0× less** |
| `clinic_plumbing` | 631 MiB | 6,540 MiB | **10.4× less** |
| private model (30 MB) | 284 MiB | 1,919 MiB | **6.8× less** |

### Each tool's default job

What a caller gets without changing any setting: IfcOpenShell and web-ifc cut
openings out of walls, goifc does not. Faster here partly means doing less.

#### Parse + walk + tessellate

| Model | goifc | goifc, 1 thread | IfcOpenShell | IfcOpenShell, 26 threads | web-ifc | goifc vs best other |
|---|---|---|---|---|---|---|
| `ifcopenhouse` | 1.7 ms | 2.0 ms | 149 ms | 94 ms | 29 ms | **17.6× faster** |
| `duplex_a` | 22 ms | 35 ms | 2,979 ms | 437 ms | 168 ms | **7.6× faster** |
| `fzk_haus` | 19 ms | 31 ms | 2,786 ms | 833 ms | 188 ms | **9.7× faster** |
| `clinic_arch` | 116 ms | 208 ms | 15.8 s | 3,205 ms | 956 ms | **8.3× faster** |
| `schependomlaan` | 216 ms | 682 ms | 16.8 s | 4,576 ms | 2,463 ms | **11.4× faster** |
| `clinic_plumbing` | 457 ms | 1,307 ms | 347.0 s | 65.9 s | 3,760 ms | **8.2× faster** |
| private model (30 MB) | 148 ms | 574 ms | 88.2 s | 17.4 s | 1,669 ms | **11.3× faster** |

#### Tessellate

| Model | goifc | goifc, 1 thread | IfcOpenShell | IfcOpenShell, 26 threads | web-ifc | goifc vs best other |
|---|---|---|---|---|---|---|
| `ifcopenhouse` | 0.2 ms | 0.2 ms | 134 ms | 82 ms | 18 ms | **110.1× faster** |
| `duplex_a` | 2.5 ms | 5.0 ms | 2,798 ms | 342 ms | 56 ms | **22.4× faster** |
| `fzk_haus` | 2.5 ms | 8.9 ms | 2,692 ms | 767 ms | 107 ms | **42.1× faster** |
| `clinic_arch` | 5.8 ms | 41 ms | 14.8 s | 2,409 ms | 205 ms | **35.2× faster** |
| `schependomlaan` | 9.3 ms | 139 ms | 15.3 s | 3,148 ms | 259 ms | **27.9× faster** |
| `clinic_plumbing` | 55 ms | 486 ms | 342.5 s | 61.6 s | 768 ms | **13.8× faster** |
| private model (30 MB) | 26 ms | 247 ms | 87.5 s | 16.7 s | 424 ms | **16.4× faster** |

#### Triangles

| Model | goifc | IfcOpenShell, 26 threads | web-ifc | goifc vs best other |
|---|---|---|---|---|
| `ifcopenhouse` | 468 | 1,146 | 1,098 | **2.3× fewer** |
| `duplex_a` | 24,827 | 27,740 | 26,774 | even |
| `fzk_haus` | 18,020 | 24,168 | 21,722 | **1.2× fewer** |
| `clinic_arch` | 157,093 | 191,265 | 174,325 | **1.1× fewer** |
| `schependomlaan` | 256,177 | 261,238 | 261,315 | even |
| `clinic_plumbing` | 3,329,982 | 3,354,132 | 3,178,306 | even |
| private model (30 MB) | 1,107,556 | 1,137,319 | 1,132,259 | even |

### Geometry agreement with IfcOpenShell's default job

How many of the building elements IfcOpenShell meshes (openings cut) the tool
reproduces within 1 cm on every side of the world bounding box, and in brackets
how many it meshed at all when that is fewer. IfcOpenShell is the reference, so
its own column is complete by construction.

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
| goifc | 3.3 ms |
| IfcOpenShell | 314 ms |
| web-ifc | 266 ms |

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
