# Security Policy

## Threat model

goifc parses IFC/STEP files supplied by third parties — a modeller, a client, an
upload. **The file is the untrusted input; the caller is not the adversary.**

Against that model:

- **A panic, a hang, or unbounded memory growth on a malformed file is a
  vulnerability.** Consumers run this on files they did not author, often in a
  shared worker, so a crafted file that takes the process down is a denial of
  service. The parse and geometry paths carry explicit recursion guards and are
  fuzzed (`make fuzz`) for exactly this reason.
- **A wrong quantity is a bug, not a vulnerability.** Please open an issue.

## What is bounded

The work goifc does on a file is bounded by the file's size, not only by
recursion depth:

- **Each element** has a tessellation budget. It counts the work of meshing,
  the vertices and triangles shipped, and the walk for a fallback box. Past the
  budget the element's items are boxed; past twice the budget they are left out.
- **Each `Build`** has a budget sized to the file: 512 units per instance, plus
  one element budget. A shape, face or profile shared by thousands of elements
  cannot cost each of them a full element budget. Past the Build budget, the
  remaining elements are boxed, one box each; past twice it they are left out.
  Which elements those are is settled in element order, so it is the same on
  every run.
- **Storey plans** are charged per vertex and triangle they cut, against a
  budget sized to the file. A parent cycle is detected, not followed.
- **Property sets** that many elements share are read once and shared. Copies
  made to merge a type's sets into an occurrence's are charged to a budget.
- **Relationships** are looked up by index, so an N-element relationship costs
  N, not N².
- **Recursion** has explicit depth guards on the parse and geometry paths.

Every budget that runs out says so in the result's warnings, with a count. No
model in the [coverage corpus](https://docs.goifc.org/latest/coverage/) reaches
any of these budgets.

What is still not bounded:

- **The file itself.** goifc holds the parsed file in memory, in proportion to
  its size. Limit the size of the uploads you accept.
- **Cancellation.** `Build` takes no `context.Context`, so a caller cannot stop
  it early. Its time is bounded, but a deadline needs the caller's own process
  or worker limit.

A file that makes goifc's work or memory grow faster than its size is a
vulnerability under the threat model above. Please report it.

## Reporting a Vulnerability

Please report security vulnerabilities privately via [GitHub Security
Advisories](https://github.com/blox-eng/goifc/security/advisories/new) for
this repository, rather than opening a public issue.

Include as much detail as you can: the affected version, a minimal
reproduction (ideally an IFC/STEP fragment that triggers the issue), and the
impact you believe it has.

There is no guaranteed response time or support SLA. This is maintained on a
best-effort basis.
