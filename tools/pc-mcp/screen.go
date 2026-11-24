package main

// 画面の縮小（PNG → 1/factor の JPEG）

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
)

func shrink(png []byte, factor int, quality int) ([]byte, int, int, error) {
	src, _, err := image.Decode(bytes.NewReader(png))
	if err != nil {
		return nil, 0, 0, err
	}
	b := src.Bounds()
	if factor < 1 {
		factor = 1
	}
	w, h := b.Dx()/factor, b.Dy()/factor
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var r, g, bl, a uint32
			for dy := 0; dy < factor; dy++ {
				for dx := 0; dx < factor; dx++ {
					cr, cg, cb, ca := src.At(b.Min.X+x*factor+dx, b.Min.Y+y*factor+dy).RGBA()
					r, g, bl, a = r+cr, g+cg, bl+cb, a+ca
				}
			}
			n := uint32(factor * factor)
			dst.Set(x, y, color.RGBA64{uint16(r / n), uint16(g / n), uint16(bl / n), uint16(a / n)})
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: quality}); err != nil {
		return nil, 0, 0, err
	}
	return out.Bytes(), w, h, nil
}
