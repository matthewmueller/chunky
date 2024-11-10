package sftp

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/matthewmueller/chunky/internal/repos"
	"github.com/matthewmueller/virt"
	"github.com/pkg/sftp"
	"github.com/zalando/go-keyring"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
)

func Parse(ctx context.Context, url *url.URL, onPassword func(ctx context.Context, prompt string) (string, error)) (ssh.Signer, error) {
	keyFile := url.Query().Get("key")
	if keyFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		keyFile = filepath.Join(home, ".ssh", "id_rsa")
	}

	keyData, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, err
	}

	// Try loading the key without a passphrase
	signer, err := ssh.ParsePrivateKey([]byte(keyData))
	if err != nil {
		if _, ok := err.(*ssh.PassphraseMissingError); !ok {
			return nil, err
		}

		// Try loading from the URL
		if password, ok := url.User.Password(); ok {
			signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(keyData), []byte(password))
			if err != nil {
				return nil, err
			}
			return signer, nil
		}

		// Try fetching from the keyring
		user, err := user.Current()
		if err != nil {
			return nil, err
		}

		// Try loading from the keyring
		if password, err := keyring.Get("Chunky CLI", user.Username); err == nil {
			if signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(keyData), []byte(password)); err == nil {
				return signer, nil
			} else if !errors.Is(err, keyring.ErrNotFound) && !errors.Is(err, x509.IncorrectPasswordError) {
				return nil, err
			}
		}

		// Prompt the user
		password, err := onPassword(ctx, "Enter passphrase for "+keyFile+": ")
		if err != nil {
			return nil, err
		}

		// Test the password one more time
		if signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(keyData), []byte(password)); err != nil {
			return nil, err
		}

		// Upon success, save the password to the keyring for future use
		if err := keyring.Set("Chunky CLI", user.Username, password); err != nil {
			return nil, err
		}
	}

	return signer, nil
}

func Dial(url *url.URL, signer ssh.Signer) (*Client, error) {
	sshc, err := ssh.Dial("tcp", url.Host, &ssh.ClientConfig{
		User:            url.User.Username(),
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		return nil, err
	}
	sftp, err := sftp.NewClient(sshc,
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
		err = errors.Join(err, sftp.Close())
		err = errors.Join(err, sshc.Close())
		return err
	}

	return &Client{sftp, dir, closer}, nil
}

type Client struct {
	sftp   *sftp.Client
	dir    string
	closer func() error
}

var _ repos.Repo = (*Client)(nil)

func (c *Client) Close() (err error) {
	return c.closer()
}

func (c *Client) Upload(ctx context.Context, from fs.FS) error {
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
			if err := c.sftp.MkdirAll(remotePath); err != nil {
				return fmt.Errorf("sftp: unable to create directory %q: %w", path, err)
			}
			// Handle the case where the permissions are 0
			mode := info.Mode()
			if mode.Perm() == 0 {
				mode = fs.FileMode(fs.ModeDir | 0755)
			}
			// Set the permissions
			if err := c.sftp.Chmod(remotePath, mode); err != nil {
				return fmt.Errorf("sftp: unable to chmod directory %q: %w", path, err)
			}
			return nil
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

func (c *Client) uploadFile(from fs.FS, localPath, remotePath string, mode fs.FileMode) error {
	// Open the local file
	localFile, err := from.Open(localPath)
	if err != nil {
		return fmt.Errorf("sftp: unable to open local file %q: %w", localPath, err)
	}
	defer localFile.Close()

	// Open the remote file
	remoteFile, err := c.sftp.OpenFile(remotePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return fmt.Errorf("sftp: unable to open remote file %q: %w", remotePath, err)
	}
	defer remoteFile.Close()

	// Handle the case where the permissions are 0
	if mode.Perm() == 0 {
		mode = fs.FileMode(mode.Type() | 0644)
	}

	// Set the permissions
	if err := remoteFile.Chmod(mode); err != nil {
		return fmt.Errorf("sftp: unable to chmod remote file %q: %w", remotePath, err)
	}

	// Copy the file
	_, err = io.Copy(remoteFile, localFile)
	if err != nil {
		return fmt.Errorf("sftp: unable to copy file %q: %w", localPath, err)
	}

	return nil

}

func (c *Client) Download(ctx context.Context, to virt.FS, paths ...string) error {
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

func (c *Client) downloadFile(to virt.FS, path string) error {
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

func (c *Client) Walk(ctx context.Context, dir string, fn fs.WalkDirFunc) error {
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
