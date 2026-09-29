package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const goMod = "module fx\n\ngo 1.24\n"

const (
	cleanSrc = `package main

import "strings"

func main() {
	for l := range strings.Lines("a\n") {
		_ = strings.TrimSpace(l) == "a"
	}
}
`
	foundSrc = `package main

import "strings"

func main() {
	for l := range strings.Lines("a\n") {
		_ = l == "a"
	}
}
`
	brokenSrc = "package main\n\nfunc main() { var x int = \"s\"; _ = x }\n"
)

func TestExitCodes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("exit code test runs the built binary through a shell-free path; not run on windows")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go command not found: %v", err)
	}
	bin := filepath.Join(t.TempDir(), "lineslint")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	project := func(src string) string {
		dir := t.TempDir()
		for name, body := range map[string]string{"go.mod": goMod, "main.go": src} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	clean, found, broken := project(cleanSrc), project(foundSrc), project(brokenSrc)

	cases := []struct {
		name       string
		dir        string
		args       []string
		want       int
		stderrHas  string
		stdoutHas  string
		stdoutNone string
	}{
		{name: "clean", dir: clean, args: []string{"./..."}, want: 0},
		{name: "findings", dir: found, args: []string{"./..."}, want: 1, stderrHas: "still ends in"},
		{name: "type error", dir: broken, args: []string{"./..."}, want: 2, stderrHas: "cannot use"},
		{name: "missing package", dir: clean, args: []string{"./nope"}, want: 2},
		{name: "bad flag", dir: clean, args: []string{"-nosuchflag", "./..."}, want: 2, stderrHas: "not defined"},
		{name: "no arguments", dir: clean, args: nil, want: 2},
		{name: "json with findings", dir: found, args: []string{"-json", "./..."}, want: 1, stdoutHas: `"posn"`},
		{name: "json clean", dir: clean, args: []string{"-json", "./..."}, want: 0, stdoutNone: `"posn"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(bin, tc.args...)
			cmd.Dir = tc.dir
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			got := 0
			if ee, ok := err.(*exec.ExitError); ok {
				got = ee.ExitCode()
			} else if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("exit %d, want %d\nstdout: %s\nstderr: %s", got, tc.want, &stdout, &stderr)
			}
			if tc.stderrHas != "" && !strings.Contains(stderr.String(), tc.stderrHas) {
				t.Errorf("stderr lacks %q: %s", tc.stderrHas, &stderr)
			}
			if tc.stdoutHas != "" && !strings.Contains(stdout.String(), tc.stdoutHas) {
				t.Errorf("stdout lacks %q: %s", tc.stdoutHas, &stdout)
			}
			if tc.stdoutNone != "" && strings.Contains(stdout.String(), tc.stdoutNone) {
				t.Errorf("stdout has %q: %s", tc.stdoutNone, &stdout)
			}
		})
	}
}

func TestRemap(t *testing.T) {
	for _, tc := range []struct {
		code int
		json bool
		out  string
		want int
	}{
		{0, false, "", 0}, {3, false, "", 1}, {1, false, "", 2}, {2, false, "", 2}, {-1, false, "", 2},
		{0, true, `{"fx":{}}`, 0}, {0, true, `{"a":{"b":[{"posn":"x"}]}}`, 1},
	} {
		if got := remap(tc.code, tc.json, []byte(tc.out)); got != tc.want {
			t.Errorf("remap(%d, %v, %q) = %d, want %d", tc.code, tc.json, tc.out, got, tc.want)
		}
	}
}

func TestIsVetInvocation(t *testing.T) {
	for args, want := range map[string]bool{
		"-V=full": true, "-flags": true, "/tmp/vet.cfg": true, "./...": false, "-json": false, "": false,
	} {
		var a []string
		if args != "" {
			a = []string{args}
		}
		if got := isVetInvocation(a); got != want {
			t.Errorf("isVetInvocation(%q) = %v, want %v", args, got, want)
		}
	}
}
