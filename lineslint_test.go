package lineslint_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/__GH_OWNER__/lineslint"
)

func TestFixtures(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), lineslint.Analyzer,
		"sinks", "sanitizers", "negatives", "flow", "mole", "bytesl")
}
