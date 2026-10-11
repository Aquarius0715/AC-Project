package maintenance

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"testing"
)

// tinyPNG and tinyJPEG are real one-pixel images.
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

// pngHeader is the PNG signature and an IHDR chunk declaring w×h gray pixels: all that a reader of the header sees.
func pngHeader(w, h uint32) []byte {
	ihdr := binary.BigEndian.AppendUint32(binary.BigEndian.AppendUint32([]byte("IHDR"), w), h)
	ihdr = append(ihdr, 8, 0, 0, 0, 0) // 8-bit gray, no interlace
	out := binary.BigEndian.AppendUint32([]byte("\x89PNG\r\n\x1a\n"), 13)
	return binary.BigEndian.AppendUint32(append(out, ihdr...), crc32.ChecksumIEEE(ihdr))
}

// jpegSized is a real JPEG whose frame header (SOF0) declares w×h pixels.
func jpegSized(t *testing.T, w, h uint16) []byte {
	t.Helper()
	b := tinyJPEG(t)
	i := bytes.Index(b, []byte{0xFF, 0xC0})
	if i < 0 {
		t.Fatal("no SOF0 marker")
	}
	binary.BigEndian.PutUint16(b[i+5:], h)
	binary.BigEndian.PutUint16(b[i+7:], w)
	return b
}

// TestImageSizeOK: an image is read only as far as its header, and refused when it is not the declared type or
// declares more than 16,384 pixels a side or 50 megapixels (IR389), however few bytes it has.
func TestImageSizeOK(t *testing.T) {
	for name, c := range map[string]struct {
		mime string
		b    []byte
		want bool
	}{
		"one-pixel png":         {"image/png", tinyPNG(t), true},
		"one-pixel jpeg":        {"image/jpeg", tinyJPEG(t), true},
		"phone photo":           {"image/jpeg", jpegSized(t, 4032, 3024), true},
		"16384 × 3000":          {"image/png", pngHeader(16384, 3000), true},
		"16385 wide":            {"image/png", pngHeader(16385, 10), false},
		"64 megapixels":         {"image/png", pngHeader(8000, 8000), false},
		"jpeg of 60000 × 60000": {"image/jpeg", jpegSized(t, 60000, 60000), false},
		"png named jpeg":        {"image/jpeg", tinyPNG(t), false},
		"signature only":        {"image/png", []byte("\x89PNG\r\n\x1a\nx"), false},
		"pdf":                   {"application/pdf", []byte("%PDF-1.7"), false},
	} {
		if got := ImageSizeOK(c.mime, c.b); got != c.want {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
	// ValidBlob applies it: a few hundred bytes that declare a pixel flood are refused
	flood := pngHeader(50000, 50000)
	if ValidBlob(BlobInput{Name: "flood.png", Mime: "image/png", Size: len(flood), Bytes: flood}, 5<<20) {
		t.Error("a PNG declaring 2.5 gigapixels passes")
	}
}
