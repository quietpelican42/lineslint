package bytesl

import (
	"bytes"
	"strconv"
)

func positives(b []byte, m map[string]int) {
	for line := range bytes.Lines(b) {
		if string(line) == "beta" { // want `string\(line\) still ends`
			return
		}
		if bytes.Equal(line, []byte("beta")) { // want `bytes.Equal with "beta"`
			return
		}
		if bytes.HasSuffix(line, []byte(".go")) { // want `bytes.HasSuffix with ".go"`
			return
		}
		m[string(line)]++          // want `map key`
		strconv.Atoi(string(line)) // want `strconv.Atoi`
	}
}

func negatives(b []byte, m map[string]int) {
	for line := range bytes.Lines(b) {
		trimmed := bytes.TrimSpace(line)
		if string(trimmed) == "beta" || bytes.Equal(trimmed, []byte("beta")) {
			return
		}
		m[string(bytes.TrimSuffix(line, []byte("\n")))]++
		if bytes.Equal(line, []byte("beta\n")) {
			return
		}
		strconv.Atoi(string(bytes.TrimRight(line, "\r\n")))
	}
}
