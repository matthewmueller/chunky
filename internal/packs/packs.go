package packs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/matthewmueller/chunky/internal/chunker"
	"github.com/matthewmueller/chunky/internal/repos"
	"github.com/matthewmueller/chunky/internal/sha256"
)

func New() *Pack {
	return &Pack{
		headers: []*header{},
		buffer:  new(bytes.Buffer),
	}
}

type Pack struct {
	headers []*header
	buffer  *bytes.Buffer
}

type header struct {
	Kind   kind   `json:"kind,omitempty"`
	ID     string `json:"id,omitempty"`
	Offset uint   `json:"offset,omitempty"`
	Length uint   `json:"length,omitempty"`
}

type kind uint8

const (
	kindBlob kind = iota + 1
	kindFile
)

type File struct {
	Path    string
	Mode    fs.FileMode
	Size    uint64
	ModTime time.Time
	Data    []byte
}

type fileState struct {
	Mode    fs.FileMode `json:"mode,omitempty"`
	Size    uint64      `json:"size,omitempty"`
	Chunks  []string    `json:"chunks,omitempty"`
	ModTime int64       `json:"modtime,omitempty"`
}

func (p *Pack) Add(file *File) error {
	// Create the file state
	state := &fileState{
		Mode:    file.Mode,
		Size:    uint64(len(file.Data)),
		ModTime: file.ModTime.Unix(),
	}

	// Chunk the data
	chunker := chunker.New(file.Data)
	for {
		chunk, err := chunker.Chunk()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
		hash := sha256.Hash(chunk.Data)
		if err := p.addBlob(hash, chunk.Data); err != nil {
			return fmt.Errorf("packs: unable to add blob: %w", err)
		}
		state.Chunks = append(state.Chunks, hash)
	}

	// Write the file state
	fileData, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("packs: unable to marshal file state: %w", err)
	}

	// Add the file
	if err := p.addFile(file.Path, fileData); err != nil {
		return fmt.Errorf("packs: unable to add file: %w", err)
	}

	return nil
}

func (p *Pack) addBlob(blobHash string, data []byte) error {
	// Create a header
	header := &header{kindBlob, blobHash, uint(p.buffer.Len()), 0}
	p.headers = append(p.headers, header)

	// Write the data
	n, err := p.buffer.Write(data)
	if err != nil {
		return err
	}

	// Update the length
	header.Length = uint(n)

	return nil
}

func (p *Pack) addFile(fpath string, data []byte) error {
	// Create a header
	header := &header{kindFile, fpath, uint(p.buffer.Len()), 0}
	p.headers = append(p.headers, header)

	// Write the data
	n, err := p.buffer.Write(data)
	if err != nil {
		return err
	}

	// Update the length
	header.Length = uint(n)

	return nil
}

func (p *Pack) Pack() ([]byte, error) {
	out := new(bytes.Buffer)

	// If the buffer is empty, return nil
	if p.buffer.Len() == 0 {
		return nil, nil
	}

	// Write the headers
	headers, err := json.Marshal(p.headers)
	if err != nil {
		return nil, err
	}

	enc, err := zstd.NewWriter(out)
	if err != nil {
		return nil, err
	}

	// Write the headers and the data
	enc.Write(headers)
	enc.Write([]byte{'\n'})
	enc.Write(p.buffer.Bytes())

	if err := enc.Close(); err != nil {
		return nil, err
	}

	return out.Bytes(), nil
}

func Unpack(data []byte) (*Pack, error) {
	dec, err := zstd.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer dec.Close()
	out := new(bytes.Buffer)
	if _, err := io.Copy(out, dec); err != nil {
		return nil, err
	}

	line, err := out.ReadBytes('\n')
	if err != nil {
		return nil, err
	}

	// Decode the first set of headers up to the first newline
	var headers []*header
	if err := json.Unmarshal(line, &headers); err != nil {
		return nil, err
	}

	return &Pack{
		headers: headers,
		buffer:  bytes.NewBufferString(out.String()),
	}, nil
}

func (p *Pack) findHeader(id string) (*header, error) {
	for _, header := range p.headers {
		if header.ID == id {
			return header, nil
		}
	}
	return nil, fmt.Errorf("packs: header not found")
}

func (p *Pack) Read(path string) (*File, error) {
	header, err := p.findHeader(path)
	if err != nil {
		return nil, err
	} else if header.Kind != kindFile {
		return nil, fmt.Errorf("packs: %s not a file", path)
	}

	packData := p.buffer.Bytes()

	var entry fileState
	entryData := packData[header.Offset : header.Offset+header.Length]
	if err := json.Unmarshal(entryData, &entry); err != nil {
		return nil, err
	}

	file := &File{
		Path:    path,
		Mode:    entry.Mode,
		ModTime: time.Unix(entry.ModTime, 0),
	}

	for _, chunk := range entry.Chunks {
		header, err := p.findHeader(chunk)
		if err != nil {
			return nil, err
		} else if header.Kind != kindBlob {
			return nil, fmt.Errorf("packs: %s not a blob", chunk)
		}
		data := packData[header.Offset : header.Offset+header.Length]
		file.Data = append(file.Data, data...)
	}

	return file, nil
}

// Read a pack from a repository
func Read(ctx context.Context, repo repos.Repo, packId string) (*Pack, error) {
	packFile, err := repos.Download(ctx, repo, path.Join("packs", packId))
	if err != nil {
		return nil, fmt.Errorf("commits: unable to download pack %q: %w", packId, err)
	}
	return Unpack(packFile.Data)
}
