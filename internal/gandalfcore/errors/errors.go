package errors

import (
	"fmt"
	"strings"

	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

func FormatSnapError(err types.SnapError) string {
	lines := []string{
		err.Code,
		fmt.Sprintf("Problem: %s", err.Problem),
		fmt.Sprintf("Cause: %s", err.Cause),
		fmt.Sprintf("Fix: %s", err.Fix),
	}
	if err.Path != nil {
		lines = append(lines, fmt.Sprintf("Path: %s", *err.Path))
	}
	return strings.Join(lines, "\n") + "\n"
}
