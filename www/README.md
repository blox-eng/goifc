# www — goifc.org

The landing page: one screen, no scroll. Hand-written static HTML with no build
step and no dependencies — `index.html`, one stylesheet, one script, and the
marks and fonts it serves itself. Open `index.html` in a browser, or
`python3 -m http.server --directory www` for a server that resolves the root
path the way the host does.

It is built on the same frame as openblox.sh — the same type, the same
isometric projection, the same chrome — so the two read as one family. Only
the accent and the drawing differ.

`404.html` is served by Pages for any path that matches neither a file nor a
redirect. It carries **no script at all**: `app.js` drives the copy button, the
theme toggle and the canvas, none of which exist on that page, and it reaches
for those elements without guarding — so including it would throw before the
theme handler ran, leaving a page whose toggle does nothing. The theme still
follows `prefers-color-scheme` through CSS, and the toggle persists nothing, so
there is no choice to carry across from the landing page in the first place.
Inlining a small script instead is not an option either: `_headers` sets
`script-src 'self'` with no `'unsafe-inline'`, and that is worth more than a
toggle on an error page.

The drawing is the goifc mark, brought to life the way openblox.sh animates its
own: one solid in, measured parts out. STEP lines are read one at a time, each
landing as a strip of the solid, and the two parts drop out beneath it at the
same combined width. Hovering (or tapping) measures them. Both are the same box
with the same opening, and they report different volumes because they report
different things: the left one the modeller's authored net quantity (`qto`), the
right one a volume derived from the mesh, which counts the opening in (`geometry`,
gross) — drawn as what it is, a dashed bound. The figures are one consistent
illustrative element, worked out in `app.js`. Every claim on the page has to be
true of the code on `main`; if a change to the library makes one false, the page
changes in the same pull request.

The argument for goifc — what the numbers mean, which of them are bounds, where
the edges are — lives in the documentation. This page's job is to say what
goifc is and get out of the way. It describes what goifc does and never
compares it to anything; the ambition is carried by the coverage numbers it
links to, not announced. Its copy matches `README.md` and `docs/index.md` —
change all three together, so there is never a third version.

The reference documentation is a separate site — MkDocs, in `docs/`, deployed
with mike to `docs.goifc.org` by `.github/workflows/docs.yml`. This directory is
not that site and does not link into it by relative path.

## Deployment

Cloudflare Pages, connected to this repository. The domain is on Cloudflare,
which is what makes apex support and the edge redirects free; GitHub Pages
could not serve both sites, because one repository gets one custom domain, and
that one belongs to the docs.

Connecting the project and pointing the apex at it are manual steps in the
Cloudflare dashboard, done once and not by anything in this repository. Until
they are, nothing here is served at `goifc.org`. The settings to use:

| Setting | Value |
|---|---|
| Production branch | `main` |
| Build command | `.github/scripts/check-links.sh www` |
| Build output directory | `www` |
| Root directory | repository root |

The build command is a check, not a compiler: there is nothing to compile, and
the thing worth failing on is a reference that does not resolve. It is the same
script CI runs (`.github/workflows/landing.yml`), so a broken link fails the
preview deploy and the pull request alike. Pages builds every pull request as
its own preview URL.

`_redirects` and `_headers` are read by Pages and not served. The redirects send
`/docs`, `/latest/…` and `/dev/…` on the apex to `docs.goifc.org`, scoped to
those prefixes so they cannot swallow this page.

### Going live, in order

The order matters. Each step is safe only once the one before it is done.

1. **Verify `goifc.org` at the organization level** — GitHub → `blox-eng`
   organization settings → Pages → Add a domain → `goifc.org`, then add the TXT
   record GitHub gives you in Cloudflare DNS and wait for it to verify. Do this
   before any CNAME goes live: an unverified custom domain on a public
   repository is the standard subdomain-takeover vector. Verifying the apex
   covers `docs.goifc.org`.
2. **Create the Cloudflare Pages project** from this repository with the
   settings in the table above, then add `goifc.org` as its custom domain.
   Cloudflare writes the apex record itself.
3. **Point `docs.goifc.org` at GitHub Pages** — Cloudflare DNS: `CNAME docs →
   blox-eng.github.io`, **DNS only** (grey cloud). If the record is proxied,
   GitHub cannot issue the certificate.
4. **Set the repository's Pages custom domain** — `blox-eng/goifc` → Settings →
   Pages → Custom domain → `docs.goifc.org`. Wait for the DNS check to pass,
   then tick **Enforce HTTPS**.
5. **Merge the pull request that moved the docs.** Its first push to `main`
   deploys the docs with `site_url` on `docs.goifc.org` and checks that the
   `CNAME` at the root of `gh-pages` says the same. Merging earlier would serve
   the docs at a domain that does not resolve.
6. **Confirm the old URL redirects** — `curl -sI
   https://blox-eng.github.io/goifc/latest/` should answer `301` with
   `location: https://docs.goifc.org/latest/`.
7. **Set the repository's homepage** to `https://goifc.org` (repository →
   About → Website), so the GitHub sidebar points at the front door.

## Fonts

IBM Plex Sans, Sans Condensed and Mono, latin subset, taken from the Google
Fonts CDN once and committed under `assets/fonts/` — the same files openblox.sh
serves. They are served from this origin rather than a font CDN: a library
whose point is carrying no runtime you did not ask for should not phone one on
every visit. IBM Plex is licensed under the SIL Open Font License 1.1, whose
text ships alongside the files in `assets/fonts/LICENSE.txt`.

## Analytics

None. The page makes no third-party request of any kind, and the
Content-Security-Policy in `_headers` says so: `default-src 'none'`, everything
else `'self'`. Adding analytics means naming its hosts there, one directive at
a time, the way openblox.sh does.

## Social preview

`assets/social-preview.png` is 1280×640, set in the same Plex faces as the page
and rendered from them rather than drawn separately. Regenerate it if the
headline or the mark changes.
