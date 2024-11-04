package sha256

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
)

func Hash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func New() hash.Hash {
	return sha256.New()
}
