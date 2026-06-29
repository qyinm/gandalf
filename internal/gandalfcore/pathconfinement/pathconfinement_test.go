package pathconfinement_test

import (
	"strings"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/pathconfinement"
)

func TestRejectsTraversalInDest(t *testing.T) {
	t.Parallel()
	roots := &pathconfinement.Roots{
		HomeDir:     "/home/user",
		ProjectPath: "/home/user/project",
	}
	_, err := pathconfinement.ValidateConstrainedWritePath("/home/user/../../etc/passwd", roots)
	if err == nil || !strings.Contains(err.Error(), "traversal") {
		t.Fatalf("expected traversal error, got %v", err)
	}
}

func TestAllowsPathsUnderHome(t *testing.T) {
	t.Parallel()
	roots := &pathconfinement.Roots{
		HomeDir:     "/home/user",
		ProjectPath: "/home/user/project",
	}
	got, err := pathconfinement.ValidateConstrainedWritePath("/home/user/.codex/config.toml", roots)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/home/user/.codex/config.toml" {
		t.Fatalf("got %q", got)
	}
}

func TestRootsFromPathsCleansRootsBeforeValidation(t *testing.T) {
	t.Parallel()
	home := "/tmp//gandalf/home"
	project := "/tmp//gandalf/project"
	roots := pathconfinement.RootsFromPaths(&home, &project)
	got, err := pathconfinement.ValidateConstrainedWritePath("/tmp/gandalf/home/.codex/config.toml", roots)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/tmp/gandalf/home/.codex/config.toml" {
		t.Fatalf("got %q", got)
	}
}

func TestValidateHomeRelativeImportSegment(t *testing.T) {
	t.Parallel()
	if err := pathconfinement.ValidateHomeRelativeImportSegment(".codex/config.toml"); err != nil {
		t.Fatalf("expected ok: %v", err)
	}
	if err := pathconfinement.ValidateHomeRelativeImportSegment("../secret"); err == nil {
		t.Fatal("expected traversal error")
	}
}
