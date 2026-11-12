package chatattach

import (
	"bytes"
	"image"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
)

// maxScalePixels bounds the images decoded to scale them: an RGBA copy
// takes four bytes a pixel.
const maxScalePixels = 40_000_000

// jpegQualities are tried in turn until a scaled photo fits MaxImageBytes.
var jpegQualities = []int{85, 70, 55}

// fitImage returns an image within MaxImageBytes and MaxImageEdge: as it
// is, or scaled to fit and encoded again, as PNG when that fits and as JPEG
// otherwise.
func fitImage(name, mimeType string, b []byte) ([]byte, string, error) {
	if mimeType == "image/webp" {
		// The standard library has no WebP decoder: send it as it is or not
		// at all.
		if len(b) > MaxImageBytes {
			return nil, "", refuse(name, "is a WebP image larger than %s; save it as PNG or JPEG to attach it scaled down", sizeText(MaxImageBytes))
		}
		return b, mimeType, nil
	}
	cfg, err := decodeConfig(mimeType, b)
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, "", refuse(name, "could not be read as an image")
	}
	if len(b) <= MaxImageBytes && max(cfg.Width, cfg.Height) <= MaxImageEdge {
		return b, mimeType, nil
	}
	if cfg.Width*cfg.Height > maxScalePixels {
		return nil, "", refuse(name, "is too large to scale down (%d×%d pixels)", cfg.Width, cfg.Height)
	}
	src, err := decode(mimeType, b)
	if err != nil {
		return nil, "", refuse(name, "could not be read as an image")
	}
	w, h := fitSize(cfg.Width, cfg.Height, MaxImageEdge)
	scaled := scaleDown(src, w, h)
	if mimeType != "image/jpeg" {
		var out bytes.Buffer
		if err := png.Encode(&out, scaled); err == nil && out.Len() <= MaxImageBytes {
			return out.Bytes(), "image/png", nil
		}
	}
	flat := flatten(scaled)
	for _, q := range jpegQualities {
		var out bytes.Buffer
		if err := jpeg.Encode(&out, flat, &jpeg.Options{Quality: q}); err != nil {
			break
		}
		if out.Len() <= MaxImageBytes {
			return out.Bytes(), "image/jpeg", nil
		}
	}
	return nil, "", refuse(name, "is still larger than %s once scaled down", sizeText(MaxImageBytes))
}

func decodeConfig(mimeType string, b []byte) (image.Config, error) {
	r := bytes.NewReader(b)
	switch mimeType {
	case "image/png":
		return png.DecodeConfig(r)
	case "image/jpeg":
		return jpeg.DecodeConfig(r)
	default:
		return gif.DecodeConfig(r)
	}
}

// decode decodes the image; a GIF's first frame.
func decode(mimeType string, b []byte) (image.Image, error) {
	r := bytes.NewReader(b)
	switch mimeType {
	case "image/png":
		return png.Decode(r)
	case "image/jpeg":
		return jpeg.Decode(r)
	default:
		return gif.Decode(r)
	}
}

// fitSize scales w×h to fit edge on its long side, keeping its aspect ratio;
// a size that fits stays as it is.
func fitSize(w, h, edge int) (int, int) {
	long := max(w, h)
	if long <= edge {
		return w, h
	}
	sw := max(1, (w*edge+long/2)/long)
	sh := max(1, (h*edge+long/2)/long)
	return sw, sh
}

// scaleDown scales src to w×h, no larger than src, each pixel the average
// of the source pixels it covers.
func scaleDown(src image.Image, w, h int) *image.RGBA {
	b := src.Bounds()
	rgba, ok := src.(*image.RGBA)
	if !ok || rgba.Rect.Min != (image.Point{}) {
		rgba = image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		draw.Draw(rgba, rgba.Rect, src, b.Min, draw.Src)
	}
	sw, sh := rgba.Rect.Dx(), rgba.Rect.Dy()
	if w == sw && h == sh {
		return rgba
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		y0, y1 := y*sh/h, max((y+1)*sh/h, y*sh/h+1)
		for x := range w {
			x0, x1 := x*sw/w, max((x+1)*sw/w, x*sw/w+1)
			var r, g, bl, a uint64
			for sy := y0; sy < y1; sy++ {
				row := rgba.Pix[sy*rgba.Stride+x0*4 : sy*rgba.Stride+x1*4]
				for i := 0; i < len(row); i += 4 {
					r += uint64(row[i])
					g += uint64(row[i+1])
					bl += uint64(row[i+2])
					a += uint64(row[i+3])
				}
			}
			n := uint64((y1 - y0) * (x1 - x0))
			o := dst.PixOffset(x, y)
			dst.Pix[o] = uint8((r + n/2) / n)
			dst.Pix[o+1] = uint8((g + n/2) / n)
			dst.Pix[o+2] = uint8((bl + n/2) / n)
			dst.Pix[o+3] = uint8((a + n/2) / n)
		}
	}
	return dst
}

// flatten puts img on white, for JPEG, which has no transparency.
func flatten(img *image.RGBA) *image.RGBA {
	out := image.NewRGBA(img.Rect)
	for i := 0; i < len(img.Pix); i += 4 {
		// Premultiplied: over white adds the uncovered share of white.
		back := 255 - img.Pix[i+3]
		out.Pix[i] = img.Pix[i] + back
		out.Pix[i+1] = img.Pix[i+1] + back
		out.Pix[i+2] = img.Pix[i+2] + back
		out.Pix[i+3] = 255
	}
	return out
}
