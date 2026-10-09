package photoimage

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func fixture(t *testing.T, jpg bool) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	var b bytes.Buffer
	var e error
	if jpg {
		e = jpeg.Encode(&b, img, nil)
	} else {
		e = png.Encode(&b, img)
	}
	if e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestCanonicalPixelsLimitsAndMetadata(t *testing.T) {
	pngData := fixture(t, false)
	out, e := Decode(pngData)
	if e != nil || out.MediaType != "image/png" || out.Width != 4 || out.Height != 2 {
		t.Fatal("PNG conversion")
	}
	img, _, e := image.Decode(bytes.NewReader(out.Content))
	if e != nil {
		t.Fatal(e)
	}
	_, _, _, alpha := img.At(3, 1).RGBA()
	if alpha != 0 {
		t.Fatal("transparency lost")
	}
	jpg := fixture(t, true)
	tag := []byte("private metadata")
	app := append([]byte{0xff, 0xe1, 0, byte(len(tag) + 2)}, tag...)
	withTag := append(append(append([]byte{}, jpg[:2]...), app...), jpg[2:]...)
	out, e = Decode(withTag)
	if e != nil || out.MediaType != "image/jpeg" || bytes.Contains(out.Content, tag) {
		t.Fatal("metadata retained")
	}
	for _, bad := range [][]byte{nil, []byte("<svg></svg>"), []byte("GIF89a"), make([]byte, MaxBytes+1), jpg[:8]} {
		if _, e := Decode(bad); e != ErrInvalid {
			t.Fatal("invalid image accepted")
		}
	}
	// PNG dimensions are checked before decoding/allocating pixel storage.
	bomb := append([]byte{}, pngData...)
	binary.BigEndian.PutUint32(bomb[16:20], 4097)
	if _, e := Decode(bomb); e != ErrInvalid {
		t.Fatal("oversized dimensions accepted")
	}
	// One APP1 EXIF segment with orientation 6 rotates 4x2 into 2x4.
	exif := []byte{'E', 'x', 'i', 'f', 0, 0, 'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0}
	segment := append([]byte{0xff, 0xe1, 0, byte(len(exif) + 2)}, exif...)
	rotated := append(append(append([]byte{}, jpg[:2]...), segment...), jpg[2:]...)
	out, e = Decode(rotated)
	if e != nil || out.Width != 2 || out.Height != 4 || bytes.Contains(out.Content, []byte("Exif")) {
		t.Fatal("orientation or stripping failed")
	}
}
func TestAllOrientationMappings(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			src.Set(x, y, color.NRGBA{R: byte(y*3 + x + 1), A: 255})
		}
	}
	expected := [][]byte{{1, 2, 3, 4, 5, 6}, {3, 2, 1, 6, 5, 4}, {6, 5, 4, 3, 2, 1}, {4, 5, 6, 1, 2, 3}, {1, 4, 2, 5, 3, 6}, {4, 1, 5, 2, 6, 3}, {6, 3, 5, 2, 4, 1}, {3, 6, 2, 5, 1, 4}}
	for orientation := 1; orientation <= 8; orientation++ {
		img := orient(src, orientation)
		i := 0
		for y := 0; y < img.Bounds().Dy(); y++ {
			for x := 0; x < img.Bounds().Dx(); x++ {
				r, _, _, _ := img.At(x, y).RGBA()
				if byte(r>>8) != expected[orientation-1][i] {
					t.Fatal("wrong pixel transform")
				}
				i++
			}
		}
	}
}
