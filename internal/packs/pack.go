package packs

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"strconv"

	"github.com/klauspost/compress/zstd"
)

func New() *Pack {
	return &Pack{nil, 0}
}

// Chunk can either be file metadata or a blob. If the file is small enough to
// fit in one chunk, it can be a file with data.
type Chunk struct {
	Path    string      `json:"path,omitempty"`
	Mode    fs.FileMode `json:"mode,omitempty"`
	Size    int64       `json:"size,omitempty"`
	Hash    string      `json:"hash,omitempty"`
	ModTime int64       `json:"modtime,omitempty"`
	// Data can be set if the file is small enough to fit in one chunk. Otherwise
	// it will be nil and the file will be chunked with links to the blobs
	Data []byte `json:"data,omitempty"`
	Refs []*Ref `json:"refs,omitempty"`
}

// Ref is a reference to another blob chunk
type Ref struct {
	Pack string
	Hash string
}

func (c *Chunk) Key() string {
	if c.Path != "" {
		return c.Path
	}
	return c.Hash
}

func (c *Chunk) Kind() string {
	if c.Path == "" {
		return "blob"
	}
	return "file"
}

func (c *Chunk) Link(pack string, chunk *Chunk) {
	c.Refs = append(c.Refs, &Ref{
		Pack: pack,
		Hash: chunk.Hash,
	})
}

func (c *Chunk) Links() []*Ref {
	return c.Refs
}

func (c *Chunk) Length() int64 {
	n := int64(len(c.Path))
	n += int64(len(c.Hash))
	n += 8 // Size (int64)
	n += 8 // ModTime (int64)
	n += 4 // Mode (uint32)
	n += int64(len(c.Data))
	for _, blob := range c.Refs {
		n += int64(len(blob.Pack))
		n += int64(len(blob.Hash))
	}
	return n
}

func (c *Chunk) Print(w io.StringWriter) {
	if c.Path == "" {
		w.WriteString(strconv.Itoa(len(c.Data)))
		w.WriteString(" ")
		w.WriteString(c.Hash)
		w.WriteString("\n")
		return
	}
	w.WriteString(strconv.Itoa(int(c.Size)))
	w.WriteString(" ")
	w.WriteString(c.Path)
	w.WriteString("\n")
}

func (c *Chunk) String() string {
	var buf bytes.Buffer
	c.Print(&buf)
	return buf.String()
}

type Pack struct {
	chunks []*Chunk
	length int64
}

func (p *Pack) Add(chunks ...*Chunk) {
	for _, chunk := range chunks {
		p.chunks = append(p.chunks, chunk)
		p.length += chunk.Length()
	}
}

func (p *Pack) Pack() ([]byte, error) {
	data := new(bytes.Buffer)
	writer, err := zstd.NewWriter(data)
	if err != nil {
		return nil, fmt.Errorf("packs: unable to create zstd writer %w", err)
	}
	enc := gob.NewEncoder(writer)
	for _, chunk := range p.chunks {
		if err := enc.Encode(chunk); err != nil {
			return nil, fmt.Errorf("packs: unable to encode chunk: %w", err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("packs: unable to close zstd writer %w", err)
	}
	return data.Bytes(), nil
}

func (p *Pack) Chunk(key string) *Chunk {
	for _, chunk := range p.chunks {
		if chunk.Key() == key {
			return chunk
		}
	}
	return nil
}

func (p *Pack) Chunks() []*Chunk {
	return p.chunks
}

func (p *Pack) Print(w io.StringWriter) {
	for _, chunk := range p.chunks {
		chunk.Print(w)
	}
}

func (p *Pack) String() string {
	var buf bytes.Buffer
	p.Print(&buf)
	return buf.String()
}

func (p *Pack) Encode(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(p.chunks)
}

// Length returns the number of bytes in the pack
func (p *Pack) Length() int64 {
	return p.length
}

// Unpack reads a pack from a byte slice
func Unpack(data []byte) (*Pack, error) {
	reader, err := zstd.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	pack := New()
	dec := gob.NewDecoder(reader)
	for {
		var chunk Chunk
		if err := dec.Decode(&chunk); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("packs: unable to decode chunk %w", err)
		}
		pack.Add(&chunk)
	}
	return pack, nil
}
