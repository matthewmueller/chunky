package timeid

import (
	"time"
)

const layout = "20060102150405"

// Encode encodes a time in seconds into a sortable string format (YYYYMMDDHHMMSS).
func Encode(t time.Time) string {
	return t.Format(layout)
}

// Decode decodes a string from Encode back into a time
func Decode(s string) (time.Time, error) {
	return time.Parse(layout, s)
}
