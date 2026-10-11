package maintenance

import (
	"strings"
	"testing"
)

// IR368: an upload's name is a label. Names in any script pass; control characters, path separators and
// bidirectional formatting characters do not.
func TestValidFileName(t *testing.T) {
	for _, n := range []string{"photo.jpg", "写真 1.jpg", "Foto unit (2).png", " padded.pdf ", "ملف.png", strings.Repeat("a", 200)} {
		if !ValidFileName(n) {
			t.Errorf("%q refused", n)
		}
	}
	for _, n := range []string{"", "   ", strings.Repeat("a", 201), "a\r\nSet-Cookie: x.png", "nul\x00.png", "tab\t.png", "del\x7f.png", "c1\u0085.png",
		"../../etc/passwd", "a/b.png", `a\b.png`, "photo‮gpj.exe", "x⁦y.png", "lrm‎.png", "alm؜.png"} {
		if ValidFileName(n) {
			t.Errorf("%q accepted", n)
		}
	}
	png := []byte("\x89PNG\r\n\x1a\nx")
	if !ValidBlob(BlobInput{Name: "p.png", Mime: "image/png", Size: len(png), Bytes: png}, 100) {
		t.Error("a valid photo is refused")
	}
	if ValidBlob(BlobInput{Name: "p‮gnp.exe", Mime: "image/png", Size: len(png), Bytes: png}, 100) {
		t.Error("a name with a right-to-left override is accepted")
	}
}
