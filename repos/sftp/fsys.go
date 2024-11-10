package sftp

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"

	"github.com/matthewmueller/virt"
)

var _ virt.FS = (*Client)(nil)
var _ fs.ReadDirFS = (*Client)(nil)

func (c *Client) Open(name string) (fs.File, error) {
	return c.sftp.Open(path.Join(c.dir, name))
}

func (c *Client) ReadDir(name string) ([]fs.DirEntry, error) {
	fis, err := c.sftp.ReadDir(path.Join(c.dir, name))
	if err != nil {
		return nil, fmt.Errorf("sftp: unable to read directory %q: %w", name, err)
	}
	des := make([]fs.DirEntry, len(fis))
	for i, fi := range fis {
		des[i] = fs.FileInfoToDirEntry(fi)
	}
	return des, nil
}

func (c *Client) MkdirAll(name string, perm fs.FileMode) error {
	fpath := path.Join(c.dir, name)

	// Create the directory
	if err := c.sftp.MkdirAll(fpath); err != nil {
		return fmt.Errorf("sftp: unable to create directory %q: %w", name, err)
	}

	// Handle the case where the permissions are 0
	if perm.Perm() == 0 {
		perm = fs.FileMode(fs.ModeDir | 0755)
	}

	// Set the permissions
	if err := c.sftp.Chmod(fpath, perm); err != nil {
		return fmt.Errorf("sftp: unable to chmod directory %q: %w", name, err)
	}

	return nil
}

func (c *Client) WriteFile(name string, data []byte, mode fs.FileMode) error {
	fpath := path.Join(c.dir, name)

	// Open the remote file
	remoteFile, err := c.sftp.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return fmt.Errorf("sftp: unable to open remote file %q: %w", name, err)
	}
	defer remoteFile.Close()

	// Handle the case where the permissions are 0
	if mode.Perm() == 0 {
		mode = fs.FileMode(mode.Type() | 0644)
	}

	// Set the permissions
	if err := remoteFile.Chmod(mode); err != nil {
		return fmt.Errorf("sftp: unable to chmod remote file %q: %w", name, err)
	}

	// Write the data to the remote file
	if _, err := remoteFile.ReadFrom(bytes.NewReader(data)); err != nil {
		return fmt.Errorf("sftp: unable to write to remote file %q: %w", name, err)
	}

	return nil
}

func (c *Client) RemoveAll(name string) error {
	return c.sftp.RemoveAll(path.Join(c.dir, name))
}
