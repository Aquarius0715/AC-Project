package integration

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"testing"
)

// tinyPNG and tinyJPEG are real one-pixel images: an upload is read down to its header (IR389).
func tinyPNG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewGray(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func tinyJPEG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewGray(image.Rect(0, 0, 1, 1)), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// pngHeader is the PNG signature and an IHDR chunk declaring w×h gray pixels: a pixel flood in a few dozen bytes.
func pngHeader(w, h uint32) []byte {
	ihdr := binary.BigEndian.AppendUint32(binary.BigEndian.AppendUint32([]byte("IHDR"), w), h)
	ihdr = append(ihdr, 8, 0, 0, 0, 0)
	out := binary.BigEndian.AppendUint32([]byte("\x89PNG\r\n\x1a\n"), 13)
	return binary.BigEndian.AppendUint32(append(out, ihdr...), crc32.ChecksumIEEE(ihdr))
}
