package packs

import (
	"bytes"
	"context"
	"encoding/gob"
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
		records: []*record{},
		// files: map[string]*record{},
		// blobs: map[string]*blobState{},
	}
}

type Pack struct {
	records []*record
}

// type header struct {
// 	Kind   kind   `json:"kind,omitempty"`
// 	ID     string `json:"id,omitempty"`
// 	Offset uint   `json:"offset,omitempty"`
// 	Length uint   `json:"length,omitempty"`
// }

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

type record struct {
	Kind    kind        `json:"kind,omitempty"`
	Hash    string      `json:"hash,omitempty"`
	Path    string      `json:"path,omitempty"`
	Mode    fs.FileMode `json:"mode,omitempty"`
	Size    uint64      `json:"size,omitempty"`
	Chunks  []string    `json:"chunks,omitempty"`
	ModTime int64       `json:"modtime,omitempty"`
	Data    []byte      `json:"data,omitempty"`
}

func (p *Pack) Add(file *File) error {
	// Create the file state
	state := &record{
		Kind:    kindFile,
		Hash:    sha256.Hash(file.Data),
		Path:    file.Path,
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
	p.records = append(p.records, state)

	// fileData, err := json.Marshal(state)
	// if err != nil {
	// 	return fmt.Errorf("packs: unable to marshal file state: %w", err)
	// }

	// // Add the file
	// if err := p.addFile(file.Path, fileData); err != nil {
	// 	return fmt.Errorf("packs: unable to add file: %w", err)
	// }

	return nil
}

func (p *Pack) addBlob(blobHash string, data []byte) error {
	p.records = append(p.records, &record{
		Hash: blobHash,
		Kind: kindBlob,
		Data: data,
	})
	// // Create a header
	// header := &header{kindBlob, blobHash, uint(p.buffer.Len()), 0}
	// p.headers = append(p.headers, header)

	// // Write the data
	// n, err := p.buffer.Write(data)
	// if err != nil {
	// 	return err
	// }

	// // Update the length
	// header.Length = uint(n)

	return nil
}

// func (p *Pack) addFile(fpath string, data []byte) error {
// 	// Create a header
// 	header := &header{kindFile, fpath, uint(p.buffer.Len()), 0}
// 	p.headers = append(p.headers, header)

// 	// Write the data
// 	n, err := p.buffer.Write(data)
// 	if err != nil {
// 		return err
// 	}

// 	// Update the length
// 	header.Length = uint(n)

// 	return nil
// }

func (p *Pack) Pack() ([]byte, error) {
	out := new(bytes.Buffer)
	writer, err := zstd.NewWriter(out)
	if err != nil {
		return nil, err
	}
	enc := gob.NewEncoder(writer)
	for _, record := range p.records {
		if err := enc.Encode(record); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func Unpack(data []byte) (*Pack, error) {
	reader, err := zstd.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	pack := New()
	dec := gob.NewDecoder(reader)
	for {
		var record record
		if err := dec.Decode(&record); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		pack.records = append(pack.records, &record)
	}
	// for dec.More() {
	// 	var record record
	// 	if err := dec.Decode(&record); err != nil {
	// 		return nil, err
	// 	}
	// 	pack.records = append(pack.records, &record)
	// }

	// defer dec.Close()
	// out := new(bytes.Buffer)
	// if _, err := io.Copy(out, dec); err != nil {
	// 	return nil, err
	// }

	// line, err := out.ReadBytes('\n')
	// if err != nil {
	// 	return nil, err
	// }

	// // Decode the first set of headers up to the first newline
	// var headers []*header
	// if err := json.Unmarshal(line, &headers); err != nil {
	// 	return nil, err
	// }
	return pack, nil
	// return &Pack{
	// 	records: pack.records,
	// 	headers: headers,
	// 	buffer:  bytes.NewBufferString(out.String()),
	// }, nil
}

func (p *Pack) findRecordByHash(hash string) (*record, error) {
	for _, record := range p.records {
		if record.Hash == hash {
			return record, nil
		}
	}
	return nil, fmt.Errorf("packs: record %q not found", hash)
}

func (p *Pack) findRecordByPath(path string) (*record, error) {
	for _, record := range p.records {
		if record.Path == path {
			return record, nil
		}
	}
	return nil, fmt.Errorf("packs: record %q not found", path)
}

func (p *Pack) Read(path string) (*File, error) {
	record, err := p.findRecordByPath(path)
	if err != nil {
		return nil, err
	} else if record.Kind != kindFile {
		return nil, fmt.Errorf("packs: %s not a file", path)
	}

	// packData := p.buffer.Bytes()

	// var entry record
	// entryData := packData[header.Offset : header.Offset+header.Length]
	// if err := json.Unmarshal(entryData, &entry); err != nil {
	// 	return nil, err
	// }

	file := &File{
		Path:    path,
		Mode:    record.Mode,
		ModTime: time.Unix(record.ModTime, 0),
	}

	for _, chunk := range record.Chunks {
		record, err := p.findRecordByHash(chunk)
		if err != nil {
			return nil, err
		} else if record.Kind != kindBlob {
			return nil, fmt.Errorf("packs: %s not a blob", chunk)
		}
		// data := packData[header.Offset : header.Offset+header.Length]
		file.Data = append(file.Data, record.Data...)
	}

	return file, nil
}

func (p *Pack) Encode(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(p.records)
}

// Read a pack from a repository
func Read(ctx context.Context, repo repos.Repo, packId string) (*Pack, error) {
	packFile, err := repos.Download(ctx, repo, path.Join("packs", packId))
	if err != nil {
		return nil, fmt.Errorf("commits: unable to download pack %q: %w", packId, err)
	}
	return Unpack(packFile.Data)
}
