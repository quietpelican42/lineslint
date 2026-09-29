package lineslint_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"

	"github.com/__GH_OWNER__/lineslint"
)

// The real-world bug the analyzer was built around: tw93/Mole at a pinned
// commit passes strings.Lines elements to os.Lstat. The checkout is not part
// of this repository. Fetch it into a gitignored directory and point
// LINESLINT_MOLE_DIR at it:
//
//	mkdir -p .corpus/Mole && cd .corpus/Mole && git init -q &&
//	git remote add origin https://github.com/tw93/Mole.git &&
//	git fetch --depth 1 origin f32fa0c6b082ed76705cbc45f2c42169a09e2503 &&
//	git checkout -q FETCH_HEAD
const (
	moleCommit       = "f32fa0c6b082ed76705cbc45f2c42169a09e2503"
	moleScannerSHA   = "446b6ffc16a103e0eead7f828edae47bb08503ed433c9aab902def4a42444b2c"
	moleScannerPath  = "cmd/analyze/scanner.go"
	moleScannerFirst = 824 // line of the strings.Lines loop
)

func TestPinnedMole(t *testing.T) {
	dir := os.Getenv("LINESLINT_MOLE_DIR")
	if dir == "" {
		t.Skipf("LINESLINT_MOLE_DIR is not set; skipping the pinned tw93/Mole check (commit %s, see mole_test.go for the fetch commands)", moleCommit)
	}
	src, err := os.ReadFile(filepath.Join(dir, moleScannerPath))
	if err != nil {
		t.Fatalf("read %s: %v", moleScannerPath, err)
	}
	sum := sha256.Sum256(src)
	if got := hex.EncodeToString(sum[:]); got != moleScannerSHA {
		t.Fatalf("%s has sha256 %s, want %s (the checkout is not commit %s)", moleScannerPath, got, moleScannerSHA, moleCommit)
	}

	pkgs, err := packages.Load(&packages.Config{Mode: packages.LoadAllSyntax, Dir: dir}, "./cmd/analyze")
	if err != nil {
		t.Fatal(err)
	}
	if n := packages.PrintErrors(pkgs); n > 0 {
		t.Fatalf("%d package errors loading Mole; run `go mod download` in %s", n, dir)
	}
	graph, err := checker.Analyze([]*analysis.Analyzer{lineslint.Analyzer}, pkgs, nil)
	if err != nil {
		t.Fatal(err)
	}
	var lstat, empty bool
	for _, act := range graph.Roots {
		for _, d := range act.Diagnostics {
			pos := act.Package.Fset.Position(d.Pos)
			if filepath.Base(pos.Filename) != "scanner.go" || pos.Line < moleScannerFirst {
				continue
			}
			t.Logf("%s:%d: %s", moleScannerPath, pos.Line, d.Message)
			lstat = lstat || strings.Contains(d.Message, "os.Lstat")
			empty = empty || strings.Contains(d.Message, `never empty`)
		}
	}
	if !lstat {
		t.Error("os.Lstat(line) in the strings.Lines loop was not reported")
	}
	if !empty {
		t.Error(`line == "" in the strings.Lines loop was not reported`)
	}
}
