package main

// Renders the README images into docs/ from the same code that draws Mansur:
//
//	MANSUR_ASSETS=docs go test -run TestReadmeAssets .

import (
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestReadmeAssets(t *testing.T) {
	dir := os.Getenv("MANSUR_ASSETS")
	if dir == "" {
		t.Skip("set MANSUR_ASSETS=docs to render the README images")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	save := func(name string, pm *pixmap, scale int) {
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, upscale(pm, scale)); err != nil {
			t.Fatal(err)
		}
	}
	save("banner.png", banner(), 6)
	save("poses.png", poses(), 5)
	save("menu.png", menu(), 4)
	save("chat.png", chat(), 4)
	if err := demoGIF(filepath.Join(dir, "demo.gif")); err != nil {
		t.Fatal(err)
	}
}

// card is a dark rounded panel in the app's theme.
func card(w, h int) *pixmap {
	pm := newPixmap(w, h)
	pm.box(0, 0, w, h, uiBg, uiBorder)
	return pm
}

func blit(pm *pixmap, c *canvas, x0, y0 int) {
	for i, col := range c {
		if col&0xff != 0 {
			pm.set(x0+i%cw, y0+i/cw, col)
		}
	}
}

// bigText draws text with every font pixel doubled.
func bigText(pm *pixmap, x, y int, s string, col rgba) {
	tmp := newPixmap(textW(s), glyphH)
	tmp.text(0, 0, s, col)
	for i, c := range tmp.px {
		if c != 0 {
			pm.fill(x+(i%tmp.w)*2, y+(i/tmp.w)*2, 2, 2, c)
		}
	}
}

func sparkles(pm *pixmap, pts [][2]int) {
	for _, p := range pts {
		pm.set(p[0], p[1], catHorn)
		pm.set(p[0]-1, p[1], catShade)
		pm.set(p[0]+1, p[1], catShade)
		pm.set(p[0], p[1]-1, catShade)
		pm.set(p[0], p[1]+1, catShade)
	}
}

// bubble draws a speech bubble with its tail pointing down at tailX.
func bubble(pm *pixmap, x, y int, s string) {
	w, h := textW(s)+10, lineH-2+8
	pm.box(x, y, w, h, uiBg, uiBorder)
	pm.text(x+5, y+4, s, uiText)
	cx := x + w/2 - 2
	pm.fill(cx+1, y+h-1, 2, 1, uiBg)
	pm.set(cx, y+h, uiBorder)
	pm.fill(cx+1, y+h, 2, 1, uiBg)
	pm.set(cx+3, y+h, uiBorder)
	pm.set(cx+1, y+h+1, uiBorder)
	pm.set(cx+2, y+h+1, uiBg)
	pm.set(cx+3, y+h+1, uiBorder)
	pm.fill(cx+2, y+h+2, 2, 1, uiBorder)
}

func banner() *pixmap {
	pm := card(232, 52)
	bigText(pm, 12, 11, "MANSUR", catBody)
	pm.text(12, 31, "a tiny gpt cat for macos", uiText)
	pm.text(12, 41, "pure go, offline, 490 kb brain", uiGrey)
	sparkles(pm, [][2]int{{98, 12}, {170, 40}, {222, 30}})
	p := petAt(stIdle, 30)
	blit(pm, p.render(), 186, 17)
	bubble(pm, 182, 4, "hi!")
	return pm
}

// petAt returns a pet that has spent ticks in state s, so particles and animations are in motion.
func petAt(s state, ticks int) *Pet {
	area := NSRect{Size: NSSize{W: 4000, H: 4000}}
	p := newPet(1, area)
	p.blinkIn = 1 << 30
	switch s {
	case stJump:
		p.jump()
	case stSleep, stEat:
		p.set(s)
	case stHappy:
		p.set(s)
		p.happyT = 1 << 20
	case stWalk:
		p.set(s)
		p.stDur = 1 << 20
	}
	for range ticks {
		p.update(NSPoint{X: -1e6, Y: -1e6}, area)
		if s == stJump && p.st != stJump {
			break
		}
	}
	return p
}

func poses() *pixmap {
	pets := []struct {
		label string
		p     *Pet
	}{
		{"hi", petAt(stIdle, 30)},
		{"pet me", petAt(stHappy, 22)},
		{"jump", petAt(stJump, 4)},
		{"snack", petAt(stEat, 10)},
		{"sleep", petAt(stSleep, 50)},
	}
	pm := card(len(pets)*40+8, 52)
	for i, it := range pets {
		x := 8 + i*40
		blit(pm, it.p.render(), x, 6)
		pm.text(x+16-textW(it.label)/2, 41, it.label, uiGrey)
	}
	return pm
}

func panel(title string, lines []string, hover int) *pixmap {
	tw := textW(title)
	for _, l := range lines {
		tw = max(tw, textW(l))
	}
	w, h := tw+18, pad*2+len(lines)*itemH
	pm := newPixmap(w, h)
	pm.box(0, 0, w, h, uiBg, uiBorder)
	for i, l := range lines {
		y := pad + i*itemH
		col := uiText
		if i < 2 {
			col = uiGrey
		} else if i == hover {
			pm.fill(2, y, w-4, itemH, uiHover)
			pm.text(5, y+2, ">", uiText)
		}
		pm.text(12, y+2, l, col)
	}
	return pm
}

func menu() *pixmap {
	en := func(k string) string { return trIn("en", k) }
	m := panel("", []string{en("full") + "████░", fmt.Sprintf(en("age"), 3), en("talk"), en("feed"),
		en("sleep"), en("breaks") + en("on"), en("autostart") + en("off"), en("language") + en("auto"), en("quit")}, 2)
	pm := newPixmap(m.w+44, m.h+4)
	for i, c := range m.px {
		pm.set(i%m.w, 2+i/m.w, c)
	}
	blit(pm, petAt(stIdle, 10).render(), m.w+8, pm.h-32)
	return pm
}

func chat() *pixmap {
	w := textW(trIn("en", "hint")) + 12
	in := newPixmap(w, pad*2+lineH*3+6)
	in.box(0, 0, w, in.h, uiBg, uiBorder)
	in.text(6, pad+1, trIn("en", "write"), uiGrey)
	in.text(6, pad+lineH+4, "what day is it?_", uiText)
	in.text(6, in.h-pad-lineH+1, trIn("en", "hint"), uiGrey)

	say := "today is tuesday"
	bw := textW(say) + 10
	pm := newPixmap(w+bw+10, 60)
	for i, c := range in.px {
		pm.set(i%w, pm.h-in.h+i/w, c)
	}
	bubble(pm, w+6, 2, say)
	blit(pm, petAt(stIdle, 10).render(), w+6+bw/2-16, 28)
	return pm
}

// demoGIF animates a short day in Mansur's life.
func demoGIF(path string) error {
	type scene struct {
		s     state
		ticks int
		say   string
	}
	scenes := []scene{
		{stIdle, 40, "hi!"},
		{stWalk, 30, ""},
		{stHappy, 40, "purr ♥"},
		{stJump, 12, "hop!"},
		{stEat, 60, "nom nom!"},
		{stSleep, 60, ""},
	}
	const W, H, scale = 104, 64, 4
	anim := &gif.GIF{}
	for _, sc := range scenes {
		area := NSRect{Size: NSSize{W: 4000, H: 4000}}
		p := newPet(1, area)
		switch sc.s {
		case stJump:
			p.jump()
		case stHappy:
			p.set(sc.s)
			p.happyT = 1 << 20
		case stWalk:
			p.set(sc.s)
			p.stDur = 1 << 20
			p.dir = 1
		default:
			p.set(sc.s)
		}
		for i := range sc.ticks {
			p.update(NSPoint{X: -1e6, Y: -1e6}, area)
			if sc.s == stJump && p.st != stJump {
				p.jump()
			}
			if i%2 == 1 {
				continue // 10 fps keeps the file small
			}
			pm := card(W, H)
			blit(pm, p.render(), W/2-16, H-34)
			if sc.say != "" {
				bubble(pm, W/2-(textW(sc.say)+10)/2, 4, sc.say)
			}
			anim.Image = append(anim.Image, paletted(upscale(pm, scale)))
			anim.Delay = append(anim.Delay, 10)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return gif.EncodeAll(f, anim)
}

func upscale(pm *pixmap, s int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, pm.w*s, pm.h*s))
	for i, c := range pm.px {
		if c&0xff == 0 {
			continue
		}
		col := color.NRGBA{byte(c >> 24), byte(c >> 16), byte(c >> 8), byte(c)}
		x, y := (i%pm.w)*s, (i/pm.w)*s
		for dy := range s {
			for dx := range s {
				img.SetNRGBA(x+dx, y+dy, col)
			}
		}
	}
	return img
}

func paletted(img *image.NRGBA) *image.Paletted {
	pal := color.Palette{color.NRGBA{}}
	idx := map[color.NRGBA]uint8{{}: 0}
	out := image.NewPaletted(img.Rect, nil)
	for i := 0; i < len(img.Pix); i += 4 {
		c := color.NRGBA{img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3]}
		if c.A == 0 {
			c = color.NRGBA{}
		}
		k, ok := idx[c]
		if !ok {
			k = uint8(len(pal))
			idx[c] = k
			pal = append(pal, c)
		}
		out.Pix[i/4] = k
	}
	out.Palette = pal
	return out
}
