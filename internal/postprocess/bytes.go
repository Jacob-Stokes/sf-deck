package postprocess

import "bytes"

// bytesReader returns an io.Reader over a byte slice without taking a
// dependency on bytes.NewReader from the public API.
func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }
