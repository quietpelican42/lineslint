package lineslint_test

import (
	"bytes"
	"net"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// These tests run the real standard library on the exact patterns the
// analyzer reports, to show each class of finding is a genuine misbehaviour,
// and that the trimmed form behaves. They do not exercise the analyzer.

// firstLine returns the first line of text, as yielded by strings.Lines.
func firstLine(t *testing.T, text string) string {
	t.Helper()
	for line := range strings.Lines(text) {
		return line
	}
	t.Fatal("no lines")
	return ""
}

func TestLinesKeepTerminator(t *testing.T) {
	var got []string
	for line := range strings.Lines("alpha\r\nbeta\ngamma") {
		got = append(got, line)
	}
	want := []string{"alpha\r\n", "beta\n", "gamma"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("strings.Lines = %q, want %q", got, want)
	}
	var bgot []string
	for line := range bytes.Lines([]byte("alpha\nbeta")) {
		bgot = append(bgot, string(line))
	}
	if len(bgot) != 2 || bgot[0] != "alpha\n" || bgot[1] != "beta" {
		t.Fatalf("bytes.Lines = %q", bgot)
	}
	for line := range strings.Lines("\n\n") {
		if line == "" {
			t.Fatal("strings.Lines yielded an empty string")
		}
	}
}

func TestGroundTruthCompareSwitchSuffix(t *testing.T) {
	const text = "alpha\nbeta\ngamma\n"
	matches, trimmedMatches, switchHits, suffixHits := 0, 0, 0, 0
	for line := range strings.Lines(text) {
		if line == "beta" {
			matches++
		}
		if strings.TrimSpace(line) == "beta" {
			trimmedMatches++
		}
		switch line {
		case "alpha", "beta", "gamma":
			switchHits++
		}
		if strings.HasSuffix(line, "a") {
			suffixHits++
		}
	}
	if matches != 0 || switchHits != 0 || suffixHits != 0 {
		t.Errorf("expected misbehaviour: == %d, switch %d, HasSuffix %d (all should be 0)", matches, switchHits, suffixHits)
	}
	if trimmedMatches != 1 {
		t.Errorf("trimmed comparison matched %d times, want 1", trimmedMatches)
	}
}

func TestGroundTruthEmptyComparison(t *testing.T) {
	blank := 0
	for line := range strings.Lines("a\n\nb\n") {
		if line == "" {
			blank++
		}
	}
	if blank != 0 {
		t.Fatalf("line == \"\" matched %d blank lines, want 0", blank)
	}
}

func TestGroundTruthMapKey(t *testing.T) {
	seen := map[string]bool{"alpha": true}
	hits := 0
	for line := range strings.Lines("alpha\nbeta\n") {
		if seen[line] {
			hits++
		}
	}
	if hits != 0 {
		t.Fatalf("map lookup with a raw line hit %d times, want 0", hits)
	}
	// The last, unterminated line does match: the same text is found or missed
	// depending on where it sits.
	last := 0
	for line := range strings.Lines("beta\nalpha") {
		if seen[line] {
			last++
		}
	}
	if last != 1 {
		t.Fatalf("unterminated last line matched %d times, want 1", last)
	}
}

func TestGroundTruthPaths(t *testing.T) {
	dir := t.TempDir()
	var names []string
	for _, n := range []string{"a.txt", "b.txt", "c.txt"} {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		names = append(names, p)
	}
	text := strings.Join(names, "\n")
	raw, trimmed := 0, 0
	for line := range strings.Lines(text) {
		if _, err := os.Lstat(line); err == nil {
			raw++
		}
		if _, err := os.Lstat(strings.TrimSuffix(line, "\n")); err == nil {
			trimmed++
		}
	}
	if raw != 1 || trimmed != 3 {
		t.Errorf("Lstat found %d raw, %d trimmed; want 1 and 3", raw, trimmed)
	}

	line := firstLine(t, text)
	if _, err := os.Stat(line); err == nil {
		t.Error("os.Stat succeeded on a line ending in newline")
	}
	if _, err := os.Open(line); err == nil {
		t.Error("os.Open succeeded on a line ending in newline")
	}
	if _, err := os.ReadFile(line); err == nil {
		t.Error("os.ReadFile succeeded on a line ending in newline")
	}
	if got := filepath.Base(line); !strings.HasSuffix(got, "\n") {
		t.Errorf("filepath.Base dropped the newline: %q", got)
	}
	if got := filepath.Ext(line); got != ".txt\n" {
		t.Errorf("filepath.Ext = %q, want %q", got, ".txt\n")
	}
	if err := os.Remove(line); err == nil {
		t.Error("os.Remove succeeded on a line ending in newline")
	}
	if _, err := os.Stat(names[0]); err != nil {
		t.Errorf("the file must still exist after the failed Remove: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub"+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		found = found || e.Name() == "sub\n"
	}
	if !found {
		t.Error("MkdirAll with a newline-terminated element did not create a directory named with a newline")
	}
}

func TestGroundTruthNumbers(t *testing.T) {
	cases := []struct {
		name string
		call func(string) error
		ok   string
	}{
		{"Atoi", func(s string) error { _, err := strconv.Atoi(s); return err }, "42"},
		{"ParseInt", func(s string) error { _, err := strconv.ParseInt(s, 10, 64); return err }, "42"},
		{"ParseUint", func(s string) error { _, err := strconv.ParseUint(s, 10, 64); return err }, "42"},
		{"ParseFloat", func(s string) error { _, err := strconv.ParseFloat(s, 64); return err }, "4.2"},
		{"ParseBool", func(s string) error { _, err := strconv.ParseBool(s); return err }, "true"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			line := firstLine(t, tc.ok+"\nnext\n")
			if err := tc.call(line); err == nil {
				t.Errorf("%s accepted %q", tc.name, line)
			}
			if err := tc.call(strings.TrimSpace(line)); err != nil {
				t.Errorf("%s rejected the trimmed line: %v", tc.name, err)
			}
		})
	}
}

func TestGroundTruthNetworkParsers(t *testing.T) {
	ip := firstLine(t, "192.0.2.1\nnext\n")
	if net.ParseIP(ip) != nil {
		t.Error("net.ParseIP accepted a line ending in newline")
	}
	if net.ParseIP(strings.TrimSpace(ip)) == nil {
		t.Error("net.ParseIP rejected the trimmed line")
	}
	if _, err := netip.ParseAddr(ip); err == nil {
		t.Error("netip.ParseAddr accepted a line ending in newline")
	}
	cidr := firstLine(t, "192.0.2.0/24\nnext\n")
	if _, _, err := net.ParseCIDR(cidr); err == nil {
		t.Error("net.ParseCIDR accepted a line ending in newline")
	}
	if _, _, err := net.ParseCIDR(strings.TrimSpace(cidr)); err != nil {
		t.Errorf("net.ParseCIDR rejected the trimmed line: %v", err)
	}
	raw := firstLine(t, "http://example.com/a\nnext\n")
	if _, err := url.Parse(raw); err == nil {
		t.Error("url.Parse accepted a line ending in newline")
	}
	if _, err := url.Parse(strings.TrimSpace(raw)); err != nil {
		t.Errorf("url.Parse rejected the trimmed line: %v", err)
	}
	if _, err := url.ParseRequestURI(raw); err == nil {
		t.Error("url.ParseRequestURI accepted a line ending in newline")
	}
}

func TestGroundTruthExec(t *testing.T) {
	name := firstLine(t, "true\nfalse\n")
	if _, err := exec.LookPath(name); err == nil {
		t.Skip("a command literally named \"true\\n\" exists on this machine")
	}
	if err := exec.Command(name).Run(); err == nil {
		t.Error("exec.Command ran a command named with a trailing newline")
	}
	if _, err := exec.LookPath("true"); err != nil {
		t.Skipf("no true(1) on PATH: %v", err)
	}
	if err := exec.Command(strings.TrimSpace(name)).Run(); err != nil {
		t.Errorf("exec.Command with the trimmed name failed: %v", err)
	}
	// As an argument the newline reaches the child verbatim.
	if _, err := exec.LookPath("printf"); err != nil {
		t.Skipf("no printf on PATH: %v", err)
	}
	arg := firstLine(t, "hello\nnext\n")
	out, err := exec.Command("printf", "%s|", arg).Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "hello\n|" {
		t.Errorf("child received %q, want %q", out, "hello\n|")
	}
}

func TestGroundTruthBytesLines(t *testing.T) {
	for line := range bytes.Lines([]byte("beta\nx\n")) {
		if string(line) == "beta" || bytes.Equal(line, []byte("beta")) || bytes.HasSuffix(line, []byte("a")) {
			t.Fatalf("bytes.Lines element %q behaved as if trimmed", line)
		}
		if !bytes.Equal(bytes.TrimSpace(line), []byte("beta")) {
			t.Fatalf("trimmed bytes line %q != beta", line)
		}
		break
	}
}

func TestGroundTruthOtherOSFunctions(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("src", filepath.Join(dir, "ln")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	// Functions that fail on a name ending in newline.
	fails := map[string]func(string) error{
		"Chmod":    func(n string) error { return os.Chmod(n, 0o600) },
		"Truncate": func(n string) error { return os.Truncate(n, 0) },
		"Readlink": func(n string) error { _, err := os.Readlink(n); return err },
		"Chdir":    func(n string) error { return os.Chdir(n) },
		"RemoveAll-then-Stat": func(n string) error {
			_ = os.RemoveAll(n)
			_, err := os.Stat(n)
			return err
		},
	}
	for name, f := range fails {
		var target string
		switch name {
		case "Chmod", "Truncate":
			target = firstLine(t, src+"\nnext")
		case "Readlink":
			target = firstLine(t, filepath.Join(dir, "ln")+"\nnext")
		default:
			target = firstLine(t, filepath.Join(dir, "d")+"\nnext")
		}
		if err := f(target); err == nil {
			t.Errorf("os.%s succeeded on %q", name, target)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "d")); err != nil {
		t.Errorf("os.RemoveAll(line) must not have removed the real directory: %v", err)
	}
	// Functions that succeed, but on the wrong name.
	wrong := firstLine(t, filepath.Join(dir, "new")+"\nnext")
	f, err := os.Create(wrong)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err := os.Stat(filepath.Join(dir, "new")); err == nil {
		t.Error("os.Create(line) created the intended name")
	}
	if err := os.Rename(src, wrong+"2"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "new2")); err == nil {
		t.Error("os.Rename(_, line) produced the intended name")
	}
	if err := os.WriteFile(wrong+"3", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "new3")); err == nil {
		t.Error("os.WriteFile(line, ...) produced the intended name")
	}
	if got := filepath.Clean(wrong); !strings.HasSuffix(got, "new\n") {
		t.Errorf("filepath.Clean dropped the newline: %q", got)
	}
	if got := filepath.Join(dir, "x\n"); !strings.HasSuffix(got, "\n") {
		t.Errorf("filepath.Join dropped the newline: %q", got)
	}
	if strings.EqualFold("END\n", "END") {
		t.Error("EqualFold matched a line with a newline")
	}
}

func TestGroundTruthFilepathWalkers(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	line := firstLine(t, filepath.Join(dir, "f")+"\nnext")
	if _, err := filepath.EvalSymlinks(line); err == nil {
		t.Error("EvalSymlinks succeeded on a line ending in newline")
	}
	if m, err := filepath.Glob(line); err != nil || len(m) != 0 {
		t.Errorf("Glob(line) = %v, %v; want no matches", m, err)
	}
	var walkErr error
	filepath.WalkDir(line, func(_ string, _ os.DirEntry, err error) error { walkErr = err; return err })
	if walkErr == nil {
		t.Error("WalkDir succeeded on a line ending in newline")
	}
	walkErr = nil
	filepath.Walk(line, func(_ string, _ os.FileInfo, err error) error { walkErr = err; return err })
	if walkErr == nil {
		t.Error("Walk succeeded on a line ending in newline")
	}
	rel, err := filepath.Rel(dir, line)
	if err != nil || rel != "f\n" {
		t.Errorf("Rel = %q, %v; want %q", rel, err, "f\n")
	}
	if abs, err := filepath.Abs(line); err != nil || !strings.HasSuffix(abs, "f\n") {
		t.Errorf("Abs = %q, %v; want trailing newline", abs, err)
	}
	if got := filepath.Dir(line + "/x"); !strings.HasSuffix(got, "\n") {
		t.Errorf("Dir = %q", got)
	}
	if err := os.Symlink(line, filepath.Join(dir, "l")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "l")); err == nil {
		t.Error("a symlink whose target ends in newline resolved")
	}
}
