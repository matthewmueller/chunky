package sha256

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"io/fs"
)

func Hash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func String(hash [32]byte) string {
	return hex.EncodeToString(hash[:])
}

func HashFile(fsys fs.FS, path string, chunkSize int64) (string, error) {
	file, err := fsys.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	buffer := make([]byte, chunkSize)
	if _, err := io.CopyBuffer(hash, file, buffer); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func New() hash.Hash {
	return sha256.New()
}
