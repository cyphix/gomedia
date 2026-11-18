package imaging_test

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"testing"

	"github.com/cyphix/gomedia/imaging"
)

func TestDecodePixelLimit(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 40, 30))); err != nil {
		t.Fatal(err)
	}
	if _, _, err := imaging.Decode(bytes.NewReader(buf.Bytes()), 1199); !errors.Is(err, imaging.ErrTooManyPixels) {
		t.Fatalf("Decode: %v, want pixel limit error", err)
	}
	img, canvas, err := imaging.Decode(bytes.NewReader(buf.Bytes()), 1200)
	if err != nil || img.Bounds().Size() != image.Pt(40, 30) || canvas != image.Pt(40, 30) {
		t.Fatalf("Decode at limit: %v", err)
	}
}

func TestDecodeGIFWithPartialFirstFrame(t *testing.T) {
	frame := image.NewPaletted(image.Rect(10, 10, 30, 20), color.Palette{color.Black, color.White})
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, &gif.GIF{
		Image:  []*image.Paletted{frame},
		Delay:  []int{0},
		Config: image.Config{ColorModel: frame.Palette, Width: 40, Height: 30},
	}); err != nil {
		t.Fatal(err)
	}
	got, canvas, err := imaging.Decode(bytes.NewReader(buf.Bytes()), 1200)
	if err != nil || got.Bounds() != frame.Bounds() {
		t.Fatalf("decode partial GIF: %v", err)
	}
	if canvas != image.Pt(40, 30) {
		t.Fatalf("GIF canvas = %v, want (40,30)", canvas)
	}
}

func TestThumbnailFormatAndCrop(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 80, 40))
	for y := range 40 {
		for x := 40; x < 80; x++ {
			img.SetNRGBA(x, y, color.NRGBA{G: 255, A: 255})
		}
	}
	tests := []struct {
		crop   image.Rectangle
		ext    string
		format string
		size   image.Point
	}{
		{image.Rectangle{}, ".png", "png", image.Pt(20, 10)},
		{image.Rect(40, 0, 80, 40), ".jpg", "jpeg", image.Pt(20, 20)},
	}
	for _, tc := range tests {
		result, err := imaging.NewThumbnail(img, tc.crop, 20)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if err := result.Encode(&buf); err != nil {
			t.Fatal(err)
		}
		cfg, format, err := image.DecodeConfig(&buf)
		if err != nil || format != tc.format || result.Extension() != tc.ext ||
			image.Pt(cfg.Width, cfg.Height) != tc.size {
			t.Fatalf("encoded = %s %s %dx%d, %v", result.Extension(), format, cfg.Width, cfg.Height, err)
		}
	}
}
