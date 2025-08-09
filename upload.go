package chunky

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os/user"
	"path"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/matthewmueller/chunky/internal/caches"
	"github.com/matthewmueller/chunky/internal/chunkyignore"
	"github.com/matthewmueller/chunky/internal/commits"
	"github.com/matthewmueller/chunky/internal/rate"
	"github.com/matthewmueller/chunky/internal/sha256"
	"github.com/matthewmueller/chunky/internal/uploads"
	"github.com/matthewmueller/chunky/repos"
	"github.com/matthewmueller/logs"
	"github.com/matthewmueller/virt"
	"golang.org/x/sync/errgroup"
)

type Upload struct {
	From   repos.ReadFS
	To     repos.Repo
	Cache  repos.FS
	User   string
	Tags   []string
	Ignore func(path string) bool

	// MaxPackSize is the maximum pack size (default: 32MiB)
	MaxPackSize string
	maxPackSize int

	// MinChunkSize is the minimum chunk size (default: 512KiB)
	MinChunkSize string
	minChunkSize int

	// MaxChunkSize is the maximum chunk size (default: 8MiB)
	MaxChunkSize string
	maxChunkSize int

	// LimitUpload is the maximum upload rate (default: unlimited)
	LimitUpload string
	limitUpload int

	// Concurrency is the number of files to upload concurrently (default: num cpus * 2)
	Concurrency *int
	concurrency int
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

	// Parse the max pack size
	if in.MaxPackSize != "" {
		maxPackSize, err2 := humanize.ParseBytes(in.MaxPackSize)
		if err2 != nil {
			err = errors.Join(err, fmt.Errorf("invalid max pack size: %w", err2))
		} else {
			in.maxPackSize = int(maxPackSize)
		}
	} else {
		in.maxPackSize = 32 * miB
	}

	// Parse the min chunk size
	if in.MinChunkSize != "" {
		minChunkSize, err2 := humanize.ParseBytes(in.MinChunkSize)
		if err2 != nil {
			err = errors.Join(err, fmt.Errorf("invalid min chunk size: %w", err2))
		} else {
			in.minChunkSize = int(minChunkSize)
		}
	} else {
		in.minChunkSize = 512 * kiB
	}

	// Parse the max chunk size
	if in.MaxChunkSize != "" {
		maxChunkSize, err2 := humanize.ParseBytes(in.MaxChunkSize)
		if err2 != nil {
			err = errors.Join(err, fmt.Errorf("invalid max chunk size: %w", err2))
		} else {
			in.maxChunkSize = int(maxChunkSize)
		}
	} else {
		in.maxChunkSize = 8 * miB
	}

	// Parse the upload limit
	if in.LimitUpload != "" {
		uploadLimit, err2 := humanize.ParseBytes(in.LimitUpload)
		if err2 != nil {
			err = errors.Join(err, fmt.Errorf("invalid upload limit: %w", err2))
		} else {
			in.limitUpload = int(uploadLimit)
		}
	} else {
		in.limitUpload = 0 // unlimited
	}

	// Ensure the min chunk size is less than the max chunk size
	if in.minChunkSize > in.maxChunkSize {
		err = errors.Join(err, errors.New("min chunk size cannot be greater than max chunk size"))
	}

	// Ensure the max pack size is greater than the max chunk size
	if in.maxPackSize < in.maxChunkSize {
		err = errors.Join(err, errors.New("max pack size cannot be less than max chunk size"))
	}

	// Set the concurrency if provided
	if in.Concurrency != nil {
		in.concurrency = *in.Concurrency
		// Uploads aren't setup for unlimited concurrency at the moment
		if in.concurrency <= 0 {
			err = errors.Join(err, errors.New("invalid concurrency"))
		}
	} else {
		in.concurrency = DefaultConcurrency
	}

	return err
}

// Upload a directory to a repository
func (c *Client) Upload(ctx context.Context, in *Upload) error {
	if err := in.validate(); err != nil {
		return err
	}

	log := logs.Scope(c.log)

	// Download the latest commits from the cache
	cache, err := caches.Download(ctx, in.To, in.Cache)
	if err != nil {
		return err
	}

	ignore := in.Ignore
	createdAt := time.Now().UTC()
	commit := commits.New(in.User, createdAt)
	commitId := commit.ID()

	uploadCh := make(chan *repos.File, in.concurrency)
	// Start the upload workers
	eg := new(errgroup.Group)
	for i := 0; i < in.concurrency; i++ {
		eg.Go(func() error {
			return in.To.Upload(ctx, uploadCh)
		})
	}

	upload := uploads.New(log, uploadCh)
	upload.MaxPackSize = in.maxPackSize
	upload.MinChunkSize = in.minChunkSize
	upload.MaxChunkSize = in.maxChunkSize
	upload.Limiter = rate.New(in.limitUpload)

	// Walk over the files, chunk them and add them to the file system we're going
	// to upload. We'll also add each file to the commit object.
	if err := fs.WalkDir(in.From, ".", func(fpath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		} else if d.IsDir() {
			if ignore(fpath) {
				log.Debug("ignoring directory", slog.String("path", fpath))
				return fs.SkipDir
			}
			return nil
		} else if ignore(fpath) {
			log.Debug("ignoring file", slog.String("path", fpath))
			return nil
		}

		// Hash the file into a sha256 hash, this reads the file in chunks, rather
		// than loading the entire file into memory.
		fileHash, err := sha256.HashFile(in.From, fpath, in.maxChunkSize)
		if err != nil {
			return fmt.Errorf("unable to hash file %q: %w", fpath, err)
		}

		// Check if the file is already in the pack. This will duplicate content
		// if the file path in the pack is different from the file path in the
		// commit. To fix this, we also ensure the file paths are the same.
		// TODO: We should add a way to alias files in the pack to other packs.
		if cacheFile, ok := cache.Get(fpath, fileHash); ok {
			log.Debug("file already in cache", slog.String("path", fpath))
			commit.Add(cacheFile)
			return nil
		}

		// Get the file lstat
		lstat, err := in.From.Lstat(fpath)
		if err != nil {
			return err
		}

		// Create a reader for the file data, handling symlinks
		reader, err := openReader(in.From, fpath, lstat)
		if err != nil {
			return err
		}

		// Add the file to the pack
		packId, err := upload.Add(ctx, &uploads.File{
			Reader:  reader,
			Path:    fpath,
			Hash:    fileHash,
			Mode:    lstat.Mode(),
			Size:    lstat.Size(),
			ModTime: lstat.ModTime(),
		})
		if err != nil {
			return err
		}

		log.Debug("added file to pack",
			slog.String("path", fpath),
			slog.String("pack_id", packId),
		)

		// Add the file to the commit
		commit.Add(&commits.File{
			Path:   fpath,
			Id:     fileHash,
			PackId: packId,
			Size:   uint64(lstat.Size()),
		})

		return nil
	}); err != nil {
		close(uploadCh)
		return err
	}

	// Upload any remaining files in the pack
	if err := upload.Flush(ctx); err != nil {
		close(uploadCh)
		return err
	}

	// Add the commit to the tree
	commitData, err := commit.Pack()
	if err != nil {
		close(uploadCh)
		return err
	}
	uploadCh <- &repos.File{
		Path:    path.Join("commits", commitId),
		Data:    commitData,
		Mode:    0644,
		ModTime: createdAt,
	}

	// Add the commit to the cache
	if err := cache.Set(commitId, commit); err != nil {
		close(uploadCh)
		return err
	}

	// Add the latest ref
	uploadCh <- &repos.File{
		Path: path.Join("tags", "latest"),
		Data: []byte(commitId),
		Mode: 0644,
	}

	// Tag the revision
	for _, tag := range in.Tags {
		uploadCh <- &repos.File{
			Path: fmt.Sprintf("tags/%s", tag),
			Data: []byte(commitId),
			Mode: 0644,
		}
	}

	close(uploadCh)
	return eg.Wait()
}

// Open a file from the filesystem, handling symlinks. For symlinks, the
// link target is the file data.
func openReader(fsys virt.FromFS, path string, info fs.FileInfo) (io.Reader, error) {
	if info.Mode()&fs.ModeSymlink != 0 {
		// If the file is a symlink, read the link target
		link, err := fsys.Readlink(path)
		if err != nil {
			return nil, fmt.Errorf("unable to read symlink %q: %w", path, err)
		}
		return strings.NewReader(link), nil
	}
	// Otherwise, read the file data
	return fsys.Open(path)
}
