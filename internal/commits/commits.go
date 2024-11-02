package commits

import (
	"io/fs"
	"time"
)

func New(message string) *Commit {
	return &Commit{
		Message:   message,
		CreatedAt: time.Now(),
		Files:     map[string]*File{},
	}
}

type Commit struct {
	Message   string           `json:"message,omitempty"`
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

// func (c *Commit) Encode() (*virt.File, error) {
// 	data, err := json.MarshalIndent(c, "", "  ")
// 	if err != nil {
// 		return nil, err
// 	}
// 	hash := sha256.Sum256(data)
// 	return &virt.File{
// 		Path: fmt.Sprintf("commits/%02x", hash),
// 		Mode: 0644,
// 		Data: data,
// 	}, nil
// }
