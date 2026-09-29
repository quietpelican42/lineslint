package sinks

import (
	"net"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func compare(s string) bool {
	for line := range strings.Lines(s) {
		if line == "beta" { // want `line still ends in "\\n".*never equals "beta"`
			return true
		}
		if "gamma" != line { // want `never equal to "gamma"`
			return false
		}
		if line == "" { // want `line is never empty`
			return false
		}
	}
	return false
}

func switches(s string) int {
	n := 0
	for line := range strings.Lines(s) {
		switch line { // want `cannot match case "start"`
		case "start":
			n++
		case "stop":
			n--
		}
	}
	return n
}

func maps(s string) {
	set := map[string]bool{}
	lit := map[string]int{}
	for line := range strings.Lines(s) {
		set[line] = true // want `map key`
		if set[line] {   // want `map key`
			delete(set, line) // want `map key`
		}
		lit = map[string]int{line: 1} // want `map key`
	}
	_ = lit
}

func paths(s string) {
	for line := range strings.Lines(s) {
		os.Stat(line)                            // want `os.Stat cannot use it as a file path`
		os.Lstat(line)                           // want `os.Lstat`
		os.Open(line)                            // want `os.Open`
		os.ReadFile(line)                        // want `os.ReadFile`
		os.Remove(line)                          // want `os.Remove`
		os.MkdirAll(line, 0o755)                 // want `os.MkdirAll`
		os.Rename("a", line)                     // want `os.Rename`
		filepath.Join("/tmp", line)              // want `filepath.Join`
		filepath.Base(line)                      // want `filepath.Base`
		filepath.Ext(line)                       // want `filepath.Ext`
		filepath.Abs(line)                       // want `filepath.Abs`
		os.WriteFile("out", []byte(line), 0o644) // no finding: the line is the data, not the path
	}
}

func parsers(s string) {
	for line := range strings.Lines(s) {
		strconv.Atoi(line)             // want `strconv.Atoi cannot use it as a number to parse`
		strconv.ParseInt(line, 10, 64) // want `strconv.ParseInt`
		strconv.ParseFloat(line, 64)   // want `strconv.ParseFloat`
		strconv.ParseBool(line)        // want `strconv.ParseBool`
		net.ParseIP(line)              // want `net.ParseIP`
		net.ParseCIDR(line)            // want `net.ParseCIDR`
		netip.ParseAddr(line)          // want `netip.ParseAddr`
		url.Parse(line)                // want `url.Parse`
	}
}

func execs(s string) {
	for line := range strings.Lines(s) {
		exec.Command(line)                // want `exec.Command`
		exec.Command("git", "show", line) // want `exec.Command`
		exec.LookPath(line)               // want `exec.LookPath`
	}
}

func suffix(s string) int {
	n := 0
	for line := range strings.Lines(s) {
		if strings.HasSuffix(line, ".go") { // want `strings.HasSuffix with ".go"`
			n++
		}
		if strings.EqualFold(line, "END") { // want `strings.EqualFold`
			n++
		}
	}
	return n
}
