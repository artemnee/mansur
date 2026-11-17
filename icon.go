package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

// exportIcons writes the PNG set for iconutil (Mansur.iconset) into dir.
func exportIcons(dir string) error {
	if dir == "" {
		return fmt.Errorf("usage: mansur icon Mansur.iconset")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	c := &canvas{}
	drawCat(c, pose{eyes: eyeHappy, mouth: mouthSmile})

	for _, s := range []int{16, 32, 128, 256, 512} {
		for _, k := range []int{1, 2} {
			name := fmt.Sprintf("icon_%dx%d.png", s, s)
			if k == 2 {
				name = fmt.Sprintf("icon_%dx%d@2x.png", s, s)
			}
			if err := writePNG(filepath.Join(dir, name), c, s*k); err != nil {
				return err
			}
		}
	}
	return nil
}

func writePNG(path string, c *canvas, px int) error {
	img := image.NewNRGBA(image.Rect(0, 0, px, px))
	for y := range px {
		for x := range px {
			col := c[(y*ch/px)*cw+x*cw/px]
			img.Set(x, y, color.NRGBA{byte(col >> 24), byte(col >> 16), byte(col >> 8), byte(col)})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
