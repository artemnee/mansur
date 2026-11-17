package main

import "unicode"

// pixmap is an image of any size, for speech bubbles and menus.
type pixmap struct {
	w, h int
	px   []rgba
}

func newPixmap(w, h int) *pixmap { return &pixmap{w, h, make([]rgba, w*h)} }

func (p *pixmap) set(x, y int, c rgba) {
	if x >= 0 && x < p.w && y >= 0 && y < p.h {
		p.px[y*p.w+x] = c
	}
}

func (p *pixmap) fill(x, y, w, h int, c rgba) {
	for j := y; j < y+h; j++ {
		for i := x; i < x+w; i++ {
			p.set(i, j, c)
		}
	}
}

// box draws a rectangle with cut corners and a border, like in old RPGs.
func (p *pixmap) box(x, y, w, h int, bg, line rgba) {
	p.fill(x+1, y+1, w-2, h-2, bg)
	p.fill(x+2, y, w-4, 1, line)
	p.fill(x+2, y+h-1, w-4, 1, line)
	p.fill(x, y+2, 1, h-4, line)
	p.fill(x+w-1, y+2, 1, h-4, line)
	p.set(x+1, y+1, line)
	p.set(x+w-2, y+1, line)
	p.set(x+1, y+h-2, line)
	p.set(x+w-2, y+h-2, line)
}

func (p *pixmap) bytes() []byte {
	b := make([]byte, len(p.px)*4)
	for i, c := range p.px {
		b[i*4], b[i*4+1], b[i*4+2], b[i*4+3] = byte(c>>24), byte(c>>16), byte(c>>8), byte(c)
	}
	return b
}

const (
	glyphH = 7
	lineH  = 9
)

func glyph(r rune) []string {
	if g, ok := font[unicode.ToUpper(r)]; ok {
		return g
	}
	if r == 'ё' || r == 'Ё' {
		return font['Е']
	}
	return font['?']
}

func glyphW(r rune) int {
	if r == ' ' {
		return 3
	}
	return len(glyph(r)[0])
}

func textW(s string) int {
	w := 0
	for _, r := range s {
		w += glyphW(r) + 1
	}
	return max(w-1, 0)
}

func (p *pixmap) text(x, y int, s string, c rgba) {
	for _, r := range s {
		if r != ' ' {
			col := c
			if r == '♥' {
				col = cHeart
			}
			for dy, row := range glyph(r) {
				for dx := range len(row) {
					if row[dx] == 'X' {
						p.set(x+dx, y+dy, col)
					}
				}
			}
		}
		x += glyphW(r) + 1
	}
}

// wrap splits text into lines no wider than maxW font pixels.
func wrap(s string, maxW int) []string {
	var lines []string
	line := ""
	word := ""
	flush := func() {
		switch {
		case line == "":
			line = word
		case textW(line+" "+word) <= maxW:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
		word = ""
	}
	for _, r := range s {
		if r == ' ' {
			flush()
			continue
		}
		word += string(r)
		if textW(word) > maxW {
			flush()
			lines = append(lines, line)
			line = ""
		}
	}
	if word != "" || line == "" {
		flush()
	}
	return append(lines, line)
}

var font = map[rune][]string{
	'А': {".XXX.", "X...X", "X...X", "XXXXX", "X...X", "X...X", "X...X"},
	'Б': {"XXXXX", "X....", "X....", "XXXX.", "X...X", "X...X", "XXXX."},
	'В': {"XXXX.", "X...X", "X...X", "XXXX.", "X...X", "X...X", "XXXX."},
	'Г': {"XXXXX", "X....", "X....", "X....", "X....", "X....", "X...."},
	'Д': {"..XX.", ".X.X.", ".X.X.", ".X.X.", ".X.X.", "XXXXX", "X...X"},
	'Е': {"XXXXX", "X....", "X....", "XXXX.", "X....", "X....", "XXXXX"},
	'Ж': {"X.X.X", "X.X.X", ".XXX.", "..X..", ".XXX.", "X.X.X", "X.X.X"},
	'З': {".XXX.", "X...X", "....X", "..XX.", "....X", "X...X", ".XXX."},
	'И': {"X...X", "X...X", "X..XX", "X.X.X", "XX..X", "X...X", "X...X"},
	'Й': {".X.X.", "..X..", "X...X", "X..XX", "X.X.X", "XX..X", "X...X"},
	'К': {"X...X", "X..X.", "X.X..", "XX...", "X.X..", "X..X.", "X...X"},
	'Л': {"..XXX", ".X..X", ".X..X", ".X..X", ".X..X", "X...X", "X...X"},
	'М': {"X...X", "XX.XX", "X.X.X", "X.X.X", "X...X", "X...X", "X...X"},
	'Н': {"X...X", "X...X", "X...X", "XXXXX", "X...X", "X...X", "X...X"},
	'О': {".XXX.", "X...X", "X...X", "X...X", "X...X", "X...X", ".XXX."},
	'П': {"XXXXX", "X...X", "X...X", "X...X", "X...X", "X...X", "X...X"},
	'Р': {"XXXX.", "X...X", "X...X", "XXXX.", "X....", "X....", "X...."},
	'С': {".XXX.", "X...X", "X....", "X....", "X....", "X...X", ".XXX."},
	'Т': {"XXXXX", "..X..", "..X..", "..X..", "..X..", "..X..", "..X.."},
	'У': {"X...X", "X...X", "X...X", ".XXXX", "....X", "X...X", ".XXX."},
	'Ф': {"..X..", ".XXX.", "X.X.X", "X.X.X", "X.X.X", ".XXX.", "..X.."},
	'Х': {"X...X", "X...X", ".X.X.", "..X..", ".X.X.", "X...X", "X...X"},
	'Ц': {"X..X.", "X..X.", "X..X.", "X..X.", "X..X.", "XXXXX", "....X"},
	'Ч': {"X...X", "X...X", "X...X", ".XXXX", "....X", "....X", "....X"},
	'Ш': {"X.X.X", "X.X.X", "X.X.X", "X.X.X", "X.X.X", "X.X.X", "XXXXX"},
	'Щ': {"X.X.X", "X.X.X", "X.X.X", "X.X.X", "X.X.X", "XXXXX", "....X"},
	'Ъ': {"XX...", ".X...", ".X...", ".XXX.", ".X..X", ".X..X", ".XXX."},
	'Ы': {"X...X", "X...X", "X...X", "XX..X", "X.X.X", "X.X.X", "XX..X"},
	'Ь': {"X....", "X....", "X....", "XXXX.", "X...X", "X...X", "XXXX."},
	'Э': {".XXX.", "X...X", "....X", "..XXX", "....X", "X...X", ".XXX."},
	'Ю': {"X..X.", "X.X.X", "X.X.X", "XXX.X", "X.X.X", "X.X.X", "X..X."},
	'Я': {".XXXX", "X...X", "X...X", ".XXXX", "..X.X", ".X..X", "X...X"},

	'A': {".XXX.", "X...X", "X...X", "XXXXX", "X...X", "X...X", "X...X"},
	'B': {"XXXX.", "X...X", "X...X", "XXXX.", "X...X", "X...X", "XXXX."},
	'C': {".XXX.", "X...X", "X....", "X....", "X....", "X...X", ".XXX."},
	'D': {"XXXX.", "X...X", "X...X", "X...X", "X...X", "X...X", "XXXX."},
	'E': {"XXXXX", "X....", "X....", "XXXX.", "X....", "X....", "XXXXX"},
	'F': {"XXXXX", "X....", "X....", "XXXX.", "X....", "X....", "X...."},
	'G': {".XXX.", "X...X", "X....", "X.XXX", "X...X", "X...X", ".XXX."},
	'H': {"X...X", "X...X", "X...X", "XXXXX", "X...X", "X...X", "X...X"},
	'I': {"XXX", ".X.", ".X.", ".X.", ".X.", ".X.", "XXX"},
	'J': {"..XXX", "...X.", "...X.", "...X.", "...X.", "X..X.", ".XX.."},
	'K': {"X...X", "X..X.", "X.X..", "XX...", "X.X..", "X..X.", "X...X"},
	'L': {"X....", "X....", "X....", "X....", "X....", "X....", "XXXXX"},
	'M': {"X...X", "XX.XX", "X.X.X", "X.X.X", "X...X", "X...X", "X...X"},
	'N': {"X...X", "X...X", "XX..X", "X.X.X", "X..XX", "X...X", "X...X"},
	'O': {".XXX.", "X...X", "X...X", "X...X", "X...X", "X...X", ".XXX."},
	'P': {"XXXX.", "X...X", "X...X", "XXXX.", "X....", "X....", "X...."},
	'Q': {".XXX.", "X...X", "X...X", "X...X", "X.X.X", "X..X.", ".XX.X"},
	'R': {"XXXX.", "X...X", "X...X", "XXXX.", "X.X..", "X..X.", "X...X"},
	'S': {".XXXX", "X....", "X....", ".XXX.", "....X", "....X", "XXXX."},
	'T': {"XXXXX", "..X..", "..X..", "..X..", "..X..", "..X..", "..X.."},
	'U': {"X...X", "X...X", "X...X", "X...X", "X...X", "X...X", ".XXX."},
	'V': {"X...X", "X...X", "X...X", "X...X", "X...X", ".X.X.", "..X.."},
	'W': {"X...X", "X...X", "X...X", "X.X.X", "X.X.X", "XX.XX", "X...X"},
	'X': {"X...X", "X...X", ".X.X.", "..X..", ".X.X.", "X...X", "X...X"},
	'Y': {"X...X", "X...X", ".X.X.", "..X..", "..X..", "..X..", "..X.."},
	'Z': {"XXXXX", "....X", "...X.", "..X..", ".X...", "X....", "XXXXX"},

	'0': {".XXX.", "X...X", "X..XX", "X.X.X", "XX..X", "X...X", ".XXX."},
	'1': {"..X..", ".XX..", "..X..", "..X..", "..X..", "..X..", ".XXX."},
	'2': {".XXX.", "X...X", "....X", "...X.", "..X..", ".X...", "XXXXX"},
	'3': {"XXXXX", "...X.", "..X..", "...X.", "....X", "X...X", ".XXX."},
	'4': {"...X.", "..XX.", ".X.X.", "X..X.", "XXXXX", "...X.", "...X."},
	'5': {"XXXXX", "X....", "XXXX.", "....X", "....X", "X...X", ".XXX."},
	'6': {"..XX.", ".X...", "X....", "XXXX.", "X...X", "X...X", ".XXX."},
	'7': {"XXXXX", "....X", "...X.", "..X..", ".X...", ".X...", ".X..."},
	'8': {".XXX.", "X...X", "X...X", ".XXX.", "X...X", "X...X", ".XXX."},
	'9': {".XXX.", "X...X", "X...X", ".XXXX", "....X", "...X.", ".XX.."},

	'.':  {".", ".", ".", ".", ".", ".", "X"},
	',':  {"..", "..", "..", "..", "..", ".X", "X."},
	'!':  {"X", "X", "X", "X", "X", ".", "X"},
	'?':  {".XXX.", "X...X", "....X", "..XX.", "..X..", ".....", "..X.."},
	'-':  {"...", "...", "...", "XXX", "...", "...", "..."},
	':':  {".", ".", "X", ".", ".", "X", "."},
	'(':  {".X", "X.", "X.", "X.", "X.", "X.", ".X"},
	')':  {"X.", ".X", ".X", ".X", ".X", ".X", "X."},
	'%':  {"XX..X", "XX.X.", "...X.", "..X..", ".X...", ".X.XX", "X..XX"},
	'_':  {"....", "....", "....", "....", "....", "....", "XXXX"},
	'>':  {"X..", ".X.", "..X", "..X", "..X", ".X.", "X.."},
	'/':  {"....X", "...X.", "...X.", "..X..", ".X...", ".X...", "X...."},
	'"':  {"X.X", "X.X", "...", "...", "...", "...", "..."},
	'\'': {"X", "X", ".", ".", ".", ".", "."},
	'♥':  {".....", ".X.X.", "XXXXX", "XXXXX", ".XXX.", "..X..", "....."},
	'█':  {"XXXX", "XXXX", "XXXX", "XXXX", "XXXX", "XXXX", "XXXX"},
	'░':  {"XXXX", "X..X", "X..X", "X..X", "X..X", "X..X", "XXXX"},
}
