// Package logo проверяет и нормализует логотипы студий: квадрат 512×512,
// PNG (если есть прозрачность) или JPEG. Картинка всегда перекодируется —
// в базу не попадают метаданные и посторонние данные из исходного файла.
package logo

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
)

const (
	MaxUploadBytes = 5 << 20 // исходный файл
	MinSide        = 128     // меньше — логотип будет размытым
	MaxSourceSide  = 6000    // защита от «бомб» с огромным разрешением
	OutSide        = 512
)

var (
	ErrFormat   = errors.New("logo must be PNG or JPEG")
	ErrTooSmall = errors.New("logo is too small")
	ErrTooLarge = errors.New("logo is too large")
)

// Result — готовый логотип и его версия для адреса (меняется при замене).
type Result struct {
	ContentType string
	Data        []byte
	Version     string
}

func Process(raw []byte) (*Result, error) {
	if len(raw) > MaxUploadBytes {
		return nil, ErrTooLarge
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || (format != "png" && format != "jpeg") {
		return nil, ErrFormat
	}
	if cfg.Width > MaxSourceSide || cfg.Height > MaxSourceSide {
		return nil, ErrTooLarge
	}
	if cfg.Width < MinSide || cfg.Height < MinSide {
		return nil, ErrTooSmall
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, ErrFormat
	}
	square := cropSquare(src)
	out := resize(src, square, min(OutSide, square.Dx()))

	var buf bytes.Buffer
	ct := "image/jpeg"
	if transparent(out) {
		ct = "image/png"
		err = (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, out)
	} else {
		err = jpeg.Encode(&buf, out, &jpeg.Options{Quality: 90})
	}
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(buf.Bytes())
	return &Result{ContentType: ct, Data: buf.Bytes(), Version: hex.EncodeToString(sum[:])[:12]}, nil
}

// cropSquare вырезает квадрат по центру.
func cropSquare(img image.Image) image.Rectangle {
	b := img.Bounds()
	side := min(b.Dx(), b.Dy())
	x0 := b.Min.X + (b.Dx()-side)/2
	y0 := b.Min.Y + (b.Dy()-side)/2
	return image.Rect(x0, y0, x0+side, y0+side)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// resize уменьшает квадратную область усреднением пикселей (box filter):
// каждый пиксель результата — среднее по своему квадратику исходника.
func resize(src image.Image, rect image.Rectangle, side int) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, side, side))
	n := rect.Dx()
	for y := 0; y < side; y++ {
		sy0, sy1 := rect.Min.Y+y*n/side, rect.Min.Y+(y+1)*n/side
		if sy1 == sy0 {
			sy1++
		}
		for x := 0; x < side; x++ {
			sx0, sx1 := rect.Min.X+x*n/side, rect.Min.X+(x+1)*n/side
			if sx1 == sx0 {
				sx1++
			}
			var r, g, b, a, count uint64
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					cr, cg, cb, ca := src.At(sx, sy).RGBA()
					r, g, b, a = r+uint64(cr), g+uint64(cg), b+uint64(cb), a+uint64(ca)
					count++
				}
			}
			out.SetRGBA(x, y, color.RGBA{R: uint8(r / count >> 8), G: uint8(g / count >> 8), B: uint8(b / count >> 8), A: uint8(a / count >> 8)})
		}
	}
	return out
}

func transparent(img *image.RGBA) bool {
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] < 250 {
			return true
		}
	}
	return false
}
