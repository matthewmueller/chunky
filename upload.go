package chunky

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os/user"
	"path"
	"time"

	"github.com/matthewmueller/chunky/internal/caches"
	"github.com/matthewmueller/chunky/internal/chunkyignore"
	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/chunky/internal/packs"
	"github.com/matthewmueller/chunky/internal/sha256"
	"github.com/matthewmueller/chunky/repos"
	"github.com/matthewmueller/virt"
	"golang.org/x/sync/errgroup"
)

type Upload struct {
	From     fs.FS
	To       repos.Repo
	Cache    virt.FS
	User     string
	Tags     []string
	Ignore   func(path string) bool
	ReadFile func(path string) ([]byte, error)
}

func (in *Upload) validate() (err error) {
	// Required fields
	if in.From == nil {
		err = errors.Join(err, errors.New("missing from filesystem"))
	}
	if in.To == nil {
		err = errors.Join(err, errors.New("missing to repository"))
	}
	if in.Cache == nil {
		err = errors.Join(err, errors.New("missing cache"))
	}

	// Default to the current user
	if in.User == "" {
		user, err := user.Current()
		if err != nil {
			return errors.Join(err, fmt.Errorf("missing user and getting current user failed with: %w", err))
		}
		in.User = user.Username
	}

	// Validate the tags
	for _, tag := range in.Tags {
		if tag == "latest" {
			err = errors.Join(err, errors.New("tag cannot be 'latest'"))
		} else if tag == "" {
			err = errors.Join(err, errors.New("tag cannot be empty"))
		}
	}

	// Default to the .chunkyignore file
	if in.Ignore == nil {
		in.Ignore = chunkyignore.FromFS(in.From)
	}

	// Default to reading files from the 'from' filesystem
	if in.ReadFile == nil {
		in.ReadFile = func(path string) ([]byte, error) {
			return fs.ReadFile(in.From, path)
		}
	}

	return err
}

// Upload a directory to a repository
func (c *Client) Upload(ctx context.Context, in *Upload) error {
	if err := in.validate(); err != nil {
		return err
	}

	// Download the latest commits from the cache
	cache, err := caches.Download(ctx, in.To, in.Cache)
	if err != nil {
		return err
	}

	ignore := in.Ignore
	createdAt := time.Now().UTC()
	commit := commits.New(in.User, createdAt)
	commitId := commit.ID()
	pack := packs.New()

	// Walk over the files, chunk them and add them to the file system we're going
	// to upload. We'll also add each file to the commit object.
	if err := fs.WalkDir(in.From, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		} else if d.IsDir() || ignore(path) {
			return nil
		}

		file, err := in.From.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		data, err := in.ReadFile(path)
		if err != nil {
			return err
		}
		fileHash := sha256.Hash(data)

		info, err := file.Stat()
		if err != nil {
			return err
		}

		// Check if the file is already in the pack
		// TODO: right now this will duplicate content when the file path in the
		// pack is different from the file path in the commit. We should add a
		// way to alias files in the pack to other packs.
		if commitFile, ok := cache.Get(fileHash); ok && commitFile.Path == path {
			commit.Add(&commits.File{
				Path:   path,
				Size:   uint64(len(data)),
				Id:     fileHash,
				PackId: commitFile.PackId,
			})
			return nil
		}

		entry := &packs.File{
			Path:    path,
			Mode:    info.Mode(),
			ModTime: info.ModTime(),
			Data:    data,
		}

		// Add the entry to the pack
		if err := pack.Add(entry); err != nil {
			return err
		}

		// Add the file to the commit
		commit.Add(&commits.File{
			Path:   path,
			Id:     fileHash,
			PackId: commitId,
			Size:   uint64(info.Size()),
		})

		return nil
	}); err != nil {
		return err
	}

	// pack file + commit file + latest ref + tags
	capacity := 1 + 1 + 1 + len(in.Tags)
	fromCh := make(chan *virt.File, capacity)

	eg := new(errgroup.Group)
	eg.Go(func() error {
		return in.To.Upload(ctx, fromCh)
	})

	// Add the pack to the tree
	packData, err := pack.Pack()
	if err != nil {
		close(fromCh)
		return err
	}
	if len(packData) > 0 {
		fromCh <- &virt.File{
			Path:    path.Join("packs", commitId),
			Data:    packData,
			Mode:    0644,
			ModTime: createdAt,
		}
	}

	// Add the commit to the tree
	commitData, err := commit.Pack()
	if err != nil {
		close(fromCh)
		return err
	}
	fromCh <- &virt.File{
		Path:    path.Join("commits", commitId),
		Data:    commitData,
		Mode:    0644,
		ModTime: createdAt,
	}

	// Add the commit to the cache
	if err := cache.Set(commitId, commit); err != nil {
		close(fromCh)
		return err
	}

	// Add the latest ref
	fromCh <- &virt.File{
		Path: path.Join("tags", "latest"),
		Data: []byte(commitId),
		Mode: 0644,
	}

	// Tag the revision
	for _, tag := range in.Tags {
		fromCh <- &virt.File{
			Path: fmt.Sprintf("tags/%s", tag),
			Data: []byte(commitId),
			Mode: 0644,
		}
	}

	close(fromCh)

	return eg.Wait()
}

// // walkDir walks the directory tree concurrently using fs.FS and matches the signature of fs.WalkDir.
// func walkDir(fsys fs.FS, root string, fn fs.WalkDirFunc) error {
// 	var wg sync.WaitGroup
// 	errChan := make(chan error, 1)
// 	done := make(chan struct{})

// 	// Stop all workers on the first error
// 	stop := func(err error) {
// 		select {
// 		case errChan <- err:
// 		default:
// 		}
// 		close(done)
// 	}

// 	var walk func(string)
// 	walk = func(path string) {
// 		defer wg.Done()

// 		select {
// 		case <-done:
// 			return // Stop processing on error
// 		default:
// 		}

// 		entries, err := fs.ReadDir(fsys, path)
// 		if err != nil {
// 			stop(fn(path, nil, err)) // Notify fn of the error
// 			return
// 		}

// 		for _, entry := range entries {
// 			entryPath := filepath.Join(path, entry.Name())
// 			if err := fn(entryPath, entry, nil); err != nil {
// 				stop(err)
// 				return
// 			}

// 			if entry.IsDir() {
// 				wg.Add(1)
// 				go walk(entryPath)
// 			}
// 		}
// 	}

// 	// Start walking from the root directory
// 	wg.Add(1)
// 	go walk(root)

// 	// Wait for all goroutines or an error
// 	go func() {
// 		wg.Wait()
// 		close(errChan)
// 	}()

// 	// Return the first error, if any
// 	return <-errChan
// }

// func getTotalSize(value interface{}) uintptr {
// 	visited := make(map[uintptr]bool)
// 	return calculateSize(reflect.ValueOf(value), visited)
// }

// func calculateSize(v reflect.Value, visited map[uintptr]bool) uintptr {
// 	switch v.Kind() {
// 	case reflect.Ptr, reflect.Interface:
// 		if v.IsNil() {
// 			return 0
// 		}
// 		ptr := v.Pointer()
// 		if visited[ptr] {
// 			return 0
// 		}
// 		visited[ptr] = true
// 		return unsafe.Sizeof(ptr) + calculateSize(v.Elem(), visited)

// 	case reflect.Array, reflect.Slice:
// 		totalSize := uintptr(0)
// 		for i := 0; i < v.Len(); i++ {
// 			totalSize += calculateSize(v.Index(i), visited)
// 		}
// 		return unsafe.Sizeof(v.Interface()) + totalSize

// 	case reflect.Map:
// 		totalSize := uintptr(0)
// 		for _, key := range v.MapKeys() {
// 			totalSize += calculateSize(key, visited)
// 			totalSize += calculateSize(v.MapIndex(key), visited)
// 		}
// 		return unsafe.Sizeof(v.Interface()) + totalSize

// 	case reflect.Struct:
// 		totalSize := uintptr(0)
// 		for i := 0; i < v.NumField(); i++ {
// 			totalSize += calculateSize(v.Field(i), visited)
// 		}
// 		return unsafe.Sizeof(v.Interface()) + totalSize

// 	default:
// 		return unsafe.Sizeof(v.Interface())
// 	}
// }
