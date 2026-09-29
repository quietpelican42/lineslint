package sanitizers

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"unicode"
)

// Every sink below sits behind a call that removes the terminator, so no
// diagnostic is expected anywhere in this file.

func trimSpace(s string) {
	for line := range strings.Lines(s) {
		line = strings.TrimSpace(line)
		if line == "beta" {
			return
		}
		os.Stat(line)
	}
}

func trimSpaceInline(s string, m map[string]int) {
	for line := range strings.Lines(s) {
		if strings.TrimSpace(line) == "beta" {
			return
		}
		m[strings.TrimSpace(line)]++
	}
}

func trimSuffixNewline(s string) {
	for line := range strings.Lines(s) {
		name := strings.TrimSuffix(line, "\n")
		os.Open(name)
		crlf := strings.TrimSuffix(line, "\r\n")
		strconv.Atoi(crlf)
		both := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if both == "x" {
			return
		}
	}
}

func trimRight(s string) {
	for line := range strings.Lines(s) {
		a := strings.TrimRight(line, "\r\n")
		b := strings.TrimRight(line, "\n")
		c := strings.Trim(line, " \n")
		if a == "x" || b == "y" || c == "z" {
			return
		}
	}
}

func trimFuncs(s string) {
	for line := range strings.Lines(s) {
		a := strings.TrimFunc(line, unicode.IsSpace)
		b := strings.TrimRightFunc(line, unicode.IsSpace)
		if a == "x" || b == "y" {
			return
		}
	}
}

func cutSuffix(s string) {
	for line := range strings.Lines(s) {
		name, _ := strings.CutSuffix(line, "\n")
		os.Stat(name)
		if name == "beta" {
			return
		}
	}
}

func fields(s string) {
	for line := range strings.Lines(s) {
		f := strings.Fields(line)
		if len(f) > 1 && f[1] == "ok" {
			strconv.Atoi(f[0])
		}
		g := strings.FieldsFunc(line, unicode.IsSpace)
		if g[0] == "x" {
			return
		}
	}
}

// Split elements are not tracked (documented recall limit): the last element
// of Split(line, " ") does keep the newline, but is never reported.
func split(s string) {
	for line := range strings.Lines(s) {
		parts := strings.Split(line, ",")
		if parts[0] == "x" {
			return
		}
		kv := strings.SplitN(line, "=", 2)
		strconv.Atoi(kv[len(kv)-1])
	}
}

func rescan(s string) {
	for line := range strings.Lines(s) {
		sc := bufio.NewScanner(strings.NewReader(line))
		for sc.Scan() {
			if sc.Text() == "beta" {
				return
			}
		}
	}
}

func sliceAfterHasSuffix(s string) {
	for line := range strings.Lines(s) {
		if strings.HasSuffix(line, "\n") {
			line = line[:len(line)-1]
		}
		if line == "beta" {
			return
		}
		os.Stat(line)
	}
}

func sliceCopy(s string) {
	for line := range strings.Lines(s) {
		body := line[:len(line)-1]
		if body == "beta" {
			return
		}
	}
}

func cutSuffixInPlace(s string) {
	for line := range strings.Lines(s) {
		line, _ = strings.CutSuffix(line, "\n")
		if line == "beta" {
			return
		}
	}
}

func trimRightInPlace(s string, m map[string]int) {
	for line := range strings.Lines(s) {
		line = strings.TrimRight(line, "\r\n")
		m[line]++
	}
}

func deferredAfterTrim(s string) {
	for line := range strings.Lines(s) {
		line = strings.TrimSpace(line)
		func() { os.Stat(line) }()
	}
}
