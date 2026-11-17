package main

// All pixel art is drawn in code on a 32×32 canvas; the window scales it up without smoothing.

const (
	cw = 32
	ch = 32
)

type rgba uint32

const (
	cNone  rgba = 0
	cLine  rgba = 0x3b2a3aff
	cWhite rgba = 0xffffffff
	cHeart rgba = 0xff5c7aff
	cZ     rgba = 0x7f9cffff
	cBerry rgba = 0xd13b62ff
	cLeaf  rgba = 0x4fae5cff
)

func lerpColor(a, b rgba, k float64) rgba {
	var out rgba
	for sh := 24; sh >= 0; sh -= 8 {
		x, y := float64(a>>sh&0xff), float64(b>>sh&0xff)
		out |= rgba(x+(y-x)*k+0.5) << sh
	}
	return out
}

type eyes int

const (
	eyeOpen eyes = iota
	eyeClosed
	eyeHappy
	eyeWide
)

type mouth int

const (
	mouthSmile mouth = iota
	mouthFlat
	mouthO
)

type feet int

const (
	feetStand feet = iota
	feetStepL
	feetStepR
	feetDangle
)

type body int

const (
	bodyStand body = iota
	bodySit
	bodyLie
)

type pose struct {
	body   body
	squash int // >0 squashed, <0 stretched
	lift   int // how high above the ground (jump)
	eyes   eyes
	mouth  mouth
	look   int // -1 left, 1 right
	feet   feet
	t      int // time, for shape animations
}

type canvas [cw * ch]rgba

func (c *canvas) set(x, y int, col rgba) {
	if x >= 0 && x < cw && y >= 0 && y < ch {
		c[y*cw+x] = col
	}
}

// sprite draws a picture from strings: character → color, '.' is empty.
func (c *canvas) sprite(x, y int, rows []string, pal map[byte]rgba) {
	for dy, row := range rows {
		for dx := range len(row) {
			if col, ok := pal[row[dx]]; ok {
				c.set(x+dx, y+dy, col)
			}
		}
	}
}

func (c *canvas) bytes() []byte {
	b := make([]byte, cw*ch*4)
	for i, col := range c {
		b[i*4] = byte(col >> 24)
		b[i*4+1] = byte(col >> 16)
		b[i*4+2] = byte(col >> 8)
		b[i*4+3] = byte(col)
	}
	return b
}

// face tells where the eyes and mouth ended up, to draw food and particles next to them.
type face struct {
	top, eyeY, mouthX, mouthY int
}

var (
	heartArt = []string{
		".X.X.",
		"XXXXX",
		"XXXXX",
		".XXX.",
		"..X..",
	}
	zArt = []string{
		"XXX",
		".X.",
		"XXX",
	}
	bangArt = []string{
		"X",
		"X",
		"X",
		".",
		"X",
	}
	berryArt = []string{
		".G.",
		"RRR",
		"RRR",
		".R.",
	}
	bubbleArt = []string{
		".LLLLLLL.",
		"LWWWGWWWL",
		"LWWRRRWWL",
		"LWWRRRWWL",
		"LWWWRWWWL",
		".LLLLLLL.",
	}
	artPal = map[byte]rgba{'X': cHeart, 'G': cLeaf, 'R': cBerry, 'L': cLine, 'W': cWhite}
)
