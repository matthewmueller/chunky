package sftp

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/pkg/sftp"
)

// New returns a new sftp repo. Unlike the sftp.Dial function, this function
// uses the underlying `ssh` CLI installed on your OS.
func Load(url *url.URL) (*Client, error) {
	// Get the user and host
	userHost := url.User.Username() + "@" + url.Hostname()
	port := url.Port()
	if port == "" {
		port = "22"
	}

	// Create the command
	cmd := exec.Command("ssh", userHost, "-p", port, "-s", "sftp")

	// Send errors from ssh to stderr
	cmd.Stderr = os.Stderr

	// Get stdin pipe
	wr, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("sftp: getting stdin pipe: %w", err)
	}

	// Get stdout pipe
	rd, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("sftp: getting stdout pipe: %w", err)
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("sftp: starting command: %w", err)
	}

	// Open the SFTP session
	sftp, err := sftp.NewClientPipe(rd, wr,
		sftp.UseConcurrentReads(true),
		sftp.UseConcurrentWrites(true),
	)
	if err != nil {
		return nil, fmt.Errorf("sftp: creating sftp client: %w", err)
	}

	// Get the directory
	dir := strings.TrimPrefix(url.Path, "/")

	// Create the closer
	closer := func() error {
		err = errors.Join(err, sftp.Close())
		err = errors.Join(err, cmd.Wait())
		return err
	}

	return &Client{sftp, dir, closer}, nil
}
