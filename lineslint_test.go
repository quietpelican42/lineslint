package lineslint_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/quietpelican42/lineslint"
)

func TestFixtures(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), lineslint.Analyzer,
		"sinks", "sanitizers", "negatives", "flow", "mole", "bytesl")
}
