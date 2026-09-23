// Command coverage writes docs/coverage.md from the parity corpus.
//
// With -check it verifies the committed page matches what the code produces now,
// which is how CI catches a published accuracy number that has drifted.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/blox-eng/goifc/parity"
)

func main() {
	check := flag.Bool("check", false, "verify the committed page is up to date instead of writing it")
	flag.Parse()

	want, err := parity.Report()
	if err != nil {
		fmt.Fprintln(os.Stderr, "coverage:", err)
		os.Exit(1)
	}
	path := filepath.Join(parity.Dir(), "..", "docs", "coverage.md")

	if *check {
		got, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "coverage:", err)
			os.Exit(1)
		}
		if string(got) != want {
			fmt.Fprintln(os.Stderr, "coverage: docs/coverage.md is stale — run `make parity-report`")
			os.Exit(1)
		}
		fmt.Println("coverage: docs/coverage.md is up to date")
		return
	}

	if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "coverage:", err)
		os.Exit(1)
	}
	fmt.Println("coverage: wrote", path)
}
