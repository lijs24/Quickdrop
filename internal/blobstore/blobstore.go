package blobstore

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"
)

type StoredBlob struct {
	OriginalName string
	SafeName     string
	SHA256       string
	SizeBytes    int64
	Path         string
}

func StoreReader(dataDir, originalName string, r io.Reader) (*StoredBlob, error) {
	tmpDir := filepath.Join(dataDir, "tmp")
	blobDir := filepath.Join(dataDir, "blobs")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return nil, fmt.Errorf("create tmp directory: %w", err)
	}
	if err := os.MkdirAll(blobDir, 0o755); err != nil {
		return nil, fmt.Errorf("create blobs directory: %w", err)
	}

	tmp, err := os.CreateTemp(tmpDir, "upload-*")
	if err != nil {
		return nil, fmt.Errorf("create upload temp file: %w", err)
	}
	tmpPath := tmp.Name()
	removeTmp := true
	defer func() {
		if removeTmp {
			_ = os.Remove(tmpPath)
		}
	}()

	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, hash), r)
	if closeErr := tmp.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil {
		return nil, fmt.Errorf("write upload temp file: %w", err)
	}

	sum := hex.EncodeToString(hash.Sum(nil))
	if err := VerifyFileSHA256(tmpPath, sum); err != nil {
		return nil, fmt.Errorf("verify upload temp file: %w", err)
	}

	finalDir := filepath.Join(blobDir, sum[:2])
	if err := os.MkdirAll(finalDir, 0o755); err != nil {
		return nil, fmt.Errorf("create blob prefix directory: %w", err)
	}
	finalPath := filepath.Join(finalDir, sum)
	if _, err := os.Stat(finalPath); err == nil {
		return &StoredBlob{
			OriginalName: originalName,
			SafeName:     SafeName(originalName),
			SHA256:       sum,
			SizeBytes:    size,
			Path:         finalPath,
		}, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat blob path: %w", err)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return nil, fmt.Errorf("promote blob with atomic rename: %w", err)
	}
	removeTmp = false
	return &StoredBlob{
		OriginalName: originalName,
		SafeName:     SafeName(originalName),
		SHA256:       sum,
		SizeBytes:    size,
		Path:         finalPath,
	}, nil
}

func PathForSHA(dataDir, sha string) string {
	if len(sha) < 2 {
		return filepath.Join(dataDir, "blobs", sha)
	}
	return filepath.Join(dataDir, "blobs", sha[:2], sha)
}

func VerifyFileSHA256(path, expected string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open file for sha256: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("hash file: %w", err)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("sha256 mismatch: expected %s, got %s", expected, actual)
	}
	return nil
}

func SafeName(name string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(name), "\\", "/")
	base := path.Base(normalized)
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = "file"
	}
	var b strings.Builder
	for _, r := range base {
		switch {
		case r == '<' || r == '>' || r == ':' || r == '"' || r == '/' || r == '\\' || r == '|' || r == '?' || r == '*':
			b.WriteByte('_')
		case unicode.IsControl(r):
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	safe := strings.Trim(b.String(), " .")
	if safe == "" || safe == "." || safe == ".." {
		return "file"
	}
	return safe
}
