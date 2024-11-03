package chunker

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"

	"github.com/restic/chunker"
)

var pol = chunker.Pol(0x3DA3358B4DC173)

func New(data []byte) *Chunker {
	return &Chunker{
		chunker.New(bytes.NewReader(data), pol),
		make([]byte, chunker.MaxSize),
	}
}

type Chunker struct {
	ch      *chunker.Chunker
	scratch []byte
}

func (c *Chunker) Chunk() (Chunk, error) {
	chunk, err := c.ch.Next(c.scratch)
	if err != nil {
		return Chunk{}, err
	}
	return (Chunk)(chunk), nil
}

type Chunk chunker.Chunk

func (c *Chunk) Hash() string {
	hash := sha256.Sum256(c.Data)
	return hex.EncodeToString(hash[:])
}
