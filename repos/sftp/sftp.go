package sftp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/matthewmueller/chunky/repos"
	"github.com/matthewmueller/sshx"
	"github.com/matthewmueller/virt"
	"github.com/pkg/sftp"
	"golang.org/x/sync/errgroup"
)

// Dial an SFTP connection and return a new repository.
func Dial(url *url.URL) (*Repo, error) {
	user, host := url.User.Username(), url.Host

	// Dial the SSH client
	sshClient, err := sshx.Dial(user, host)
	if err != nil {
		return nil, fmt.Errorf("sftp: unable to dial %s@%s: %w", user, host, err)
	}

	// Open a new SFTP session
	sftpClient, err := sftp.NewClient(sshClient,
		sftp.UseConcurrentReads(true),
		sftp.UseConcurrentWrites(true),
	)
	if err != nil {
		return nil, err
	}

	// Get the directory
	dir := strings.TrimPrefix(url.Path, "/")

	// Create the closer
	closer := func() error {
		err = errors.Join(err, sftpClient.Close())
		err = errors.Join(err, sshClient.Close())
		return err
	}

	return &Repo{sftpClient, dir, closer}, nil
}

// New creates a new SFTP repository.
func New(sftp *sftp.Client, dir string) *Repo {
	return &Repo{sftp, dir, func() error { return nil }}
}

type Repo struct {
	sftp   *sftp.Client
	dir    string
	closer func() error
}

var _ repos.Repo = (*Repo)(nil)

func (c *Repo) Close() (err error) {
	return c.closer()
}

func (c *Repo) Upload(ctx context.Context, from fs.FS) error {
	eg := new(errgroup.Group)
	if err := fs.WalkDir(from, ".", func(path string, de fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("sftp: unable to walk %q: %w", path, err)
		}

		remotePath := filepath.Join(c.dir, path)

		// Get the fileinfo about the file
		info, err := de.Info()
		if err != nil {
			return fmt.Errorf("sftp: unable to get file info for %q: %w", path, err)
		}

		// Handle creating directories
		if de.IsDir() {
			return mkdirAll(c.sftp, remotePath, info.Mode())
		}

		// Handle file uploads concurrently
		eg.Go(func() error {
			return c.uploadFile(from, path, remotePath, info.Mode())
		})

		return nil
	}); err != nil {
		return fmt.Errorf("sftp: unable to walk directory: %w", err)
	}
	if err := eg.Wait(); err != nil {
		return fmt.Errorf("sftp: unable to upload files: %w", err)
	}
	return nil
}

func (c *Repo) uploadFile(from fs.FS, localPath, remotePath string, mode fs.FileMode) error {
	// Read data from the local file
	data, err := fs.ReadFile(from, localPath)
	if err != nil {
		return fmt.Errorf("sftp: unable to read local file %q: %w", localPath, err)
	}

	// Write to the remote file
	return writeFile(c.sftp, remotePath, data, mode)
}

func (c *Repo) Download(ctx context.Context, to virt.FS, paths ...string) error {
	eg := new(errgroup.Group)
	for _, path := range paths {
		eg.Go(func() error {
			return c.downloadFile(to, path)
		})
	}
	if err := eg.Wait(); err != nil {
		return fmt.Errorf("sftp: unable to download files: %w", err)
	}
	return nil
}

func (c *Repo) downloadFile(to virt.FS, path string) error {
	remotePath := filepath.Join(c.dir, path)
	remoteFile, err := c.sftp.Open(remotePath)
	if err != nil {
		return fmt.Errorf("sftp: unable to open remote file %q: %w", remotePath, err)
	}
	defer remoteFile.Close()
	fileInfo, err := remoteFile.Stat()
	if err != nil {
		return fmt.Errorf("sftp: unable to stat remote file %q: %w", remotePath, err)
	}
	// Handle directories
	if fileInfo.IsDir() {
		if err := to.MkdirAll(path, fileInfo.Mode()); err != nil {
			return fmt.Errorf("sftp: unable to create directory %q: %w", path, err)
		}
		return nil
	}
	// Handle files
	data, err := io.ReadAll(remoteFile)
	if err != nil {
		return fmt.Errorf("sftp: unable to read remote file %q: %w", remotePath, err)
	}
	return to.WriteFile(path, data, fileInfo.Mode())
}

func (c *Repo) Walk(ctx context.Context, dir string, fn fs.WalkDirFunc) error {
	walker := c.sftp.Walk(filepath.Join(c.dir, dir))
	for walker.Step() {
		if err := walker.Err(); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("sftp: unable to walk %q: %w", dir, err)
		}
		info := walker.Stat()
		de := fs.FileInfoToDirEntry(info)
		rel, err := filepath.Rel(c.dir, walker.Path())
		if err != nil {
			return fmt.Errorf("sftp: unable to get relative path: %w", err)
		}
		if err := fn(rel, de, nil); err != nil {
			return err
		}
	}
	return nil
}
