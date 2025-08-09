package sha256

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"io/fs"
	"strconv"

	"github.com/matthewmueller/chunky/internal/packs"
)

func Hash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func String(hash [32]byte) string {
	return hex.EncodeToString(hash[:])
}

func HashFile(fsys fs.FS, path string, chunkSize int) (string, error) {
	file, err := fsys.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return "", err
	}

	hash := sha256.New()
	hash.Write([]byte(stamp(stat.Mode(), stat.Size())))

	buffer := make([]byte, chunkSize)
	if _, err := io.CopyBuffer(hash, file, buffer); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// Stamp helps quickly determine if a file has changed.
func stamp(mode fs.FileMode, size int64) string {
	return strconv.Itoa(int(size)) + ":" + strconv.Itoa(int(mode)) //+ ":" + strconv.Itoa(int(modTime))
}

func New(fc *packs.Chunk) *Hasher {
	h := sha256.New()
	h.Write([]byte(stamp(fc.Mode, fc.Size)))
	if fc.Data != nil {
		h.Write(fc.Data)
	}
	return &Hasher{fc, h}
}

type Hasher struct {
	fc *packs.Chunk
	h  hash.Hash
}

func (h *Hasher) Write(bc *packs.Chunk) (n int, err error) {
	return h.h.Write(bc.Data)
}

func (h *Hasher) String() string {
	return hex.EncodeToString(h.h.Sum(nil))
}
