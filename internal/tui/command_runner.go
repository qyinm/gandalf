package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/qyinm/gandalf/internal/gandalfcore/setup"
)

type nativeSetupCommandRunner struct {
	homeDir string
}

func (runner nativeSetupCommandRunner) Run(ctx context.Context, command setup.CommandPlan) error {
	cmd := exec.CommandContext(ctx, command.Program, command.Args...)
	cmd.Env = os.Environ()
	if strings.TrimSpace(runner.homeDir) != "" {
		environment := make([]string, 0, len(cmd.Env)+1)
		for _, value := range cmd.Env {
			if !strings.HasPrefix(value, "HOME=") {
				environment = append(environment, value)
			}
		}
		cmd.Env = append(environment, "HOME="+runner.homeDir)
	}
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	detail := strings.TrimSpace(string(output))
	if len(detail) > 800 {
		detail = detail[:800] + "…"
	}
	if detail == "" {
		return fmt.Errorf("run %s: %w", command.Program, err)
	}
	return fmt.Errorf("run %s: %w: %s", command.Program, err, detail)
}
