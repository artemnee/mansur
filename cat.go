package main

import "math"

// Mansur is a fluffy alien cat: instead of ears he has two antenna stalks with little bulbs.

const (
	catLine  rgba = 0x1d2a12ff // outline: dark olive, not black
	catDark  rgba = 0x3d5a20ff
	catShade rgba = 0x6c9a38ff
	catBody  rgba = 0x9cc75aff
	catLight rgba = 0xc4e48aff
	catMask  rgba = 0xeef7d8ff // light muzzle and chest
	catTip   rgba = 0xf6ffe6ff
	catHorn  rgba = 0xdaf7a0ff // antenna bulbs
	catEye   rgba = 0x1d2a12ff
	catIris  rgba = 0xf2d36bff
	catNose  rgba = 0xff86b0ff
	catCheek rgba = 0xff9fb8ff
	catMouth rgba = 0x1d2a12ff
)

// The head is 14×9 and faces the viewer: stripes on the forehead, a light muzzle around the nose.
var (
	headArt = []string{
		"..LLLSBBSBBB..",
		".LLLLSBBSBBBB.",
		"LLLLBBBBBBBBBS",
		"LBBBBBBBBBBBBS",
		"BBBBBBBBBBBBBS",
		"BBBBMMMMMMBBBS",
		"BBBMMMMMMMMBSS",
		".BBMMMMMMMMBS.",
		"..SSBBBBBBSS..",
	}
	headPal = map[byte]rgba{'L': catLight, 'B': catBody, 'S': catShade, 'M': catMask}
)

// shape is a colored fill; the outline is built around all filled pixels.
type shape [cw * ch]rgba

func (s *shape) set(x, y int, c rgba) {
	if x >= 0 && x < cw && y >= 0 && y < ch {
		s[y*cw+x] = c
	}
}

func (s *shape) at(x, y int) rgba {
	if x < 0 || x >= cw || y < 0 || y >= ch {
		return 0
	}
	return s[y*cw+x]
}

func (s *shape) paint(c *canvas, line rgba) {
	for y := range ch {
		for x := range cw {
			switch {
			case s.at(x, y) != 0:
				c.set(x, y, s.at(x, y))
			case s.at(x-1, y) != 0 || s.at(x+1, y) != 0 || s.at(x, y-1) != 0 || s.at(x, y+1) != 0:
				c.set(x, y, line)
			}
		}
	}
}

// shadeAt colors a body pixel by position: lighter at the top left, darker at the bottom right.
func shadeAt(nx, ny float64) rgba {
	switch d := nx*0.4 + ny; {
	case d < -0.55:
		return catLight
	case d > 0.75:
		return catDark
	case d > 0.35:
		return catShade
	}
	return catBody
}

// blob fills a shaded ellipse.
func (s *shape) blob(cx, cy, rx, ry float64) {
	for y := range ch {
		for x := range cw {
			nx, ny := (float64(x)+0.5-cx)/rx, (float64(y)+0.5-cy)/ry
			if nx*nx+ny*ny <= 1 {
				s.set(x, y, shadeAt(nx, ny))
			}
		}
	}
}

func (s *shape) fill(cx, cy, rx, ry float64, col rgba) {
	for y := range ch {
		for x := range cw {
			nx, ny := (float64(x)+0.5-cx)/rx, (float64(y)+0.5-cy)/ry
			if nx*nx+ny*ny <= 1 {
				s.set(x, y, col)
			}
		}
	}
}

// tail draws the tail as circles along a path.
func (s *shape) tail(path [][2]float64, r float64) {
	for _, pt := range path {
		for y := range ch {
			for x := range cw {
				dx, dy := float64(x)+0.5-pt[0], float64(y)+0.5-pt[1]
				if dx*dx+dy*dy > r*r {
					continue
				}
				col := catShade
				if dx < -0.5 && dy < 0 {
					col = catBody
				}
				s.set(x, y, col)
			}
		}
	}
}

func (s *shape) leg(x0, top, bot int, col rgba) {
	for y := top; y <= bot; y++ {
		for x := x0; x < x0+2; x++ {
			if y == bot {
				s.set(x, y, catDark)
			} else {
				s.set(x, y, col)
			}
		}
	}
}

func drawCat(c *canvas, p pose) face {
	var s shape
	oy := 1 - p.lift // one pixel lower so the antenna bulbs fit at the top
	sq := float64(p.squash)

	var hx, hy int
	switch p.body {
	case bodySit:
		hx, hy = 5, 14+oy
		drawSit(&s, p, oy, sq)
	case bodyLie:
		hx, hy = 1, 19+oy
		drawLie(&s, oy, sq)
	default:
		hx, hy = 1, 14+oy
		drawStand(&s, p, oy, sq)
	}
	if p.squash > 0 {
		hy++
	}

	for dy, row := range headArt {
		for dx := range len(row) {
			if col, ok := headPal[row[dx]]; ok {
				s.set(hx+dx, hy+dy, col)
			}
		}
	}
	// Fluffy cheeks.
	for _, t := range [][2]int{{-1, 5}, {-1, 6}, {14, 5}, {14, 6}} {
		s.set(hx+t[0], hy+t[1], catShade)
	}
	drawHorns(&s, p, hx, hy)

	s.paint(c, catLine)
	f := drawFace(c, p, hx, hy)
	if p.paw > 0 { // licking a paw: a small paw in front of the muzzle
		var paw shape
		x0, y0 := hx+3, hy+8-p.paw
		for dy := range 2 {
			for dx := range 3 {
				paw.set(x0+dx, y0+dy, catBody)
			}
		}
		paw.set(x0, y0+1, catMask)
		paw.set(x0+2, y0+1, catMask)
		paw.paint(c, catLine)
	}

	if p.look > 0 { // looking right: mirror the sprite
		for y := range ch {
			row := c[y*cw : (y+1)*cw]
			for i, j := 0, cw-1; i < j; i, j = i+1, j-1 {
				row[i], row[j] = row[j], row[i]
			}
		}
		f.mouthX = cw - 1 - f.mouthX
	}
	return f
}

// drawStand: standing sideways, head turned to the viewer, tail up.
func drawStand(s *shape, p pose, oy int, sq float64) {
	legs := func(xs []int, col rgba, pair int) {
		for _, x0 := range xs {
			top, bot := 27+oy, 29+oy
			switch {
			case p.feet == feetDangle:
				bot = 30 + oy
			case p.feet == feetStepL && pair == 0, p.feet == feetStepR && pair == 1:
				bot--
			}
			s.leg(x0, top, bot, col)
		}
	}
	legs([]int{11, 20}, catDark, 1)

	sway := math.Sin(float64(p.t)/8) * 1.3
	path := [][2]float64{{22, 23}, {23.5, 21.5}, {24.5, 19.5}, {25, 17.5}, {25 + sway*0.5, 15.5}, {24.5 + sway, 14}}
	for i := range path {
		path[i][1] += float64(oy)
	}
	s.tail(path, 1.2)

	s.blob(15, 23.5+float64(oy)+sq/2, 7+sq, 4-sq/2)
	legs([]int{8, 17}, catShade, 0)
	s.fill(8, 24.5+float64(oy), 1.8, 2.2, catMask)
}

// drawSit: sitting, hind leg to the side, front paws together, tail curling up.
func drawSit(s *shape, p pose, oy int, sq float64) {
	o := float64(oy)
	sway := math.Sin(float64(p.t)/8) * 1.3
	s.tail([][2]float64{{18, 29 + o}, {20, 29 + o}, {21.5, 28 + o}, {22.5, 26.5 + o}, {23, 24.5 + o}, {23 + sway*0.4, 22.5 + o}}, 1.2)
	s.blob(15.5, 27+o, 3.5, 3)
	s.blob(11.5, 25+o+sq/2, 5+sq/2, 4.5-sq/2)
	s.fill(11.5, 24.5+o, 1.5, 2, catMask)
	s.leg(9, 27+oy, 29+oy, catBody)
	s.leg(13, 27+oy, 29+oy, catBody)
}

// drawLie: lying like a loaf, paws tucked in, tail hidden, head down.
func drawLie(s *shape, oy int, sq float64) {
	s.blob(14, 26+float64(oy)+sq/2, 8+sq/2, 4.5-sq/2)
	for _, x := range []int{3, 4, 5, 8, 9, 10} {
		s.set(x, 29+oy, catBody)
	}
}

// drawHorns: antenna stalks grow where a cat's ears would be. They sway, droop in sleep and stand up when startled.
func drawHorns(s *shape, p pose, hx, hy int) {
	for _, side := range []int{-1, 1} {
		bx := hx + 3
		if side > 0 {
			bx = hx + 10
		}
		length, lean := 5, 0.3
		sw := math.Sin(float64(p.t)/6+float64(side)) * 0.8
		switch p.eyes {
		case eyeWide:
			length, lean, sw = 6, 0.1, 0
		case eyeClosed:
			length, lean = 4, 0.9
			sw *= 0.3
		}
		x, y := bx, hy-1
		for k := range length {
			f := float64(k+1) / float64(length)
			x = bx + side*int(math.Round(float64(k)*lean)) + int(math.Round(sw*f*f))
			y = hy - 1 - k
			if p.eyes == eyeClosed && k >= length-2 {
				y += 2 * (k - length + 3) // the tips droop
			}
			s.set(x, y, catBody)
		}
		for dy := 1; dy <= 2; dy++ {
			s.set(x, y-dy, catHorn)
			s.set(x+side, y-dy, catHorn)
		}
	}
}

// drawFace: yellow eyes with pupils, a pink nose, an "ω" mouth.
func drawFace(c *canvas, p pose, hx, hy int) face {
	ey := hy + 3
	eye := func(x int) {
		switch p.eyes {
		case eyeOpen:
			c.set(x, ey, catEye)
			c.set(x, ey+1, catEye)
			c.set(x+1, ey, catIris)
			c.set(x+1, ey+1, catIris)
		case eyeClosed:
			c.set(x, ey+1, catEye)
			c.set(x+1, ey+1, catEye)
		case eyeHappy:
			c.set(x-1, ey+1, catEye)
			c.set(x, ey, catEye)
			c.set(x+1, ey, catEye)
			c.set(x+2, ey+1, catEye)
		case eyeWide:
			for dy := -1; dy <= 1; dy++ {
				for dx := range 2 {
					c.set(x+dx, ey+dy, catEye)
				}
			}
			c.set(x, ey-1, catTip)
		}
	}
	eye(hx + 3)
	eye(hx + 9)
	c.set(hx+2, hy+6, catCheek)
	c.set(hx+11, hy+6, catCheek)
	c.set(hx+6, hy+5, catNose)
	c.set(hx+7, hy+5, catNose)

	my := hy + 6
	var dots [][2]int
	switch p.mouth {
	case mouthSmile:
		dots = [][2]int{{4, 0}, {5, 1}, {6, 0}, {7, 0}, {8, 1}, {9, 0}}
	case mouthFlat:
		dots = [][2]int{{5, 1}, {6, 1}, {7, 1}, {8, 1}}
	case mouthO:
		dots = [][2]int{{6, 0}, {7, 0}, {6, 1}, {7, 1}}
	}
	for _, d := range dots {
		c.set(hx+d[0], my+d[1], catMouth)
	}
	return face{top: hy - 7, eyeY: ey, mouthX: hx + 6, mouthY: my}
}
