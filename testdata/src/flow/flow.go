package flow

import (
	"iter"
	"os"
	"slices"
	"strconv"
	"strings"
)

func assignChain(s string) {
	for line := range strings.Lines(s) {
		l := line
		var v = l
		w, x := v, "k"
		if w == "beta" { // want `w still ends`
			return
		}
		_ = x
	}
}

func concat(s string) {
	for line := range strings.Lines(s) {
		p := "dir/" + line
		os.Stat(p) // want `os.Stat`
		q := "n"
		q += line
		os.Stat(q) // want `os.Stat`
		r := line + "/suffix"
		os.Stat(r) // the terminator is in the middle: not classified
	}
}

func preservingCalls(s string) {
	for line := range strings.Lines(s) {
		if strings.ToLower(line) == "beta" { // want `strings.ToLower\(line\) still ends`
			return
		}
		if strings.TrimPrefix(line, "#") == "beta" { // want `never equals "beta"`
			return
		}
		if strings.TrimSuffix(line, ".go") == "beta" { // want `never equals "beta"`
			return
		}
		if strings.Trim(line, " ") == "beta" { // want `never equals "beta"`
			return
		}
		if strings.TrimRight(line, " \t") == "beta" { // want `never equals "beta"`
			return
		}
		if line[1:] == "beta" { // want `never equals "beta"`
			return
		}
		if strings.Clone(line) == "beta" { // want `never equals "beta"`
			return
		}
	}
}

func cuts(s string) {
	for line := range strings.Lines(s) {
		key, val, ok := strings.Cut(line, "=")
		if ok && val == "true" { // want `val still ends`
			return
		}
		if key == "name" { // the key is the part before the separator: not classified
			return
		}
		rest, found := strings.CutPrefix(line, "set ")
		if found && rest == "x" { // want `rest still ends`
			return
		}
	}
}

func storedSeq(s string) {
	seq := strings.Lines(s)
	for l := range seq {
		if l == "beta" { // want `l still ends`
			return
		}
	}
}

func typedStoredSeq(s string) {
	var seq iter.Seq[string] = strings.Lines(s)
	for l := range seq {
		if l == "beta" { // want `l still ends`
			return
		}
	}
}

func directCall(s string) {
	strings.Lines(s)(func(l string) bool {
		if l == "beta" { // want `l still ends`
			return false
		}
		return true
	})
}

func collected(s string) {
	for _, l := range slices.Collect(strings.Lines(s)) {
		if l == "beta" { // want `l still ends`
			return
		}
	}
	all := slices.Collect(strings.Lines(s))
	for _, l := range all {
		os.Stat(l) // want `os.Stat`
	}
}

func closures(s string) {
	for line := range strings.Lines(s) {
		func() {
			os.Stat(line) // want `os.Stat`
		}()
		defer func() {
			strconv.Atoi(line) // want `strconv.Atoi`
		}()
	}
}

func branches(s string, c bool) {
	for line := range strings.Lines(s) {
		if c {
			_ = len(line)
		} else {
			_ = 1
		}
		os.Stat(line) // want `os.Stat`
	}
	for line := range strings.Lines(s) {
		if c {
			line = strings.ToUpper(line)
		} else {
			line = strings.TrimPrefix(line, "x")
		}
		os.Stat(line) // want `os.Stat`
	}
}

func switchClauses(s string, n int) {
	for line := range strings.Lines(s) {
		switch n {
		case 1:
			_ = line
		default:
			_ = n
		}
		os.Stat(line) // want `os.Stat`
	}
}

func nested(s, t string) {
	for a := range strings.Lines(s) {
		for b := range strings.Lines(t) {
			if a == b {
				continue
			}
			if b == "x" { // want `b still ends`
				continue
			}
		}
		if a == "y" { // want `a still ends`
			continue
		}
	}
}

func loopKeepsDirty(s string) {
	for line := range strings.Lines(s) {
		for i := 0; i < 2; i++ {
			_ = i
		}
		if line == "y" { // want `line still ends`
			continue
		}
	}
}

func loopCleans(s string) {
	for line := range strings.Lines(s) {
		for i := 0; i < 2; i++ {
			line = strings.TrimSpace(line)
		}
		if line == "y" { // the loop may have cleaned it
			continue
		}
	}
}

func switchCleans(s string, n int) {
	for line := range strings.Lines(s) {
		switch n {
		case 1:
			line = strings.TrimSpace(line)
			break
		}
		if line == "y" { // a break path cleaned it
			continue
		}
	}
}
