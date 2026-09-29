package negatives

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func correctCompare(s string) {
	for line := range strings.Lines(s) {
		if line == "beta\n" || line == "beta\r\n" {
			return
		}
		switch line {
		case "a\n", "b\n":
		}
	}
}

func writeThrough(s string) string {
	var buf bytes.Buffer
	var sb strings.Builder
	for line := range strings.Lines(s) {
		buf.WriteString(line)
		sb.WriteString(line)
		fmt.Fprint(&buf, line)
		os.WriteFile("out.txt", []byte(line), 0o644)
	}
	return buf.String() + sb.String()
}

func harmless(s string) int {
	n := 0
	for line := range strings.Lines(s) {
		if strings.HasPrefix(line, "#") || strings.Contains(line, "x") {
			n += len(line)
		}
		if strings.HasSuffix(line, "\n") || strings.HasSuffix(line, "\r\n") {
			n++
		}
		if len(line) == 1 {
			n++
		}
		if line[len(line)-1] == '\n' {
			n++
		}
	}
	return n
}

func helper(string) {}

func unclassified(s, other string, m map[string]int) {
	for line := range strings.Lines(s) {
		helper(line)       // interprocedural flows are not followed
		if line == other { // not a constant
		}
		m[fmt.Sprint(line)]++ // Sprint result is unclassified
	}
}

func reassignedClean(s string) {
	for line := range strings.Lines(s) {
		line = strings.TrimSpace(line)
		line = "prefix/" + line
		os.Stat(line)
	}
}

func conditionalTrim(s string, cond bool) {
	for line := range strings.Lines(s) {
		if cond {
			line = strings.TrimSpace(line)
		}
		// Clean on one path, so not reported.
		os.Stat(line)
	}
}

func branchThatReturns(s string) {
	for line := range strings.Lines(s) {
		if strings.HasSuffix(line, "\n") {
			line = strings.TrimSuffix(line, "\n")
		} else {
			continue
		}
		os.Stat(line)
	}
}

func trimmedInput(s string) {
	// The lines still carry the terminator here, but the loop trims first.
	for line := range strings.Lines(s) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		n, err := strconv.Atoi(trimmed)
		_, _ = n, err
	}
}

func shadowed(s string) {
	for line := range strings.Lines(s) {
		{
			line := strings.TrimSpace(line)
			if line == "beta" {
				return
			}
		}
	}
}

func notLines(s string) {
	// Split elements have no terminator.
	for _, line := range strings.Split(s, "\n") {
		if line == "beta" {
			return
		}
		os.Stat(line)
	}
	for line := range strings.SplitSeq(s, "\n") {
		if line == "beta" {
			return
		}
	}
}

func ownLines(s string) {
	Lines := func(string) []string { return nil }
	for _, line := range Lines(s) {
		if line == "beta" {
			return
		}
	}
}

func addressTaken(s string) {
	for line := range strings.Lines(s) {
		p := &line
		*p = strings.TrimSpace(*p)
		if line == "beta" {
			return
		}
	}
}

func closureReassigned(s string) {
	for line := range strings.Lines(s) {
		f := func() bool { return line == "beta" }
		line = strings.TrimSpace(line)
		_ = f()
	}
}

func compareVariable(s string, m map[string]bool) {
	for line := range strings.Lines(s) {
		for k := range m {
			if line == k {
				return
			}
		}
	}
}

func cutWithNewline(s string) {
	for line := range strings.Lines(s) {
		before, after, _ := strings.Cut(line, "\n")
		if after == "x" || before == "x" {
			return
		}
	}
}

func byteIndexing(s string) {
	for line := range strings.Lines(s) {
		if line[0] == 'a' {
			continue
		}
	}
}
