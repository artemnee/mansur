package main

import (
	"math"
	"math/rand/v2"
)

const tps = 20 // ticks per second

type state int

const (
	stIdle state = iota
	stWalk
	stHappy
	stJump
	stDrag
	stFall
	stSleep
	stEat
	stSurprised
	stClimb
)

const (
	pHeart = iota
	pZ
	pBang
)

type particle struct {
	x, y, vx, vy float64
	kind, life   int
}

// plat is the top edge of another app's window that Mansur can stand on (screen coordinates).
type plat struct{ x1, x2, y float64 }

type Pet struct {
	st      state
	stT     int // ticks spent in the current state
	stDur   int // how long the current state lasts (idle/walk)
	t       int
	dir     int
	look    int
	x, y    float64 // bottom-left corner of the window, screen coordinates
	vy      float64
	lift    float64 // jump height, in canvas pixels
	liftV   float64
	land    int // landing ticks: squashed
	blinkIn int
	blinkT  int
	petT    int // accumulated petting
	happyT  int
	calm    int // ticks without attention
	hunger  float64
	bubbleT int
	parts   []particle
	scale   float64
	size    float64
	mouse   NSPoint

	plats              []plat  // windows, front to back
	home               float64 // he wanders around this point; moves when Mansur is dragged
	climbFrom, climbTo NSPoint

	saying   string // the mood Mansur wants to talk about
	sayCD    int
	night    bool
	breaks   bool
	lastMove int
	workT    int
}

func newPet(scale float64, area NSRect) *Pet {
	p := &Pet{scale: scale, size: cw * scale, dir: -1, blinkIn: 60, hunger: 30, breaks: true}
	p.x = area.Origin.X + area.Size.W - p.size - 24
	p.y = area.Origin.Y
	p.home = p.x
	p.set(stIdle)
	return p
}

func (p *Pet) set(s state) {
	p.st, p.stT = s, 0
	switch s {
	case stIdle:
		p.stDur = 40 + rand.IntN(120)
	case stWalk:
		p.stDur = 40 + rand.IntN(100)
	}
}

// say makes Mansur want to talk (with probability chance, at most once every 6 seconds).
func (p *Pet) say(mood string, chance float64) {
	if p.sayCD == 0 && rand.Float64() < chance {
		p.saying, p.sayCD = mood, tps*6
	}
}

func (p *Pet) chatMood() string {
	switch {
	case p.night:
		return "night"
	case p.hunger > 70:
		return "hungry"
	}
	return "ok"
}

// over reports whether the cursor is over the pet's body (not the empty space for hearts).
func (p *Pet) over(m NSPoint) bool {
	lx := (m.X - p.x) / p.scale
	ly := ch - (m.Y-p.y)/p.scale
	return lx >= 5 && lx <= 27 && ly >= 9-p.lift && ly <= 31
}

const zone = 300 // how far (in points) Mansur wanders from home

// surface finds what to stand on at (x, y): the highest window edge not above y, else the screen bottom.
// It returns the height and the walkable bounds.
func (p *Pet) surface(area NSRect, x, y float64) (fy, minX, maxX float64) {
	fy, minX, maxX = area.Origin.Y, area.Origin.X, area.Origin.X+area.Size.W-p.size
	cx := x + p.size/2
	for _, pl := range p.plats {
		if cx >= pl.x1 && cx <= pl.x2 && pl.y <= y+2 && pl.y > fy {
			fy, minX, maxX = pl.y, pl.x1-p.size/2+4*p.scale, pl.x2-p.size/2-4*p.scale
		}
	}
	return
}

// walkArea returns the ground under Mansur and his walking bounds: no farther than zone from home.
func (p *Pet) walkArea(area NSRect) (fy, minX, maxX float64) {
	fy, minX, maxX = p.surface(area, p.x, p.y)
	minX, maxX = math.Max(minX, p.home-zone), math.Min(maxX, p.home+zone)
	if minX > maxX {
		minX, maxX = p.x, p.x
	}
	return
}

func (p *Pet) spawn(kind int, x, y float64) {
	pt := particle{x: x, y: y, kind: kind}
	switch kind {
	case pHeart:
		pt.vx, pt.vy, pt.life = (rand.Float64()-0.5)*0.2, -0.3, 25
	case pZ:
		pt.vx, pt.vy, pt.life = 0.12, -0.2, 35
	case pBang:
		pt.life = 12
	}
	p.parts = append(p.parts, pt)
}

func (p *Pet) heart() {
	p.spawn(pHeart, 8+rand.Float64()*14, 10-p.lift)
}

func (p *Pet) jump() {
	p.liftV = 1.8
	p.set(stJump)
}

// startClimb jumps along an arc to a point (onto a window or down).
func (p *Pet) startClimb(x, y float64) {
	p.climbFrom, p.climbTo = NSPoint{p.x, p.y}, NSPoint{x, y}
	p.set(stClimb)
}

// tryClimb jumps onto the frontmost window if it is within reach.
func (p *Pet) tryClimb(area NSRect) bool {
	if len(p.plats) == 0 {
		return false
	}
	pl := p.plats[0]
	if pl.y < p.y+p.size/2 || pl.y+p.size > area.Origin.Y+area.Size.H || pl.x2-pl.x1 < p.size*1.5 {
		return false
	}
	tx := math.Max(pl.x1-p.size/2+4*p.scale, math.Min(pl.x2-p.size/2-4*p.scale, p.x))
	if math.Abs(tx-p.home) > zone || pl.y-area.Origin.Y > 350 {
		return false
	}
	p.startClimb(tx, pl.y)
	return true
}

func (p *Pet) update(m NSPoint, area NSRect) {
	p.t++
	p.stT++
	p.calm++
	p.hunger = math.Min(100, p.hunger+100.0/(tps*60*90))
	if p.sayCD > 0 {
		p.sayCD--
	}

	moved := math.Hypot(m.X-p.mouse.X, m.Y-p.mouse.Y) > 0.5
	p.mouse = m
	if moved {
		p.lastMove = p.t
	}
	// Break reminder: 50 minutes at the computer in a row (a pause of 5+ minutes resets the count).
	if p.t-p.lastMove < tps*60*5 {
		p.workT++
	} else {
		p.workT = 0
	}
	if p.breaks && p.workT >= tps*60*50 {
		p.workT, p.sayCD = 0, 0
		p.say("break", 1)
		if p.st == stIdle || p.st == stWalk {
			p.jump()
		}
	}

	if p.st != stDrag && p.over(m) && moved {
		p.petT++
	} else if p.petT > 0 {
		p.petT--
	}
	if p.petT >= 4 {
		p.calm = 0
		p.happyT = 15
		if p.st == stIdle || p.st == stWalk {
			p.set(stHappy)
			p.say("pet", 0.5)
		}
	}

	if dx := m.X - (p.x + p.size/2); math.Abs(dx) < 400 && math.Abs(dx) > p.size/4 {
		p.look = int(math.Copysign(1, dx))
	} else {
		p.look = 0
	}

	if p.blinkIn--; p.blinkIn <= 0 {
		p.blinkT, p.blinkIn = 3, 60+rand.IntN(80)
	}
	if p.blinkT > 0 {
		p.blinkT--
	}
	if p.land > 0 {
		p.land--
	}
	if p.bubbleT > 0 {
		p.bubbleT--
	}

	ground, minX, maxX := p.walkArea(area)
	onWindow := ground > area.Origin.Y

	switch p.st {
	case stIdle:
		sleepAfter := tps * 60 * 3
		if p.night {
			sleepAfter = tps * 60
		}
		if p.calm > sleepAfter {
			p.set(stSleep)
			p.say("sleep", 0.7)
			break
		}
		if p.hunger > 70 && p.bubbleT == 0 && rand.IntN(tps*15) == 0 {
			p.bubbleT = tps * 3
			p.say("hungry", 0.6)
		}
		if p.night && rand.IntN(tps*60*20) == 0 {
			p.say("night", 1)
		}
		if rand.IntN(tps*150) == 0 {
			p.say("idle", 1)
		}
		if p.stT > p.stDur {
			switch r := rand.IntN(100); {
			case r < 15 && !onWindow && p.tryClimb(area):
			case r < 25 && onWindow:
				fy, _, _ := p.surface(area, p.x, p.y-3)
				p.startClimb(p.x, fy)
			case r < 65:
				p.dir = []int{-1, 1}[rand.IntN(2)]
				p.set(stWalk)
			default:
				p.set(stIdle)
			}
		}
	case stWalk:
		p.x += float64(p.dir) * p.scale * 0.5
		if p.x <= minX {
			p.dir = 1
		}
		if p.x >= maxX {
			p.dir = -1
		}
		if p.stT > p.stDur {
			p.set(stIdle)
		}
	case stHappy:
		if p.t%5 == 0 {
			p.heart()
		}
		if p.happyT--; p.happyT <= 0 {
			p.set(stIdle)
		}
	case stJump:
		p.lift += p.liftV
		p.liftV -= 0.5
		if p.lift <= 0 {
			p.lift, p.land = 0, 4
			p.set(stIdle)
		}
	case stClimb:
		const dur = 24
		k := float64(p.stT) / dur
		a, b := p.climbFrom, p.climbTo
		arc := 30 + 0.25*math.Abs(b.Y-a.Y)
		p.x = a.X + (b.X-a.X)*k
		p.y = a.Y + (b.Y-a.Y)*k + arc*4*k*(1-k)
		if p.stT >= dur {
			p.x, p.y, p.land = b.X, b.Y, 5
			p.set(stIdle)
			if b.Y > a.Y {
				p.say("climb", 0.6)
			}
		}
	case stFall:
		p.vy -= 0.35 * p.scale
		p.y += p.vy
		if p.y <= ground {
			p.y, p.vy, p.land = ground, 0, 5
			p.set(stIdle)
		}
	case stSleep:
		if p.t%30 == 0 {
			p.spawn(pZ, 22, 12)
		}
		if p.happyT > 0 {
			p.happyT--
			if p.t%8 == 0 {
				p.heart()
			}
		}
	case stEat:
		if p.stT >= 60 {
			p.hunger, p.bubbleT, p.sayCD = 0, 0, 0
			p.say("eat", 1)
			p.heart()
			p.heart()
			p.happyT = 20
			p.set(stHappy)
		}
	case stSurprised:
		if p.stT >= 12 {
			p.set(stIdle)
		}
	}

	if p.st != stDrag && p.st != stClimb {
		ground, minX, maxX = p.walkArea(area)
		p.x = math.Max(minX, math.Min(maxX, p.x))
		if p.y < ground {
			p.y = ground
		}
		if p.y > ground+0.5 && p.st != stFall {
			p.vy = 0
			p.set(stFall)
		}
	}

	alive := p.parts[:0]
	for _, pt := range p.parts {
		pt.x += pt.vx
		pt.y += pt.vy
		if pt.life--; pt.life > 0 {
			alive = append(alive, pt)
		}
	}
	p.parts = alive
}

func (p *Pet) click() {
	p.calm = 0
	switch p.st {
	case stSleep:
		p.spawn(pBang, 25, 6)
		p.set(stSurprised)
		p.sayCD = 0
		p.say("wake", 1)
	case stJump, stFall, stEat, stClimb:
	default:
		if rand.IntN(3) == 0 {
			p.heart()
		}
		p.jump()
		p.say("jump", 0.25)
	}
}

func (p *Pet) startDrag() {
	p.calm, p.lift = 0, 0
	p.set(stDrag)
	p.say("drag", 0.6)
}

func (p *Pet) drop() {
	p.vy, p.home = 0, p.x
	p.set(stFall)
}

func (p *Pet) feed() {
	p.calm, p.lift = 0, 0
	p.set(stEat)
}

func (p *Pet) toggleSleep() {
	p.calm = 0
	if p.st == stSleep {
		p.set(stIdle)
	} else {
		p.lift = 0
		p.set(stSleep)
	}
}

// listen is called when the human writes something: he wakes up and looks.
func (p *Pet) listen() {
	p.calm = 0
	if p.st == stSleep {
		p.set(stSurprised)
	}
}

func (p *Pet) pose() pose {
	ps := pose{eyes: eyeOpen, mouth: mouthSmile, look: p.look, lift: int(math.Round(p.lift)), t: p.t}
	switch p.st {
	case stIdle:
		ps.body = bodySit
		if (p.t/20)%2 == 1 {
			ps.squash = 1
		}
		if p.hunger >= 90 {
			ps.mouth = mouthFlat
		}
	case stWalk:
		ps.look = p.dir
		step := (p.t / 3) % 2
		ps.feet = feetStepL + feet(step)
		ps.lift += step
	case stHappy:
		ps.body, ps.eyes = bodySit, eyeHappy
		ps.squash = (p.t / 4) % 2
	case stJump, stClimb:
		ps.eyes, ps.mouth = eyeWide, mouthO
		if p.liftV > 0 || p.st == stClimb {
			ps.squash = -1
		}
	case stDrag, stFall:
		ps.eyes, ps.mouth, ps.feet, ps.squash = eyeWide, mouthO, feetDangle, -1
	case stSleep:
		ps.body, ps.eyes, ps.mouth, ps.look = bodyLie, eyeClosed, mouthFlat, 0
		if (p.t/30)%2 == 1 {
			ps.squash = 1
		}
	case stEat:
		ps.body, ps.eyes, ps.look = bodySit, eyeHappy, -1
		if (p.t/5)%2 == 0 {
			ps.mouth = mouthO
		} else {
			ps.mouth = mouthFlat
		}
	case stSurprised:
		ps.body, ps.eyes, ps.mouth = bodySit, eyeWide, mouthO
	}
	if p.land > 0 {
		ps.squash = 2
	}
	if p.blinkT > 0 && ps.eyes == eyeOpen {
		ps.eyes = eyeClosed
	}
	return ps
}

func (p *Pet) render() *canvas {
	c := &canvas{}
	f := drawCat(c, p.pose())

	if p.st == stEat {
		// The berry near the mouth shrinks as he eats it.
		bx, by := f.mouthX-5, f.mouthY-1
		switch {
		case p.stT < 20:
			c.sprite(bx, by, berryArt, artPal)
		case p.stT < 40:
			c.sprite(bx, by+1, berryArt[1:3], artPal)
		default:
			c.set(bx+1, by+2, cBerry)
		}
	}
	if p.bubbleT > 0 && p.st == stIdle {
		c.sprite(18, 0, bubbleArt, artPal)
		c.set(17, 7, cLine)
	}

	zPal := map[byte]rgba{'X': cZ}
	for _, pt := range p.parts {
		x, y := int(math.Round(pt.x)), int(math.Round(pt.y))
		switch pt.kind {
		case pHeart:
			c.sprite(x-2, y-2, heartArt, artPal)
		case pZ:
			c.sprite(x-1, y-1, zArt, zPal)
		case pBang:
			c.sprite(x, y, bangArt, artPal)
		}
	}
	return c
}
