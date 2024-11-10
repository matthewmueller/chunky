package chunky

import (
	"log/slog"
)

func New(log *slog.Logger) *Client {
	return &Client{log}
}

type Client struct {
	log *slog.Logger
}
