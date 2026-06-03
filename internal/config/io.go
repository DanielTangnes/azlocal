package config

import "bytes"

// bytesReader is a tiny helper to avoid pulling strings/bytes packages
// into the public surface; isolated for easy testing/mocking.
func bytesReader(b []byte) *bytes.Reader {
	return bytes.NewReader(b)
}
