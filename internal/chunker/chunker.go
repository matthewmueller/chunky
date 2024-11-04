package chunker

import (
	"bytes"

	"github.com/restic/chunker"
)

var pol = chunker.Pol(0x3DA3358B4DC173)

func New(data []byte) Chunker {
	return &defaultChunker{
		chunker.New(bytes.NewReader(data), pol),
		make([]byte, chunker.MaxSize),
	}
}

type Chunker interface {
	Chunk() (Chunk, error)
}

type defaultChunker struct {
	ch      *chunker.Chunker
	scratch []byte
}

func (c *defaultChunker) Chunk() (Chunk, error) {
	chunk, err := c.ch.Next(c.scratch)
	if err != nil {
		return Chunk{}, err
	}
	return (Chunk)(chunk), nil
}

type Chunk chunker.Chunk
