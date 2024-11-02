package commits

import (
	"io/fs"
	"time"
)

func New() *Commit {
	return &Commit{
		CreatedAt: time.Now(),
		Files:     map[string]*File{},
	}
}

type Commit struct {
	CreatedAt time.Time        `json:"created_at,omitempty"`
	Files     map[string]*File `json:"files,omitempty"`
}

func (c *Commit) File(path string, info fs.FileInfo) *File {
	file, ok := c.Files[path]
	if !ok {
		file = &File{Path: path}
		c.Files[path] = file
	}
	file.Mode = info.Mode()
	file.ModTime = info.ModTime().UTC()
	file.Size = info.Size()
	return file
}

func (c *Commit) Size() (size uint64) {
	for _, file := range c.Files {
		size += uint64(file.Size)
	}
	return size
}

type File struct {
	Path    string      `json:"path,omitempty"`
	Mode    fs.FileMode `json:"mode,omitempty"`
	ModTime time.Time   `json:"modtime,omitempty"`
	Size    int64       `json:"size,omitempty"`
	Objects []string    `json:"objects,omitempty"`
}

func (f *File) Add(object string) {
	f.Objects = append(f.Objects, object)
}
