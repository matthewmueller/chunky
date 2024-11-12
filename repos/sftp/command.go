package sftp

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/matthewmueller/text"
	"github.com/pkg/sftp"
)

func Load(url *url.URL) (*Client, error) {
	cmd := &Command{
		Stderr: os.Stderr,
		Env:    map[string]string{},
	}
	return cmd.Load(url)
}

// Command for running the SFTP CLI.
type Command struct {
	Dir    string
	Stderr io.Writer
	Env    map[string]string
}

// New returns a new sftp repo. Unlike the sftp.Dial function, this function
// uses the underlying `ssh` CLI installed on your OS.
func (c *Command) Load(url *url.URL) (*Client, error) {
	// Get the user and host
	userHost := url.User.Username() + "@" + url.Hostname()
	port := url.Port()
	if port == "" {
		port = "22"
	}

	// Create the command
	cmd := exec.Command("ssh", userHost, "-p", port, "-s", "sftp")
	cmd.Dir = c.Dir

	// Set the environment variables
	for k, v := range c.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	// Send errors from ssh to stderr
	cmd.Stderr = c.Stderr

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

	// Generate a unique key for this repo
	key := text.Slug(url.User.Username() + " " + url.Host + " " + url.Path)

	// Get the directory
	dir := strings.TrimPrefix(url.Path, "/")

	// Create the closer
	closer := func() error {
		err = errors.Join(err, sftp.Close())
		err = errors.Join(err, cmd.Wait())
		return err
	}

	return &Client{key, sftp, dir, closer}, nil
}
