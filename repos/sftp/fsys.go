package sftp

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"

	"github.com/matthewmueller/chunky/repos"
	"github.com/pkg/sftp"
)

var _ fs.ReadDirFS = (*Repo)(nil)
var _ fs.StatFS = (*Repo)(nil)
var _ repos.FS = (*Repo)(nil)

func (r *Repo) Open(name string) (fs.File, error) {
	return r.sftp.Open(path.Join(r.dir, name))
}
func (r *Repo) Stat(name string) (fs.FileInfo, error) {
	return r.sftp.Stat(path.Join(r.dir, name))
}

func (r *Repo) ReadDir(name string) ([]fs.DirEntry, error) {
	fis, err := r.sftp.ReadDir(path.Join(r.dir, name))
	if err != nil {
		return nil, fmt.Errorf("sftp: unable to read directory %q: %w", name, err)
	}
	des := make([]fs.DirEntry, len(fis))
	for i, fi := range fis {
		des[i] = fs.FileInfoToDirEntry(fi)
	}
	return des, nil
}

func (r *Repo) MkdirAll(name string, perm fs.FileMode) error {
	return mkdirAll(r.sftp, path.Join(r.dir, name), perm)
}

func mkdirAll(sftp *sftp.Client, fpath string, perm fs.FileMode) error {
	// Create the directory
	if err := sftp.MkdirAll(fpath); err != nil {
		return fmt.Errorf("sftp: unable to create directory %q: %w", fpath, err)
	}

	// Handle the case where the permissions are 0
	if perm.Perm() == 0 {
		perm = fs.FileMode(fs.ModeDir | 0755)
	}

	// Set the permissions
	if err := sftp.Chmod(fpath, perm); err != nil {
		return fmt.Errorf("sftp: unable to chmod directory %q: %w", fpath, err)
	}

	return nil
}

func (r *Repo) WriteFile(name string, data []byte, mode fs.FileMode) error {
	return writeFile(r.sftp, path.Join(r.dir, name), data, mode)
}

func writeFile(sftp *sftp.Client, name string, data []byte, mode fs.FileMode) error {
	// Open the remote file
	remoteFile, err := sftp.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
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

func (r *Repo) RemoveAll(name string) error {
	return r.sftp.RemoveAll(path.Join(r.dir, name))
}
