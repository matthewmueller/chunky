package chunker

import (
	"bytes"

	"github.com/restic/chunker"
)

var pol = chunker.Pol(0x3DA3358B4DC173)

func New(data []byte) Chunker {
	return &defaultChunker{
		chunker.New(bytes.NewReader(data), pol),
	}
}

type Chunk = chunker.Chunk

type Chunker interface {
	Chunk() (Chunk, error)
}

type defaultChunker struct {
	ch *chunker.Chunker
}

func (c *defaultChunker) Chunk() (Chunk, error) {
	scratch := make([]byte, chunker.MaxSize)
	chunk, err := c.ch.Next(scratch)
	if err != nil {
		return Chunk{}, err
	}
	return (Chunk)(chunk), nil
}
