package tar_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/tar"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

func TestValidateTarPathRejectsTraversal(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cases := []string{"../escape", "foo/../../etc/passwd", "/absolute/path"}
	for _, entryPath := range cases {
		if _, err := tar.ValidateTarPath(entryPath, root); err == nil {
			t.Fatalf("expected error for %q", entryPath)
		}
	}
}

func TestWriteAndReadTarRoundTrip(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	output := filepath.Join(root, "test.gandalf")
	entries := []types.TarEntry{
		{Path: ".gandalf/format-version", Content: []byte("1\n"), Mode: 0o644, Mtime: 1000, EntryType: types.TarEntryFile},
		{Path: "snapshot/", Content: nil, Mode: 0o755, Mtime: 1000, EntryType: types.TarEntryDirectory},
	}
	checksum, err := tar.WriteTar(entries, output)
	if err != nil {
		t.Fatalf("WriteTar: %v", err)
	}
	if checksum == "" {
		t.Fatal("expected archive checksum")
	}
	readEntries, readChecksum, err := tar.ReadTar(output)
	if err != nil {
		t.Fatalf("ReadTar: %v", err)
	}
	if readChecksum != checksum {
		t.Fatalf("checksum mismatch: %s vs %s", readChecksum, checksum)
	}
	if len(readEntries) != 2 {
		t.Fatalf("entries = %d", len(readEntries))
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatal(err)
	}
}
