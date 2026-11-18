// Package imaging probes images and generates bounded JPEG and PNG thumbnails.
package imaging

import (
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"io"

	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// ErrTooManyPixels means the image exceeds the decode budget.
var ErrTooManyPixels = errors.New("image exceeds pixel limit")

// Dimensions reads an image header and enforces a positive decoded pixel budget.
func Dimensions(src io.Reader, maxPixels int64) (image.Point, error) {
	if maxPixels <= 0 {
		return image.Point{}, errors.New("a positive pixel limit is required")
	}
	cfg, _, err := image.DecodeConfig(src)
	if err != nil {
		return image.Point{}, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return image.Point{}, errors.New("invalid image dimensions")
	}
	if int64(cfg.Width) > maxPixels/int64(cfg.Height) {
		return image.Point{}, fmt.Errorf("%w: %dx%d", ErrTooManyPixels, cfg.Width, cfg.Height)
	}
	return image.Pt(cfg.Width, cfg.Height), nil
}

// Decode checks dimensions before allocating decoded image pixels.
func Decode(src io.ReadSeeker, maxPixels int64) (image.Image, error) {
	offset, err := src.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	if _, err := Dimensions(src, maxPixels); err != nil {
		return nil, err
	}
	if _, err := src.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(src)
	if err != nil {
		return nil, err
	}
	size := img.Bounds().Size()
	if size.X <= 0 || size.Y <= 0 {
		return nil, errors.New("invalid decoded image dimensions")
	}
	if int64(size.X) > maxPixels/int64(size.Y) {
		return nil, ErrTooManyPixels
	}
	return img, nil
}

// Thumbnail holds the resized pixels and selects PNG when they have transparency.
type Thumbnail struct {
	img image.Image
	ext string
}

// NewThumbnail fits the selected image bounds into maxDim without upscaling.
// An empty selection means the full image.
func NewThumbnail(img image.Image, bounds image.Rectangle, maxDim int) (*Thumbnail, error) {
	if maxDim <= 0 {
		return nil, errors.New("a positive thumbnail size is required")
	}
	if bounds.Empty() {
		bounds = img.Bounds()
	}
	if bounds.Empty() || !bounds.In(img.Bounds()) {
		return nil, errors.New("invalid thumbnail bounds")
	}
	w, h := bounds.Dx(), bounds.Dy()
	nw, nh := w, h
	if w > maxDim || h > maxDim {
		if w >= h {
			nw, nh = maxDim, max(1, h*maxDim/w)
		} else {
			nw, nh = max(1, w*maxDim/h), maxDim
		}
	}
	if bounds != img.Bounds() || nw != w || nh != h {
		dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
		if nw == w && nh == h {
			draw.Draw(dst, dst.Bounds(), img, bounds.Min, draw.Src)
		} else {
			draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, bounds, draw.Src, nil)
		}
		img = dst
	}

	ext := ".png"
	if opaque(img) {
		ext = ".jpg"
	}
	return &Thumbnail{img: img, ext: ext}, nil
}

// Extension returns the encode format's filename extension, including the dot.
func (t *Thumbnail) Extension() string { return t.ext }

// Encode writes the thumbnail without copying source-file metadata.
func (t *Thumbnail) Encode(dst io.Writer) error {
	if t.ext == ".png" {
		return png.Encode(dst, t.img)
	}
	return jpeg.Encode(dst, t.img, &jpeg.Options{Quality: 85})
}

func opaque(img image.Image) bool {
	if o, ok := img.(interface{ Opaque() bool }); ok {
		return o.Opaque()
	}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a < 0xffff {
				return false
			}
		}
	}
	return true
}
