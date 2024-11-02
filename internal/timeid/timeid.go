package timeid

import (
	"time"
)

// Encode encodes a time in seconds into a sortable string format (YYYYMMDDHHMMSS).
func Encode(t time.Time) string {
	return t.Format("20060102150405")
}

func Decode(s string) (time.Time, error) {
	return time.Parse("20060102150405", s)
}
