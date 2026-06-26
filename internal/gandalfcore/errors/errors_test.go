package errors_test

import (
	"strings"
	"testing"

	hemerrors "github.com/qyinm/gandalf/internal/gandalfcore/errors"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

func TestFormatSnapErrorMatchesContract(t *testing.T) {
	t.Parallel()
	path := "~/.codex/config.toml"
	out := hemerrors.FormatSnapError(types.SnapError{
		Code:    "GANDALF_PARSE_FAILED",
		Problem: "Could not parse Codex config.",
		Cause:   "TOML syntax error at line 12.",
		Fix:     "Run `gandalf scan --skip codex` or fix the TOML file.",
		Path:    &path,
	})
	if !strings.HasPrefix(out, "GANDALF_PARSE_FAILED") {
		t.Fatalf("prefix: %q", out)
	}
	for _, want := range []string{
		"Problem: Could not parse Codex config.",
		"Cause: TOML syntax error at line 12.",
		"Fix: Run `gandalf scan --skip codex` or fix the TOML file.",
		"Path: ~/.codex/config.toml",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}
