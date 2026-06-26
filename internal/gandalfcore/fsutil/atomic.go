package fsutil

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
)

// WriteFileAtomically writes content to path using a temp file and rename.
func WriteFileAtomically(filePath string, content []byte, perm os.FileMode) error {
	tempPath, err := tempPathFor(filePath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(tempPath, content, perm); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, filePath); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

// WriteTextAtomically writes string content to path using a temp file and rename.
func WriteTextAtomically(filePath, content string, perm os.FileMode) error {
	return WriteFileAtomically(filePath, []byte(content), perm)
}

func tempPathFor(filePath string) (string, error) {
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s.%d.%s.tmp", filePath, os.Getpid(), hex.EncodeToString(suffix[:])), nil
}
