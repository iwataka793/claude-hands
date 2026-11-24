package main

// 画面（スクリーンショットの縮小）と、画面の部品の一覧（XML を短い行にまとめる）。

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"strconv"
	"strings"
)

// shrink は画像を 1/factor に縮めて JPEG にする（読みやすさと量のつり合い）。
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

// compactSource は WDA の画面 XML を「種類 「文字」 中心(x,y) 大きさ」の行にする。
// all=false なら、文字のある見えている部品だけ。
func compactSource(src string, all bool, max int) (string, int) {
	dec := xml.NewDecoder(strings.NewReader(src))
	var lines []string
	total := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		a := map[string]string{}
		for _, at := range se.Attr {
			a[at.Name.Local] = at.Value
		}
		if a["type"] == "" {
			continue
		}
		total++
		text := a["label"]
		if text == "" {
			text = a["name"]
		}
		if v := a["value"]; v != "" && v != text {
			if text != "" {
				text += " / "
			}
			text += v
		}
		if !all && (text == "" || a["visible"] == "false") {
			continue
		}
		x, _ := strconv.Atoi(a["x"])
		y, _ := strconv.Atoi(a["y"])
		w, _ := strconv.Atoi(a["width"])
		h, _ := strconv.Atoi(a["height"])
		t := strings.TrimPrefix(a["type"], "XCUIElementType")
		lines = append(lines, fmt.Sprintf("%s 「%s」 中心(%d,%d) 大きさ%dx%d", t, text, x+w/2, y+h/2, w, h))
		if len(lines) >= max {
			lines = append(lines, "…（多いので省略）")
			break
		}
	}
	return strings.Join(lines, "\n"), total
}
