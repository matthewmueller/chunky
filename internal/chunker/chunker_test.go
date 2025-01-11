package chunker_test

import (
	"bytes"
	"io"
	"testing"

	"math/rand"

	"github.com/matryer/is"
	"github.com/matthewmueller/chunky/internal/chunker"
)

func TestChunker(t *testing.T) {
	is := is.New(t)
	// generate 32MiB of deterministic pseudo-random data
	rng := rand.New(rand.NewSource(23))
	data := make([]byte, 32*1024*1024)

	_, err := rng.Read(data)
	is.NoErr(err)

	// create a chunker
	c := chunker.New(bytes.NewReader(data), chunker.MinSize, chunker.MaxSize)

	chunks := []chunker.Chunk{}
	for {
		chunk, err := c.Chunk()
		if err != nil {
			if err == io.EOF {
				break
			}
			is.NoErr(err)
		}
		chunks = append(chunks, chunk)
	}

	is.True(len(chunks) > 0)

	// check that the data is the same
	offset := uint(0)
	for _, chunk := range chunks {
		actual := chunk.Data
		expect := data[offset : offset+chunk.Length]
		is.Equal(len(actual), len(expect))
		if !bytes.Equal(actual, expect) {
			t.Fatalf("chunker mismatch")
		}
		offset += chunk.Length
	}
}
