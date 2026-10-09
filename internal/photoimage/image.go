// Package photoimage bounds and canonicalizes merchant photo uploads.
package photoimage

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
)

const MaxBytes = 5 * 1024 * 1024

var ErrInvalid = errors.New("use a JPEG or PNG up to 5 MiB, 4096 pixels per side and 8 megapixels")
var ErrBusy = errors.New("image processing busy")
var slots = make(chan struct{}, 2)

type Image struct {
	Content       []byte
	MediaType     string
	Width, Height int32
}

// Decode checks dimensions before allocation, then re-encodes pixels without metadata.
func Decode(data []byte) (Image, error) {
	if len(data) == 0 || len(data) > MaxBytes {
		return Image{}, ErrInvalid
	}
	select {
	case slots <- struct{}{}:
		defer func() { <-slots }()
	default:
		return Image{}, ErrBusy
	}
	cfg, format, e := image.DecodeConfig(bytes.NewReader(data))
	if e != nil || (format != "jpeg" && format != "png") || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 4096 || cfg.Height > 4096 || int64(cfg.Width)*int64(cfg.Height) > 8000000 {
		return Image{}, ErrInvalid
	}
	pixels, decoded, e := image.Decode(bytes.NewReader(data))
	if e != nil || decoded != format || pixels.Bounds().Dx() != cfg.Width || pixels.Bounds().Dy() != cfg.Height {
		return Image{}, ErrInvalid
	}
	if format == "jpeg" {
		pixels = orient(pixels, orientation(data))
		cfg.Width = pixels.Bounds().Dx()
		cfg.Height = pixels.Bounds().Dy()
	}
	var output bytes.Buffer
	mime := "image/png"
	if format == "jpeg" {
		mime = "image/jpeg"
		e = jpeg.Encode(&output, pixels, &jpeg.Options{Quality: 85})
	} else {
		e = png.Encode(&output, pixels)
	}
	if e != nil || output.Len() > MaxBytes {
		return Image{}, ErrInvalid
	}
	return Image{Content: output.Bytes(), MediaType: mime, Width: int32(cfg.Width), Height: int32(cfg.Height)}, nil
}
