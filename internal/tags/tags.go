package tags

import (
	"path/filepath"

	"github.com/matthewmueller/virt"
)

func Latest(ref string) *virt.File {
	return &virt.File{
		Path: filepath.Join("tags", "latest"),
		Mode: 0644,
		Data: []byte(ref),
	}
}

func New(name, ref string) *virt.File {
	return &virt.File{
		Path: filepath.Join("tags", name),
		Mode: 0644,
		Data: []byte(ref),
	}
}
