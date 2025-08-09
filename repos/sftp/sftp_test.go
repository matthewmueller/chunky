package sftp_test

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"testing"
	"time"

	"github.com/matryer/is"
	"github.com/matthewmueller/chunky"
	sftp_repo "github.com/matthewmueller/chunky/repos/sftp"
	"github.com/matthewmueller/logs"
	"github.com/matthewmueller/virt"
	"github.com/pkg/sftp"
	"golang.org/x/sync/errgroup"
)

func sftpServer(rootDir string) (*sftp.Client, func() error, error) {
	serverConn, clientConn := net.Pipe()

	server, err := sftp.NewServer(serverConn, sftp.WithServerWorkingDirectory(rootDir))
	if err != nil {
		err = errors.Join(err, serverConn.Close())
		err = errors.Join(err, clientConn.Close())
		return nil, nil, err
	}

	eg := new(errgroup.Group)
	eg.Go(func() error {
		return server.Serve()
	})

	client, err := sftp.NewClientPipe(clientConn, clientConn)
	if err != nil {
		err = errors.Join(err, clientConn.Close())
		err = errors.Join(err, serverConn.Close())
		err = errors.Join(err, server.Close())
		err = errors.Join(err, eg.Wait())
		return nil, nil, err
	}

	closer := func() (err error) {
		err = errors.Join(err, clientConn.Close())
		err = errors.Join(err, client.Close())
		err = errors.Join(err, serverConn.Close())
		err = errors.Join(err, server.Close())
		err = errors.Join(err, eg.Wait())
		return err
	}

	return client, closer, nil
}

func TestUploadDownload(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	log := logs.Default()
	sftpClient, sftpCleanup, err := sftpServer(t.TempDir())
	is.NoErr(err)
	defer sftpCleanup()

	from := virt.Tree{
		"a.txt": &virt.File{Data: []byte("a"), Mode: 0644},
		"b.txt": &virt.File{Data: []byte("b"), Mode: 0644},
		"c.txt": &virt.File{Data: []byte("c"), Mode: 0644},
	}

	repo := sftp_repo.New(sftpClient, "")
	chky := chunky.New(log)
	err = chky.Upload(ctx, &chunky.Upload{
		From:  from,
		To:    repo,
		Cache: virt.OS(t.TempDir()),
	})
	is.NoErr(err)

	// Check that the file exists on the SFTP server
	toFs := virt.OS(t.TempDir())
	err = chky.Download(ctx, &chunky.Download{
		From:     repo,
		To:       toFs,
		Revision: "latest",
	})
	is.NoErr(err)

	data, err := fs.ReadFile(toFs, "a.txt")
	is.NoErr(err)
	is.Equal(string(data), "a")
	data, err = fs.ReadFile(toFs, "b.txt")
	is.NoErr(err)
	is.Equal(string(data), "b")
	data, err = fs.ReadFile(toFs, "c.txt")
	is.NoErr(err)
	is.Equal(string(data), "c")
}

func TestUploadDownloadSymlink(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	log := logs.Default()
	sftpDir := t.TempDir()
	sftpClient, sftpCleanup, err := sftpServer(sftpDir)
	is.NoErr(err)
	defer sftpCleanup()

	from := virt.Tree{
		"from.txt": &virt.File{Data: []byte("to.txt"), Mode: 0644, ModTime: time.Now()},
		"to.txt":   &virt.File{Data: []byte("to content"), Mode: 0644, ModTime: time.Now()},
	}

	repo := sftp_repo.New(sftpClient, "")
	cache := virt.OS(t.TempDir())

	chky := chunky.New(log)
	err = chky.Upload(ctx, &chunky.Upload{
		From:  from,
		To:    repo,
		Cache: cache,
	})
	is.NoErr(err)

	// Check that the file exists on the SFTP server
	toFs := virt.OS(t.TempDir())
	err = chky.Download(ctx, &chunky.Download{
		From:     repo,
		To:       toFs,
		Revision: "latest",
	})
	is.NoErr(err)

	data, err := fs.ReadFile(toFs, "from.txt")
	is.NoErr(err)
	is.Equal(string(data), "to.txt")

	stat, err := toFs.Lstat("from.txt")
	is.NoErr(err)
	is.Equal(stat.Mode(), fs.FileMode(0644))

	data, err = fs.ReadFile(toFs, "to.txt")
	is.NoErr(err)
	is.Equal(string(data), "to content")

	// Change to a symlink
	from["from.txt"].Mode = 0755 | fs.ModeSymlink

	// Try again, with the cache
	err = chky.Upload(ctx, &chunky.Upload{
		From:  from,
		To:    repo,
		Cache: cache,
	})
	is.NoErr(err)

	// Check that the file exists on the SFTP server
	toFs = virt.OS(t.TempDir())
	err = chky.Download(ctx, &chunky.Download{
		From:     repo,
		To:       toFs,
		Revision: "latest",
	})
	is.NoErr(err)

	stat, err = toFs.Lstat("from.txt")
	is.NoErr(err)
	is.Equal(stat.Mode(), 0755|fs.ModeSymlink)

	data, err = fs.ReadFile(toFs, "from.txt")
	is.NoErr(err)
	is.Equal(string(data), "to content")

	data, err = fs.ReadFile(toFs, "to.txt")
	is.NoErr(err)
	is.Equal(string(data), "to content")
}
