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

One limitation worth stating plainly rather than leaving to be discovered: the
guards bound recursion DEPTH, not total allocation or run time. A small file
that expands into a very large tessellation is not currently bounded. If you
have a case where that matters, we would rather hear about it.

## Reporting a Vulnerability

Please report security vulnerabilities privately via [GitHub Security
Advisories](https://github.com/blox-eng/goifc/security/advisories/new) for
this repository, rather than opening a public issue.

Include as much detail as you can: the affected version, a minimal
reproduction (ideally an IFC/STEP fragment that triggers the issue), and the
impact you believe it has.

There is no guaranteed response time or support SLA. This is maintained on a
best-effort basis.
