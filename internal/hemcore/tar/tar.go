package tar

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/qyinm/hem/internal/hemcore/types"
)

const (
	blockSize      = 512
	headerSize     = 512
	magic          = "ustar"
	maxEntryBytes  = 500 * 1024 * 1024
)

// Error represents tar read/write failures.
type Error struct {
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

// WriteTar writes a tar archive and returns the SHA-256 hex digest of the archive bytes.
func WriteTar(entries []types.TarEntry, outputPath string) (string, error) {
	var blocks []byte

	for _, entry := range entries {
		header, err := encodeHeader(entry)
		if err != nil {
			return "", err
		}
		blocks = append(blocks, header...)

		if entry.EntryType == types.TarEntryFile && len(entry.Content) > 0 {
			blocks = append(blocks, entry.Content...)
			padding := blockSize - (len(entry.Content) % blockSize)
			if padding < blockSize {
				blocks = append(blocks, make([]byte, padding)...)
			}
		}
	}

	blocks = append(blocks, make([]byte, blockSize*2)...)

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return "", &Error{Message: "create output directory", Cause: err}
	}
	if err := os.WriteFile(outputPath, blocks, 0o644); err != nil {
		return "", &Error{Message: "write tar archive", Cause: err}
	}

	return hexDigest(blocks), nil
}

// ReadTar reads a tar archive from disk.
func ReadTar(inputPath string) ([]types.TarEntry, string, error) {
	archive, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, "", &Error{Message: "read tar archive", Cause: err}
	}

	checksum := hexDigest(archive)
	var entries []types.TarEntry
	offset := 0

	for offset+headerSize <= len(archive) {
		headerBlock := archive[offset : offset+headerSize]

		if isZeroBlock(headerBlock) {
			offset += blockSize
			if offset+blockSize <= len(archive) && isZeroBlock(archive[offset:offset+blockSize]) {
				break
			}
			continue
		}

		header, err := decodeHeader(headerBlock)
		if err != nil {
			return nil, "", err
		}
		offset += headerSize

		if header.typeflag != '0' && header.typeflag != '5' {
			return nil, "", &Error{Message: fmt.Sprintf(
				`Unsupported tar entry type "%c" for "%s". Only regular files (typeflag '0') and directories (typeflag '5') are accepted.`,
				header.typeflag, header.name,
			)}
		}

		if header.linkname != "" {
			return nil, "", &Error{Message: fmt.Sprintf(
				`Tar entry "%s" has a non-empty linkname field ("%s"). Link entries are not supported for security reasons.`,
				header.name, header.linkname,
			)}
		}

		size := header.size
		if header.typeflag == '5' {
			size = 0
		}
		if size > maxEntryBytes {
			return nil, "", &Error{Message: fmt.Sprintf("Tar entry too large: %s (%d bytes)", header.name, size)}
		}

		var content []byte
		if size > 0 {
			end := offset + int(size)
			if end > len(archive) {
				return nil, "", &Error{Message: fmt.Sprintf("Truncated tar entry: %s", header.name)}
			}
			content = append([]byte(nil), archive[offset:end]...)
		}

		entryType := types.TarEntryFile
		if header.typeflag == '5' {
			entryType = types.TarEntryDirectory
		}

		entries = append(entries, types.TarEntry{
			Path:      header.name,
			Content:   content,
			Mode:      header.mode,
			Mtime:     header.mtime * 1000,
			EntryType: entryType,
		})

		dataBlocks := int((size + blockSize - 1) / blockSize)
		offset += dataBlocks * blockSize
	}

	return entries, checksum, nil
}

// ValidateTarPath ensures a tar entry path cannot escape the extraction root.
func ValidateTarPath(entryPath, root string) (string, error) {
	if strings.Contains(entryPath, "..") {
		return "", &Error{Message: fmt.Sprintf(`Path traversal detected: "%s" contains ".."`, entryPath)}
	}
	if strings.ContainsRune(entryPath, '\x00') {
		return "", &Error{Message: fmt.Sprintf(`Path traversal detected: "%s" contains null byte`, entryPath)}
	}
	if filepath.IsAbs(entryPath) {
		return "", &Error{Message: fmt.Sprintf(`Path traversal detected: "%s" is absolute`, entryPath)}
	}

	resolved := filepath.Join(root, entryPath)
	normalizedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		normalizedRoot = root
	}
	normalizedResolved, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		normalizedResolved = resolved
	}

	if normalizedResolved != normalizedRoot &&
		!strings.HasPrefix(normalizedResolved, normalizedRoot+string(os.PathSeparator)) {
		return "", &Error{Message: fmt.Sprintf(`Path traversal detected: "%s" resolves outside root`, entryPath)}
	}

	return entryPath, nil
}

type tarHeader struct {
	name     string
	mode     uint32
	size     uint64
	mtime    uint64
	typeflag byte
	linkname string
}

func encodeHeader(entry types.TarEntry) ([]byte, error) {
	buf := make([]byte, headerSize)
	name := entry.Path

	writeField(buf, 0, 100, name)
	writeField(buf, 100, 8, fmt.Sprintf("%07o", entry.Mode&0o7777))
	writeField(buf, 108, 8, "0000000")
	writeField(buf, 116, 8, "0000000")

	sizeField := "00000000000"
	if entry.EntryType == types.TarEntryFile {
		sizeField = fmt.Sprintf("%011o", len(entry.Content))
	}
	writeField(buf, 124, 12, sizeField)
	writeField(buf, 136, 12, fmt.Sprintf("%011o", entry.Mtime/1000))
	writeField(buf, 148, 8, strings.Repeat(" ", 8))

	typeflag := byte('0')
	if entry.EntryType == types.TarEntryDirectory {
		typeflag = '5'
	}
	buf[156] = typeflag
	writeField(buf, 257, 6, magic)
	writeField(buf, 263, 2, "00")

	var checksum uint32
	for _, b := range buf {
		checksum += uint32(b)
	}
	writeField(buf, 148, 7, fmt.Sprintf("%07o", checksum))
	buf[155] = ' '

	return buf, nil
}

func decodeHeader(buf []byte) (*tarHeader, error) {
	name := readField(buf, 0, 100)
	mode := uint32(parseOctalField(readField(buf, 100, 8)))
	size := parseOctalField(readField(buf, 124, 12))
	mtime := parseOctalField(readField(buf, 136, 12))
	checksumField := readField(buf, 148, 8)
	typeflag := byte('0')
	if len(buf) > 156 && buf[156] != 0 {
		typeflag = buf[156]
	}
	linkname := readField(buf, 157, 100)

	savedChecksum := parseOctalFieldU32(strings.TrimSpace(checksumField))
	recomputed := append([]byte(nil), buf...)
	writeField(recomputed, 148, 8, strings.Repeat(" ", 8))
	var actual uint32
	for _, b := range recomputed {
		actual += uint32(b)
	}
	if savedChecksum != 0 && savedChecksum != actual {
		return nil, &Error{Message: fmt.Sprintf(
			`Checksum mismatch for tar entry "%s": expected %s, got %s`,
			name, formatOctal(actual), formatOctal(savedChecksum),
		)}
	}

	return &tarHeader{
		name:     name,
		mode:     mode,
		size:     size,
		mtime:    mtime,
		typeflag: typeflag,
		linkname: linkname,
	}, nil
}

func writeField(buf []byte, offset, length int, value string) {
	end := offset + length
	if end > len(buf) {
		return
	}
	for i := offset; i < end; i++ {
		buf[i] = 0
	}
	copy(buf[offset:end], []byte(value))
}

func readField(buf []byte, offset, length int) string {
	end := offset + length
	if end > len(buf) {
		end = len(buf)
	}
	nullPos := offset
	for nullPos < end && buf[nullPos] != 0 {
		nullPos++
	}
	return string(buf[offset:nullPos])
}

func parseOctalField(value string) uint64 {
	trimmed := strings.Trim(strings.Trim(value, "\x00"), " ")
	if trimmed == "" {
		return 0
	}
	parsed, err := strconv.ParseUint(trimmed, 8, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func parseOctalFieldU32(value string) uint32 {
	return uint32(parseOctalField(value))
}

func formatOctal(value uint32) string {
	return strconv.FormatUint(uint64(value), 8)
}

func isZeroBlock(buf []byte) bool {
	for _, b := range buf {
		if b != 0 {
			return false
		}
	}
	return true
}

func hexDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}