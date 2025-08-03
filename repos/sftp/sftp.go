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
	"github.com/pkg/sftp"
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

func (c *Repo) Upload(ctx context.Context, from *repos.File) error {
	remotePath := filepath.Join(c.dir, from.Path)

	// Handle creating directories
	if from.IsDir() {
		return mkdirAll(c.sftp, remotePath, from.Mode)
	}

	return c.uploadFile(from, remotePath)
}

func (c *Repo) uploadFile(file *repos.File, remotePath string) error {
	if err := writeFile(c.sftp, remotePath, file.Data, file.Mode); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("sftp: unable to write file %q: %w", remotePath, err)
		}
		if err := mkdirAll(c.sftp, filepath.Dir(remotePath), 0755); err != nil {
			return fmt.Errorf("sftp: unable to create directory %q: %w", remotePath, err)
		}
		if err := writeFile(c.sftp, remotePath, file.Data, file.Mode); err != nil {
			return fmt.Errorf("sftp: unable to write file %q: %w", remotePath, err)
		}
	}
	return nil
}

// TODO: this implementation conceptually differs from the local repo implementation
func (c *Repo) Download(ctx context.Context, path string) (*repos.File, error) {
	return c.downloadFile(path)
	// eg := new(errgroup.Group)
	// for _, path := range paths {
	// 	eg.Go(func() error {

	// 	})
	// }
	// if err := eg.Wait(); err != nil {
	// 	return fmt.Errorf("sftp: unable to download files: %w", err)
	// }
	// return nil
}

func (c *Repo) downloadFile(path string) (*repos.File, error) {
	remotePath := filepath.Join(c.dir, path)
	remoteFile, err := c.sftp.Open(remotePath)
	if err != nil {
		return nil, fmt.Errorf("sftp: unable to open remote file for download %q: %w", remotePath, err)
	}
	defer remoteFile.Close()
	fileInfo, err := remoteFile.Stat()
	if err != nil {
		return nil, fmt.Errorf("sftp: unable to stat remote file %q: %w", remotePath, err)
	}
	// Handle directories
	if fileInfo.IsDir() {
		return &repos.File{
			Path: path,
			Mode: fileInfo.Mode(),
		}, nil
	}
	// Handle files
	data, err := io.ReadAll(remoteFile)
	if err != nil {
		return nil, fmt.Errorf("sftp: unable to read remote file %q: %w", remotePath, err)
	}
	return &repos.File{
		Path: path,
		Data: data,
		Mode: fileInfo.Mode(),
	}, nil
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
