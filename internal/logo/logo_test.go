package logo

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func encodePNG(img image.Image) []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func TestProcessCropsResizesAndKeepsTransparency(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 1200, 800))
	for y := 0; y < 800; y++ {
		for x := 0; x < 1200; x++ {
			src.Set(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	src.Set(0, 0, color.NRGBA{}) // угол за пределами центрального квадрата
	res, err := Process(encodePNG(src))
	if err != nil {
		t.Fatal(err)
	}
	if res.ContentType != "image/jpeg" {
		t.Fatalf("opaque logo must become JPEG, got %s", res.ContentType)
	}
	img, err := jpeg.Decode(bytes.NewReader(res.Data))
	if err != nil || img.Bounds().Dx() != OutSide || img.Bounds().Dy() != OutSide {
		t.Fatalf("result: %v %v", img.Bounds(), err)
	}

	clear := image.NewNRGBA(image.Rect(0, 0, 300, 300))
	clear.Set(150, 150, color.NRGBA{B: 255, A: 255})
	res, err = Process(encodePNG(clear))
	if err != nil || res.ContentType != "image/png" || len(res.Version) != 12 {
		t.Fatalf("transparent logo: %+v %v", res, err)
	}
}

func TestProcessRejectsBadInput(t *testing.T) {
	if _, err := Process([]byte("<svg onload=alert(1)>")); err != ErrFormat {
		t.Fatalf("svg/html must be rejected, got %v", err)
	}
	if _, err := Process(encodePNG(image.NewNRGBA(image.Rect(0, 0, 64, 64)))); err != ErrTooSmall {
		t.Fatalf("tiny logo, got %v", err)
	}
}
