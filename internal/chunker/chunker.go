package chunker

import (
	"io"

	"github.com/restic/chunker"
)

var pol = chunker.Pol(0x3DA3358B4DC173)
var MinSize uint = chunker.MinSize
var MaxSize uint = chunker.MaxSize

func New(r io.Reader, minSize, maxSize uint) Chunker {
	return &defaultChunker{
		chunker.New(r, pol, chunker.WithBoundaries(minSize, maxSize)),
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
