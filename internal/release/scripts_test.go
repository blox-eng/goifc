// Package release tests the release scripts under .github/scripts. It holds
// tests only.
package release

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func script(t *testing.T, name string, args ...string) (string, error) {
	t.Helper()
	out, err := exec.Command(filepath.Join("..", "..", ".github", "scripts", name), args...).CombinedOutput()
	return string(out), err
}

func file(t *testing.T, s string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const site = `<main>
    <div>
      <a class="news" href="#">old</a>
      <h1>Read IFC in Go.</h1>
    </div>
</main>
`

const released = `# Changelog

<!-- Entries go under Unreleased. -->

## Unreleased

- **Not released yet.**

## v0.4.0 — 2026-11-12

### Added

- **Faster parsing.** More text.
- **A second bold run.**

## v0.3.0 — 2026-11-01

- **Older <news> & "more".**
`

func TestAnnounceWritesTheLine(t *testing.T) {
	cl, page := file(t, released), file(t, site)
	if out, err := script(t, "announce-release.sh", "0.4.0", cl, page); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	want := `      <a class="news" href="https://github.com/blox-eng/goifc/releases/tag/v0.4.0"><span class="tag">New</span> <span>v0.4.0 · Faster parsing</span> <span class="go">Release notes →</span></a>` + "\n"
	got := read(t, page)
	if !strings.Contains(got, want) || !strings.HasPrefix(got, "<main>\n") || !strings.HasSuffix(got, "</main>\n") {
		t.Fatalf("got\n%s\nwant the line\n%s", got, want)
	}
}

func TestAnnounceEscapesTheHeadline(t *testing.T) {
	page := file(t, site)
	if out, err := script(t, "announce-release.sh", "0.3.0", file(t, released), page); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if got := read(t, page); !strings.Contains(got, "v0.3.0 · Older &lt;news&gt; &amp; &quot;more&quot;<") {
		t.Fatalf("not escaped:\n%s", got)
	}
}

// A headline is the first bold run of the section, even when it wraps or has
// code in it; the code marks go, the words stay.
func TestAnnounceTakesTheWholeHeadline(t *testing.T) {
	for changelog, want := range map[string]string{
		"## v0.6.0 — 2026-11-13\n\n- **A wall that\n  wraps.** More.\n":           "v0.6.0 · A wall that wraps<",
		"## v0.6.0 — 2026-11-13\n\n- **Call `BuildSlab` once.** More **bold**.\n": "v0.6.0 · Call BuildSlab once<",
		"## v0.6.0\n\n- **No date on the heading.**\n":                            "v0.6.0 · No date on the heading<",
	} {
		page := file(t, site)
		if out, err := script(t, "announce-release.sh", "0.6.0", file(t, changelog), page); err != nil || !strings.Contains(read(t, page), want) {
			t.Fatalf("%q: %v %s\n%s", changelog, err, out, read(t, page))
		}
	}
}

// A section without a headline, a page without exactly one news line, or a
// malformed version leaves the page as it is and fails.
func TestAnnounceRefuses(t *testing.T) {
	for name, c := range map[string]struct{ version, changelog, page string }{
		"no such section":          {"0.5.0", released, site},
		"no bold bullet":           {"0.4.0", "## v0.4.0 — 2026-11-12\n\n- A plain line.\n\n## v0.3.0 — 2026-11-01\n\n- **Not this one.**\n", site},
		"0.4.0 is not 0.4.00":      {"0.4.0", "## v0.4.00 — 2026-11-12\n\n- **Wrong release.**\n", site},
		"no news line":             {"0.4.0", released, "<main></main>\n"},
		"two news lines":           {"0.4.0", released, site + `<a class="news" href="#">x</a>` + "\n"},
		"markup in the version":    {`0.4.0"><script>`, released, site},
		"a version with no number": {"latest", released, site},
	} {
		page := file(t, c.page)
		out, err := script(t, "announce-release.sh", c.version, file(t, c.changelog), page)
		if err == nil || !strings.Contains(out, "announce-release:") {
			t.Errorf("%s: no refusal (%v): %s", name, err, out)
		}
		if read(t, page) != c.page {
			t.Errorf("%s: the page changed", name)
		}
	}
}

// The guard takes the newest release from the changelog, skipping
// Unreleased, and passes only the line announce-release.sh writes for it.
func TestNewsGuard(t *testing.T) {
	cl, page := file(t, released), file(t, site)
	out, err := script(t, "news-guard.sh", cl, page)
	if err == nil || !strings.Contains(out, "announce-release.sh 0.4.0") {
		t.Fatalf("a stale line passed or named no fix: %v %s", err, out)
	}
	if read(t, page) != site {
		t.Fatal("the guard changed the page")
	}
	if out, err := script(t, "announce-release.sh", "0.4.0", cl, page); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if out, err := script(t, "news-guard.sh", cl, page); err != nil {
		t.Fatalf("an announced line failed: %v %s", err, out)
	}
	if out, err := script(t, "news-guard.sh", file(t, "## Unreleased\n\n- **Soon.**\n"), page); err == nil || !strings.Contains(out, "news-guard:") {
		t.Fatalf("a changelog with no release passed: %v %s", err, out)
	}
}

// A patch release often has no headline. The line keeps naming the newest
// release that has one, and the guard passes that.
func TestNewsGuardKeepsTheLastHeadline(t *testing.T) {
	cl := file(t, "## Unreleased\n\n## v0.4.1 — 2026-11-13\n\n### Fixed\n\n- A plain fix.\n\n"+released[strings.Index(released, "## v0.4.0"):])
	page := file(t, site)
	if out, err := script(t, "announce-release.sh", "0.4.0", cl, page); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if out, err := script(t, "news-guard.sh", cl, page); err != nil {
		t.Fatalf("the last headline failed the guard: %v %s", err, out)
	}
	stale := file(t, site)
	if out, err := script(t, "announce-release.sh", "0.3.0", cl, stale); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if out, err := script(t, "news-guard.sh", cl, stale); err == nil || !strings.Contains(out, "announce-release.sh 0.4.0") {
		t.Fatalf("a line older than the last headline passed: %v %s", err, out)
	}
}

// The headline is the first bold bullet of the section, not its first
// bullet; quotes and code marks in it are handled.
func TestAnnounceSkipsPlainBullets(t *testing.T) {
	page := file(t, site)
	cl := "## v0.7.0 — 2026-11-13\n\n- A plain line.\n- **Don't call `Old` again.**\n"
	if out, err := script(t, "announce-release.sh", "0.7.0", file(t, cl), page); err != nil || !strings.Contains(read(t, page), "v0.7.0 · Don&#39;t call Old again<") {
		t.Fatalf("%v %s\n%s", err, out, read(t, page))
	}
}

// scriptIn runs a script with dir as its working directory.
func scriptIn(t *testing.T, dir, name string, args ...string) (string, error) {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("..", "..", ".github", "scripts", name))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(abs, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// The guard passes only when the changelog has the tag's own heading: a
// longer version that starts with it does not count.
func TestChangelogGuard(t *testing.T) {
	for name, c := range map[string]struct {
		tag, changelog string
		pass           bool
	}{
		"the heading is there":   {"v0.4.0", released, true},
		"no heading for the tag": {"v0.5.0", released, false},
		"v0.1.1 is not v0.1.10":  {"v0.1.1", "## Unreleased\n\n## v0.1.10 — 2026-01-01\n", false},
		"a pre-release tag":      {"v1.0.0-rc.1", "## v1.0.0-rc.1 — 2026-01-01\n", true},
		"a heading with no date": {"v0.6.0", "## v0.6.0\n", true},
		"Unreleased only":        {"v0.1.0", "## Unreleased\n\n- **Soon.**\n", false},
	} {
		out, err := script(t, "changelog-guard.sh", c.tag, file(t, c.changelog))
		if c.pass && err != nil {
			t.Errorf("%s: failed: %v %s", name, err, out)
		}
		if !c.pass && (err == nil || !strings.Contains(out, c.tag) || !strings.Contains(out, "make promote")) {
			t.Errorf("%s: passed or named no fix: %v %s", name, err, out)
		}
	}
}

// A clone with no v* tags, such as a fork, has nothing to check.
func TestChangelogGuardNoTags(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if err := os.WriteFile(filepath.Join(dir, "CHANGELOG.md"), []byte(released), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := scriptIn(t, dir, "changelog-guard.sh")
	if err != nil || !strings.Contains(out, "no v* tag") {
		t.Fatalf("%v: %s", err, out)
	}
}
