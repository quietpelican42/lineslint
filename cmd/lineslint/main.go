// Command lineslint reports strings.Lines and bytes.Lines elements that are
// used as if the trailing newline were absent.
//
// It accepts every flag of a go/analysis singlechecker and can also run as
// "go vet -vettool=$(which lineslint)". Exit codes: 0 no findings, 1 findings,
// 2 the packages could not be analyzed or the command line was invalid.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/tools/go/analysis/singlechecker"

	"github.com/__GH_OWNER__/lineslint"
)

// innerEnv marks the child process that runs the real singlechecker.
const innerEnv = "LINESLINT_INNER"

func main() {
	if os.Getenv(innerEnv) != "" || isVetInvocation(os.Args[1:]) {
		singlechecker.Main(lineslint.Analyzer)
		return
	}
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// isVetInvocation reports whether go vet is driving this process through the
// unitchecker protocol, where the driver interprets the native exit status.
func isVetInvocation(args []string) bool {
	if len(args) == 0 {
		return false
	}
	if args[0] == "-flags" || strings.HasPrefix(args[0], "-V=") {
		return true
	}
	return len(args) == 1 && strings.HasSuffix(args[0], ".cfg")
}

// run executes the singlechecker in a child process and maps its native exit
// status (0 clean, 3 diagnostics, 1 load or type errors, 2 bad flags) to the
// 0, 1, 2 convention.
func run(args []string, stdout, stderr io.Writer) int {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "lineslint: %v\n", err)
		return 2
	}
	cmd := exec.Command(exe, args...)
	cmd.Env = append(os.Environ(), innerEnv+"=1")
	cmd.Stderr = stderr
	jsonMode := hasBoolFlag(args, "json")
	var captured bytes.Buffer
	if jsonMode {
		cmd.Stdout = io.MultiWriter(stdout, &captured)
	} else {
		cmd.Stdout = stdout
	}
	err = cmd.Run()
	code := 0
	var ee *exec.ExitError
	switch {
	case errors.As(err, &ee):
		code = ee.ExitCode()
	case err != nil:
		fmt.Fprintf(stderr, "lineslint: %v\n", err)
		return 2
	}
	return remap(code, jsonMode, captured.Bytes())
}

// remap converts the singlechecker exit status. In -json mode the native
// status is 0 even with findings, so the output is inspected instead.
func remap(code int, jsonMode bool, out []byte) int {
	switch code {
	case 0:
		if jsonMode && bytes.Contains(out, []byte(`"posn"`)) {
			return 1
		}
		return 0
	case 3:
		return 1
	default:
		return 2
	}
}

func hasBoolFlag(args []string, name string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		a = strings.TrimLeft(a, "-")
		if a == name || a == name+"=true" {
			return true
		}
	}
	return false
}
