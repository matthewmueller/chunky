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
)

type Upload struct {
	From repos.ReadFS
	To   repos.Writer
	// Cache remembers what was uploaded, so the next upload skips it (default:
	// none)
	Cache repos.FS

	// Missing lists the files To is missing, from Client.Missing. When set,
	// only those are sent and the rest are referenced for Receive to resolve,
	// so Cache isn't read or written.
	Missing Manifest

	User   string
	Tags   []string
	Paths  []string
	Ignore func(string) bool

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
	if in.Cache == nil {
		in.Cache = virt.Tree{}
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
		switch tag {
		case "latest":
			err = errors.Join(err, errors.New("tag cannot be 'latest'"))
		case "":
			err = errors.Join(err, errors.New("tag cannot be empty"))
		}
	}

	// Default to the current directory
	if len(in.Paths) == 0 {
		in.Paths = []string{"."}
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
func (c *Client) Upload(ctx context.Context, in *Upload) (revision string, err error) {
	if err := in.validate(); err != nil {
		return "", fmt.Errorf("invalid upload input: %w", err)
	}
	if in.To == nil {
		return "", errors.New("invalid upload input: missing to repository")
	}

	log := logs.Scope(c.log)

	// Without Missing, the cache decides which files To already has
	cache, err := caches.Load(c.log, in.Cache)
	if err != nil {
		return "", fmt.Errorf("unable to load the cache: %w", err)
	}

	// Sync the cache with To when we can read it
	if repo, ok := in.To.(repos.Repo); ok && in.Missing == nil {
		now := time.Now()
		if err := cache.Download(ctx, repo); err != nil {
			return "", fmt.Errorf("unable to download latest commits from the cache: %w", err)
		}
		log.Debug("downloaded latest commits from the cache", slog.Duration("in", time.Since(now)))
	}

	createdAt := time.Now().UTC()
	commit := commits.New(in.User, createdAt)
	commitId := commit.ID()

	upload := uploads.New(log, in.To, in.concurrency)
	upload.MaxPackSize = in.maxPackSize
	upload.MinChunkSize = in.minChunkSize
	upload.MaxChunkSize = in.maxChunkSize
	upload.Limiter = rate.New(in.limitUpload)

	// Chunk the files into packs, and add each file to the commit
	if err := in.walk(func(fpath, fileHash string) error {
		// Get the file lstat
		lstat, err := in.From.Lstat(fpath)
		if err != nil {
			return fmt.Errorf("unable to lstat file %q: %w", fpath, err)
		}

		// Reference the files To has. Receive resolves their packs.
		if in.Missing != nil && in.Missing[fpath] != fileHash {
			commit.Add(&commits.File{
				Path: fpath,
				Id:   fileHash,
				Size: uint64(lstat.Size()),
			})
			return nil
		}

		// Check if the file is already in the pack. This will duplicate content
		// if the file path in the pack is different from the file path in the
		// commit. To fix this, we also ensure the file paths are the same.
		// TODO: We should add a way to alias files in the pack to other packs.
		if in.Missing == nil {
			if cacheFile, ok := cache.Get(fpath, fileHash); ok {
				log.Debug("file already in cache", slog.String("path", fpath))
				commit.Add(cacheFile)
				return nil
			}
		}

		// Create a reader for the file data, handling symlinks
		reader, err := openReader(in.From, fpath, lstat)
		if err != nil {
			return fmt.Errorf("unable to open reader %q: %w", fpath, err)
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
			return fmt.Errorf("unable to add file %q to pack: %w", fpath, err)
		}

		log.Debug("added file to pack",
			slog.String("commit_id", commitId),
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
		return "", err
	}

	// Upload any remaining files in the pack before committing
	if err := upload.Flush(ctx); err != nil {
		return "", fmt.Errorf("unable to flush pack before committing: %w", err)
	}

	// Upload the commit file directly
	commitData, err := commit.Pack()
	if err != nil {
		return "", fmt.Errorf("unable to pack commit %q: %w", commitId, err)
	}
	if err := upload.Direct(ctx, &repos.File{
		Path:    path.Join("commits", commitId),
		Data:    commitData,
		Mode:    0644,
		ModTime: createdAt,
	}); err != nil {
		return "", fmt.Errorf("unable to upload commit %q: %w", commitId, err)
	}

	// Upload the tags concurrently
	tags := append([]string{"latest"}, in.Tags...)
	tagFiles := make([]*repos.File, len(tags))
	for i, tag := range tags {
		tagFiles[i] = &repos.File{
			Path: path.Join("tags", tag),
			Data: []byte(commitId),
			Mode: 0644,
		}
	}
	if err := upload.Direct(ctx, tagFiles...); err != nil {
		return "", fmt.Errorf("unable to upload tags for commit %q: %w", commitId, err)
	}

	// Cache the commit, so the next upload can skip its files without reading
	// To. With Missing, the commit's references have no packs to cache.
	if in.Missing == nil {
		if err := cache.Add(commit); err != nil {
			return "", fmt.Errorf("unable to cache commit %q: %w", commitId, err)
		}
	}

	return commitId, nil
}

// walk the upload's files that aren't ignored, with their hashes
func (in *Upload) walk(fn func(fpath, hash string) error) error {
	for _, p := range in.Paths {
		if err := fs.WalkDir(in.From, p, func(fpath string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			} else if in.Ignore(fpath) {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			} else if d.IsDir() {
				return nil
			}
			// Hash the file into a sha256 hash, this reads the file in chunks, rather
			// than loading the entire file into memory.
			hash, err := sha256.HashFile(in.From, fpath, in.maxChunkSize)
			if err != nil {
				return fmt.Errorf("unable to hash file %q: %w", fpath, err)
			}
			return fn(fpath, hash)
		}); err != nil {
			return fmt.Errorf("unable to walk directory %q: %w", p, err)
		}
	}
	return nil
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
