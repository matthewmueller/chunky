package packs

import (
	"context"
	"path"

	"github.com/matthewmueller/chunky/repos"
)

// func (*Pack) Read(path string) (*virt.File, error) {
// 	return nil, fmt.Errorf("not implemented")
// }

func Read(ctx context.Context, repo repos.Repo, packId string) (*Pack, error) {
	packFile, err := repos.Download(ctx, repo, path.Join("packs", packId))
	if err != nil {
		return nil, err
	}
	return Unpack(packFile.Data)
}
