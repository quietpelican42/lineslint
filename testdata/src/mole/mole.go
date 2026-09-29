// Package mole reproduces the shape of a real bug in tw93/Mole
// (cmd/analyze/scanner.go): file paths are read back out of command output
// with strings.Lines and passed to os.Lstat without removing the "\n".
package mole

import (
	"os"
	"strings"
)

type entry struct {
	path string
	size int64
}

func largeFiles(output []byte) []entry {
	var found []entry
	for line := range strings.Lines(strings.TrimSpace(string(output))) {
		if line == "" { // want `line is never empty`
			continue
		}
		info, err := os.Lstat(line) // want `os.Lstat cannot use it as a file path`
		if err != nil {
			continue
		}
		found = append(found, entry{path: line, size: info.Size()})
	}
	return found
}

func largeFilesFixed(output []byte) []entry {
	var found []entry
	for line := range strings.Lines(strings.TrimSpace(string(output))) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		info, err := os.Lstat(line)
		if err != nil {
			continue
		}
		found = append(found, entry{path: line, size: info.Size()})
	}
	return found
}
