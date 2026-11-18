package main

// A thin AppKit layer via purego/objc. No cgo, so `go install` does not need Xcode.
// Three transparent windows: Mansur himself, the speech bubble and the pixel panel (menu / text input).

import (
	"bytes"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"os/signal"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/artemnee/mansur/brain"
)

type NSPoint struct{ X, Y float64 }
type NSSize struct{ W, H float64 }
type NSRect struct {
	Origin NSPoint
	Size   NSSize
}

var (
	cfDataCreate                   func(alloc uintptr, data *byte, length int) uintptr
	cfRelease                      func(ref uintptr)
	cgColorSpaceCreateDeviceRGB    func() uintptr
	cgDataProviderCreateWithCFData func(data uintptr) uintptr
	cgImageCreate                  func(w, h, bpc, bpp, bpr int, space uintptr, info uint32, provider uintptr, decode uintptr, interpolate bool, intent int32) uintptr
	cgWindowListCopyWindowInfo     func(option, relativeTo uint32) uintptr
	cgRectFromDict                 func(dict uintptr, rect *NSRect) bool
)

var sels = map[string]objc.SEL{}

func sel(name string) objc.SEL {
	s, ok := sels[name]
	if !ok {
		s = objc.RegisterName(name)
		sels[name] = s
	}
	return s
}

func cls(name string) objc.ID { return objc.ID(objc.GetClass(name)) }

func nsStr(s string) objc.ID {
	b := append([]byte(s), 0)
	id := cls("NSString").Send(sel("stringWithUTF8String:"), &b[0])
	runtime.KeepAlive(b)
	return id
}

func goStr(s objc.ID) string {
	if s == 0 {
		return ""
	}
	n := objc.Send[uint](s, sel("lengthOfBytesUsingEncoding:"), uint(4))
	buf := make([]byte, n+1)
	objc.Send[bool](s, sel("getCString:maxLength:encoding:"), &buf[0], n+1, uint(4))
	return string(buf[:n])
}

func loadLibs() {
	open := func(name string) uintptr {
		h, err := purego.Dlopen(fmt.Sprintf("/System/Library/Frameworks/%[1]s.framework/%[1]s", name), purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			panic(err)
		}
		return h
	}
	open("AppKit")
	open("QuartzCore")
	cf := open("CoreFoundation")
	cg := open("CoreGraphics")
	purego.RegisterLibFunc(&cfDataCreate, cf, "CFDataCreate")
	purego.RegisterLibFunc(&cfRelease, cf, "CFRelease")
	purego.RegisterLibFunc(&cgColorSpaceCreateDeviceRGB, cg, "CGColorSpaceCreateDeviceRGB")
	purego.RegisterLibFunc(&cgDataProviderCreateWithCFData, cg, "CGDataProviderCreateWithCFData")
	purego.RegisterLibFunc(&cgImageCreate, cg, "CGImageCreate")
	purego.RegisterLibFunc(&cgWindowListCopyWindowInfo, cg, "CGWindowListCopyWindowInfo")
	purego.RegisterLibFunc(&cgRectFromDict, cg, "CGRectMakeWithDictionaryRepresentation")
}

func mouseLoc() NSPoint { return objc.Send[NSPoint](cls("NSEvent"), sel("mouseLocation")) }

// surface is a transparent borderless window that shows a pixel image.
type surface struct {
	win, view, layer objc.ID
	frame            NSRect
	prev             []byte
	shown            bool
}

var colorSpace uintptr

func newSurface(winClass string, viewClass objc.Class, mouse bool) *surface {
	win := cls(winClass).Send(sel("alloc")).Send(sel("initWithContentRect:styleMask:backing:defer:"),
		NSRect{Size: NSSize{1, 1}}, uint(0), uint(2), false)
	win.Send(sel("setOpaque:"), false)
	win.Send(sel("setBackgroundColor:"), cls("NSColor").Send(sel("clearColor")))
	win.Send(sel("setHasShadow:"), false)
	win.Send(sel("setReleasedWhenClosed:"), false)
	win.Send(sel("setLevel:"), 3) // NSFloatingWindowLevel
	// canJoinAllSpaces | stationary | ignoresCycle | fullScreenAuxiliary
	win.Send(sel("setCollectionBehavior:"), uint(1|1<<4|1<<6|1<<8))
	if !mouse {
		win.Send(sel("setIgnoresMouseEvents:"), true)
	}
	view := objc.ID(viewClass).Send(sel("alloc")).Send(sel("initWithFrame:"), NSRect{Size: NSSize{1, 1}})
	view.Send(sel("setWantsLayer:"), true)
	layer := view.Send(sel("layer"))
	layer.Send(sel("setMagnificationFilter:"), nsStr("nearest"))
	win.Send(sel("setContentView:"), view)
	return &surface{win: win, view: view, layer: layer}
}

func (s *surface) place(x, y, w, h float64) {
	r := NSRect{NSPoint{math.Round(x), math.Round(y)}, NSSize{w, h}}
	if r != s.frame {
		s.frame = r
		s.win.Send(sel("setFrame:display:"), r, false)
	}
}

func (s *surface) draw(w, h int, buf []byte) {
	if bytes.Equal(buf, s.prev) {
		return
	}
	s.prev = buf
	data := cfDataCreate(0, &buf[0], len(buf))
	prov := cgDataProviderCreateWithCFData(data)
	// 1 = kCGImageAlphaPremultipliedLast; pixels are either opaque or empty.
	img := cgImageCreate(w, h, 8, 32, w*4, colorSpace, 1, prov, 0, false, 0)
	s.layer.Send(sel("setContents:"), objc.ID(img))
	cfRelease(img)
	cfRelease(prov)
	cfRelease(data)
}

func (s *surface) show() {
	if !s.shown {
		s.win.Send(sel("orderFrontRegardless"))
		s.shown = true
	}
}

func (s *surface) hide() {
	if s.shown {
		s.win.Send(sel("orderOut:"), objc.ID(0))
		s.shown = false
	}
}

// Dark green theme for the bubble and menu: background, highlighted item, border, text, dimmed text.
const (
	uiBg     rgba = 0x141f12ff
	uiHover  rgba = 0x2e4a24ff
	uiBorder rgba = 0x9cc75aff
	uiText   rgba = 0xeef7d8ff
	uiGrey   rgba = 0x7f9a6aff
)

const (
	modeNone = iota
	modeMenu
	modeChat
)

type item struct {
	label string
	act   func() // nil means a plain label
}

type ui struct {
	app              objc.ID
	pet              *Pet
	petS, bubS, panS *surface
	fs               float64 // pixel size of the bubble and panel

	downAt, grab NSPoint
	dragging     bool

	brain    *brain.Model
	replies  chan reply
	history  []brain.Turn // conversation memory: the last exchanges
	lastChat time.Time
	thinking bool

	bubble        []rune
	bubT, bubLife int

	mode   int
	items  []item
	hover  int
	input  []rune
	panelT int

	born   time.Time
	sig    chan os.Signal
	keys   map[string]objc.ID
	timer  objc.ID
	upd    chan string // latest release tag from a background check
	newVer string      // a newer release is out
	slow   bool        // ticking at 1/slowSteps rate while nothing happens
	onQuit func()
}

var u *ui

// area is the visible part of the screen the pet is on (without the menu bar and Dock).
func (u *ui) area() NSRect {
	scr := u.petS.win.Send(sel("screen"))
	if scr == 0 {
		scr = cls("NSScreen").Send(sel("mainScreen"))
	}
	return objc.Send[NSRect](scr, sel("visibleFrame"))
}

func (u *ui) key(s string) objc.ID {
	if k, ok := u.keys[s]; ok {
		return k
	}
	k := nsStr(s).Send(sel("retain"))
	u.keys[s] = k
	return k
}

// scanWindows refreshes the list of windows Mansur can climb onto.
func (u *ui) scanWindows() {
	arr := cgWindowListCopyWindowInfo(1|16, 0) // onScreenOnly | excludeDesktopElements
	if arr == 0 {
		return
	}
	defer cfRelease(arr)
	list := objc.ID(arr)
	n := objc.Send[int](list, sel("count"))
	primaryH := objc.Send[NSRect](cls("NSScreen").Send(sel("screens")).Send(sel("firstObject")), sel("frame")).Size.H
	me := int32(os.Getpid())
	var plats []plat
	for i := 0; i < n && len(plats) < 8; i++ {
		d := list.Send(sel("objectAtIndex:"), uint(i))
		if objc.Send[int32](d.Send(sel("objectForKey:"), u.key("kCGWindowLayer")), sel("intValue")) != 0 ||
			objc.Send[int32](d.Send(sel("objectForKey:"), u.key("kCGWindowOwnerPID")), sel("intValue")) == me {
			continue
		}
		var r NSRect
		if !cgRectFromDict(uintptr(d.Send(sel("objectForKey:"), u.key("kCGWindowBounds"))), &r) || r.Size.W < 150 || r.Size.H < 80 {
			continue
		}
		plats = append(plats, plat{r.Origin.X, r.Origin.X + r.Size.W, primaryH - r.Origin.Y})
	}
	u.pet.plats = plats
}

func (u *ui) tick() {
	pool := cls("NSAutoreleasePool").Send(sel("new"))
	defer pool.Send(sel("drain"))

	select {
	case <-u.sig:
		u.quit()
	default:
	}
	select {
	case r := <-u.replies:
		u.thinking = false
		if r.q != "" {
			u.history = append(u.history, brain.Turn{Q: r.q, A: r.text})
			u.history = u.history[max(0, len(u.history)-2):]
			u.lastChat = time.Now()
		}
		if r.text != "" {
			u.showBubble(u.fill(r.text))
		}
	default:
	}
	select {
	case tag := <-u.upd:
		if newer(tag, appVersion()) && tag != u.newVer {
			u.newVer = tag
			u.showBubble(fmt.Sprintf(tr("update_new"), tag))
		}
	default:
	}

	p := u.pet
	steps := 1
	if u.slow {
		steps = slowSteps
	}
	m, area := mouseLoc(), u.area()
	for range steps {
		if p.t%tps == 0 {
			h := time.Now().Hour()
			p.night = h >= 23 || h < 5
		}
		if p.t%(tps*5) == 0 {
			u.scanWindows()
		}
		if minute := tps * 60 / fast; p.t%minute == 0 && p.t > 0 {
			p.bond.minute(time.Now(), p.t-p.lastMove < minute, p.calm > minute*30)
			u.offerUpdates()
			u.checkUpdates(false)
			saveState(p, u.born)
			if fast > 1 {
				b := p.bond
				fmt.Printf("affection %.2f  cuddly %.2f  greedy %.2f  independent %.2f  chatty %.2f  sulky %v\n",
					b.Affection, b.Cuddly, b.Greedy, b.Independent, b.Chatty, b.sulky())
			}
		}
		p.update(m, area)
		if p.saying != "" && !u.thinking {
			u.speak(p.saying, "")
			p.saying = ""
		}
	}
	u.syncPet()
	u.updateBubble()
	u.updatePanel()
	u.setSlow(u.quiet(m))
}

// slowSteps: when nothing is happening the timer fires this many times less often, and each
// firing runs that many logic steps, so the pet lives at the same speed while the Mac wakes up less.
const slowSteps = 4

// quiet: Mansur sits or sleeps, nothing is on screen around him and the cursor is far away.
func (u *ui) quiet(m NSPoint) bool {
	p := u.pet
	near := math.Abs(m.X-(p.x+p.size/2)) < 300 && math.Abs(m.Y-(p.y+p.size/2)) < 300
	return (p.st == stIdle || p.st == stSleep) && len(p.parts) == 0 && p.land == 0 && p.bubbleT == 0 &&
		u.bubble == nil && u.mode == modeNone && !u.thinking && p.saying == "" && !near
}

func (u *ui) setSlow(slow bool) {
	if slow != u.slow || u.timer == 0 {
		u.slow = slow
		u.startTimer()
	}
}

func (u *ui) startTimer() {
	if u.timer != 0 {
		u.timer.Send(sel("invalidate"))
	}
	interval, tolerance := 1.0/float64(tps), 0.01
	if u.slow {
		interval, tolerance = slowSteps/float64(tps), 0.05
	}
	u.timer = cls("NSTimer").Send(sel("timerWithTimeInterval:target:selector:userInfo:repeats:"),
		interval, u.petS.view, sel("tick:"), objc.ID(0), true)
	u.timer.Send(sel("setTolerance:"), tolerance)
	cls("NSRunLoop").Send(sel("currentRunLoop")).Send(sel("addTimer:forMode:"), u.timer, nsStr("kCFRunLoopCommonModes"))
}

func (u *ui) syncPet() {
	p := u.pet
	u.petS.place(p.x, p.y, p.size, p.size)
	u.petS.draw(cw, ch, p.render().bytes())
}

// reply is a finished answer; q is the question it answered (empty means nothing to remember).
type reply struct{ q, text string }

// minConf: below this confidence an answer counts as made up at random. On the held-out test
// of the bilingual model it drops about one wrong answer in six and almost no right ones; the
// network is often confidently wrong, so a higher threshold would mostly cut good answers.
const minConf = 0.80

// speak asks the mini-GPT for an answer in the background so the animation does not freeze.
// An empty text means Mansur talks on his own; otherwise he answers the human, remembering past exchanges.
func (u *ui) speak(mood, text string) {
	if u.brain == nil || u.thinking {
		return
	}
	u.thinking = true
	var history []brain.Turn
	if text != "" && time.Since(u.lastChat) < 3*time.Minute {
		history = append(history, u.history...)
	}
	lang := uiLang()
	if text != "" {
		lang = msgLang(text) // answer in the language the human wrote in
	}
	mood = lang + "/" + mood
	go func() {
		rng := rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
		// Try both with and without memory: follow-ups ("and tomorrow?") only make sense with it,
		// while unrelated memory sometimes confuses plain questions. Keep the more confident answer.
		contexts := [][]brain.Turn{nil}
		if len(history) > 0 {
			contexts = append(contexts, history)
		}
		best, bestConf := "", -1.0
		for range 2 {
			for _, h := range contexts {
				if r, conf := u.brain.Reply(mood, h, text, 0.7, rng); r != "" && conf > bestConf {
					best, bestConf = r, conf
				}
			}
		}
		q := text
		if text != "" && bestConf < minConf {
			// The network is unsure, so let it honestly say it doesn't know: the "unknown" mood was
			// trained to produce exactly that. Such answers are not remembered.
			best, _ = u.brain.Reply(lang+"/unknown", nil, "", 0.7, rng)
			q = ""
		}
		u.replies <- reply{q: q, text: best}
	}()
}

// fill puts into the answer what the network cannot know: time, date, satiety, age.
func (u *ui) fill(s string) string {
	now := time.Now()
	days := []string{"воскресенье", "понедельник", "вторник", "среда", "четверг", "пятница", "суббота"}
	months := []string{"января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"}
	name := func(fallback string) string {
		if n := u.pet.bond.Name; n != "" {
			return strings.ToLower(n) // the brain writes in lower case; the font is upper case anyway
		}
		return fallback
	}
	age, ageEn := "меньше дня", "less than a day"
	if n := int(time.Since(u.born).Hours() / 24); n == 1 {
		ageEn = "1 day"
	} else if n > 1 {
		ageEn = fmt.Sprintf("%d days", n)
	}
	switch n := int(time.Since(u.born).Hours() / 24); {
	case n == 0:
	case n%10 == 1 && n%100 != 11:
		age = fmt.Sprintf("%d день", n)
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
		age = fmt.Sprintf("%d дня", n)
	default:
		age = fmt.Sprintf("%d дней", n)
	}
	s = strings.NewReplacer(
		"{время}", now.Format("15:04"),
		"{дата}", fmt.Sprintf("%d %s", now.Day(), months[now.Month()-1]),
		"{день}", days[now.Weekday()],
		"{вчера}", days[(now.Weekday()+6)%7],
		"{завтра}", days[(now.Weekday()+1)%7],
		"{послезавтра}", days[(now.Weekday()+2)%7],
		"{сытость}", fmt.Sprintf("%d%%", 100-int(u.pet.hunger)),
		"{возраст}", age,
		"{имя}", name("человек"),
		"{name_en}", name("friend"),
		"{time_en}", strings.ToLower(now.Format("3:04 PM")),
		"{date_en}", strings.ToLower(now.Format("January 2")),
		"{day_en}", strings.ToLower(now.Weekday().String()),
		"{yesterday_en}", strings.ToLower(now.AddDate(0, 0, -1).Weekday().String()),
		"{tomorrow_en}", strings.ToLower(now.AddDate(0, 0, 1).Weekday().String()),
		"{after_en}", strings.ToLower(now.AddDate(0, 0, 2).Weekday().String()),
		"{hunger_en}", fmt.Sprintf("%d%%", 100-int(u.pet.hunger)),
		"{age_en}", ageEn,
	).Replace(s)
	return strings.NewReplacer("{", "", "}", "").Replace(s) // in case the network left a template unfinished
}

func (u *ui) showBubble(s string) {
	u.bubble, u.bubT = []rune(s), 0
	u.bubLife = tps*3 + len(u.bubble)*3
}

func (u *ui) updateBubble() {
	if u.bubble == nil {
		u.bubS.hide()
		return
	}
	if u.bubT++; u.bubT > u.bubLife {
		u.bubble = nil
		u.bubS.hide()
		return
	}
	lines := wrap(string(u.bubble), 96)
	tw := 0
	for _, l := range lines {
		tw = max(tw, textW(l))
	}
	w, boxH := tw+10, len(lines)*lineH-2+8
	h := boxH + 4
	pm := newPixmap(w, h)
	pm.box(0, 0, w, boxH, uiBg, uiBorder)
	cx := w/2 - 2
	pm.fill(cx+1, boxH-1, 2, 1, uiBg)
	pm.set(cx, boxH, uiBorder)
	pm.fill(cx+1, boxH, 2, 1, uiBg)
	pm.set(cx+3, boxH, uiBorder)
	pm.set(cx+1, boxH+1, uiBorder)
	pm.set(cx+2, boxH+1, uiBg)
	pm.set(cx+3, boxH+1, uiBorder)
	pm.fill(cx+2, boxH+2, 2, 1, uiBorder)
	pm.set(cx+3, boxH+3, uiBorder)
	left := u.bubT // typewriter: one letter per tick
	for i, l := range lines {
		r := []rune(l)
		pm.text(5, 4+i*lineH, string(r[:min(len(r), left)]), uiText)
		left -= len(r) + 1
		if left <= 0 {
			break
		}
	}

	p, a := u.pet, u.area()
	W, H := float64(w)*u.fs, float64(h)*u.fs
	x := p.x + p.size/2 - W/2
	x = math.Max(a.Origin.X, math.Min(a.Origin.X+a.Size.W-W, x))
	y := math.Min(p.y+p.size*0.95, a.Origin.Y+a.Size.H-H) // above the antenna tips
	u.bubS.place(x, y, W, H)
	u.bubS.draw(w, h, pm.bytes())
	u.bubS.show()
}

// offerUpdates mentions once, a few minutes after the first start, that update checks exist.
// They stay off unless the human turns them on.
func (u *ui) offerUpdates() {
	if updates != nil || appVersion() == "" || u.pet.t < tps*60*3/fast || u.bubble != nil || u.thinking {
		return
	}
	off := false
	updates = &off
	u.showBubble(tr("update_offer"))
}

// checkUpdates asks GitHub for the latest release in the background, at most once a day.
func (u *ui) checkUpdates(now bool) {
	if updates == nil || !*updates || appVersion() == "" || !now && time.Since(updateChecked) < 24*time.Hour {
		return
	}
	updateChecked = time.Now()
	go func() {
		if tag, err := latestRelease(); err == nil && tag != "" {
			u.upd <- tag
		}
	}()
}

func (u *ui) toggleUpdates() {
	on := updates == nil || !*updates
	updates = &on
	saveState(u.pet, u.born)
	u.checkUpdates(true)
	u.openMenu()
}

func onoff(b bool) string {
	if b {
		return tr("on")
	}
	return tr("off")
}

func (u *ui) openMenu() {
	p := u.pet
	full := int(math.Round((100 - p.hunger) / 20))
	sleepLabel := tr("sleep")
	if p.st == stSleep {
		sleepLabel = tr("wake")
	}
	var about []item // what he knows about you and what he became
	if n := p.bond.Name; n != "" {
		about = append(about, item{tr("friend") + n, nil})
	}
	if t := p.bond.trait(); t != "" {
		about = append(about, item{tr("trait") + tr(t), nil})
	}
	langLabel := tr("auto")
	if langSetting != "" {
		langLabel = langSetting
	}
	u.items = []item{
		{tr("full") + strings.Repeat("█", full) + strings.Repeat("░", 5-full), nil},
		{fmt.Sprintf(tr("age"), int(time.Since(u.born).Hours()/24)), nil},
		{tr("talk"), func() { u.openInput(modeChat) }},
		{tr("feed"), func() { p.feed(); u.closePanel() }},
		{sleepLabel, func() { p.toggleSleep(); u.closePanel() }},
		{tr("breaks") + onoff(p.breaks), func() { p.breaks = !p.breaks; u.openMenu() }},
		{tr("autostart") + onoff(autostartOn()), func() { _ = setAutostart(!autostartOn()); u.openMenu() }},
		{tr("language") + langLabel, u.nextLang},
		{tr("quit"), u.quit},
	}
	if appVersion() != "" { // local builds have no release to compare with
		u.items = slices.Insert(u.items, len(u.items)-1, item{tr("updates") + onoff(updates != nil && *updates), u.toggleUpdates})
	}
	if u.newVer != "" {
		u.items = slices.Insert(u.items, 0, item{fmt.Sprintf(tr("update_to"), u.newVer), func() { openReleases(); u.closePanel() }})
	}
	u.items = slices.Insert(u.items, 2+min(1, len(u.newVer)), about...)
	if u.mode != modeMenu {
		u.hover = -1
	}
	u.mode = modeMenu
	u.showPanel()
}

// nextLang cycles the language setting: system → ru → en → system.
func (u *ui) nextLang() {
	langSetting = map[string]string{"": "ru", "ru": "en", "en": ""}[langSetting]
	saveState(u.pet, u.born)
	u.openMenu()
}

func (u *ui) openInput(mode int) {
	u.mode, u.input = mode, nil
	u.showPanel()
}

func (u *ui) showPanel() {
	u.panelT = 0
	u.drawPanel()
	u.app.Send(sel("activateIgnoringOtherApps:"), true)
	u.panS.win.Send(sel("makeKeyAndOrderFront:"), objc.ID(0))
	u.panS.win.Send(sel("makeFirstResponder:"), u.panS.view)
	u.panS.shown = true
}

func (u *ui) closePanel() {
	if u.mode == modeNone {
		return
	}
	u.mode = modeNone
	u.panS.hide()
	u.app.Send(sel("deactivate"))
}

const (
	pad   = 4
	itemH = lineH + 2
)

func (u *ui) drawPanel() {
	var pm *pixmap
	switch u.mode {
	case modeMenu:
		tw := 0
		for _, it := range u.items {
			tw = max(tw, textW(it.label))
		}
		w, h := tw+18, pad*2+len(u.items)*itemH
		pm = newPixmap(w, h)
		pm.box(0, 0, w, h, uiBg, uiBorder)
		for i, it := range u.items {
			y := pad + i*itemH
			col := uiText
			if it.act == nil {
				col = uiGrey
			} else if i == u.hover {
				pm.fill(2, y, w-4, itemH, uiHover)
				pm.text(5, y+2, ">", uiText)
			}
			pm.text(12, y+2, it.label, col)
		}
	case modeChat:
		hint := tr("hint")
		w := max(140, textW(hint)+12) // wide enough for the hint line
		tw := w - 12
		title := tr("write")
		text := string(u.input)
		if (u.panelT/8)%2 == 0 {
			text += "_"
		}
		lines := wrap(text, tw)
		h := pad*2 + lineH*(len(lines)+2) + 6
		pm = newPixmap(w, h)
		pm.box(0, 0, w, h, uiBg, uiBorder)
		pm.text(6, pad+1, title, uiGrey)
		for i, l := range lines {
			pm.text(6, pad+lineH+4+i*lineH, l, uiText)
		}
		pm.text(6, h-pad-lineH+1, hint, uiGrey)
	default:
		return
	}

	p, a := u.pet, u.area()
	W, H := float64(pm.w)*u.fs, float64(pm.h)*u.fs
	x := p.x + 4*p.scale - W
	if x < a.Origin.X {
		x = p.x + 28*p.scale
	}
	y := math.Min(p.y+4*p.scale, a.Origin.Y+a.Size.H-H)
	u.panS.place(x, y, W, H)
	u.panS.draw(pm.w, pm.h, pm.bytes())
}

func (u *ui) itemAt(m NSPoint) int {
	f := u.panS.frame
	lx := (m.X - f.Origin.X) / u.fs
	ly := (f.Origin.Y + f.Size.H - m.Y) / u.fs
	if lx < 0 || lx > f.Size.W/u.fs || ly < pad {
		return -1
	}
	i := int((ly - pad) / itemH)
	if i >= len(u.items) {
		return -1
	}
	return i
}

func (u *ui) updatePanel() {
	if u.mode == modeNone {
		return
	}
	u.panelT++
	// Clicked elsewhere: another app took the focus.
	if u.panelT > 5 && !objc.Send[bool](u.panS.win, sel("isKeyWindow")) {
		u.closePanel()
		return
	}
	if u.mode == modeMenu {
		if i := u.itemAt(mouseLoc()); i != u.hover {
			u.hover = i
			u.drawPanel()
		}
	} else if u.panelT%8 == 0 {
		u.drawPanel() // blinking cursor
	}
}

func (u *ui) panelClick() {
	if u.mode != modeMenu {
		return
	}
	if i := u.itemAt(mouseLoc()); i >= 0 && u.items[i].act != nil {
		u.items[i].act()
	}
}

func (u *ui) panelKey(ev objc.ID) {
	code := objc.Send[uint16](ev, sel("keyCode"))
	const cmd = 1 << 20
	switch {
	case code == 53: // esc
		u.closePanel()
		return
	case u.mode == modeMenu:
		switch code {
		case 125, 126: // down, up
			step := 1
			if code == 126 {
				step = -1
			}
			for range len(u.items) {
				u.hover = (u.hover + step + len(u.items)) % len(u.items)
				if u.items[u.hover].act != nil {
					break
				}
			}
		case 36, 76:
			if u.hover >= 0 && u.items[u.hover].act != nil {
				u.items[u.hover].act()
				return
			}
		}
	case code == 36 || code == 76: // enter
		u.submit()
		return
	case code == 51: // backspace
		if len(u.input) > 0 {
			u.input = u.input[:len(u.input)-1]
		}
	case objc.Send[uint](ev, sel("modifierFlags"))&cmd != 0:
		if goStr(ev.Send(sel("charactersIgnoringModifiers"))) == "v" {
			pb := cls("NSPasteboard").Send(sel("generalPasteboard"))
			u.typeText(goStr(pb.Send(sel("stringForType:"), nsStr("public.utf8-plain-text"))))
		}
	default:
		u.typeText(goStr(ev.Send(sel("characters"))))
	}
	u.panelT = 0
	u.drawPanel()
}

func (u *ui) typeText(s string) {
	limit := brain.MaxQ
	for _, r := range s {
		if r >= 32 && r != 127 && (r < 0xF700 || r > 0xF8FF) && len(u.input) < limit {
			u.input = append(u.input, r)
		}
	}
}

func (u *ui) submit() {
	text := strings.TrimSpace(string(u.input))
	if text == "" {
		return
	}
	if n := parseName(text); n != "" {
		u.pet.bond.Name = n
		saveState(u.pet, u.born)
	}
	u.pet.listen()
	u.speak(u.pet.chatMood(), strings.ToLower(text))
	u.input = nil
	u.drawPanel()
}

func (u *ui) mouseDown(ev objc.ID) {
	const controlKey = 1 << 18
	if objc.Send[uint](ev, sel("modifierFlags"))&controlKey != 0 {
		u.toggleMenu()
		return
	}
	m := mouseLoc()
	u.downAt = m
	u.grab = NSPoint{m.X - u.pet.x, m.Y - u.pet.y}
	u.dragging = false
}

func (u *ui) mouseDragged() {
	m := mouseLoc()
	if !u.dragging && math.Hypot(m.X-u.downAt.X, m.Y-u.downAt.Y) > 3 {
		u.dragging = true
		u.closePanel() // the menu is anchored to Mansur, so close it while he is dragged
		u.pet.startDrag()
	}
	if u.dragging {
		u.pet.x, u.pet.y = m.X-u.grab.X, m.Y-u.grab.Y
		u.syncPet()
	}
}

func (u *ui) mouseUp() {
	if u.dragging {
		u.scanWindows() // fresh window edges to land on
		u.pet.drop()
	} else {
		u.pet.click()
	}
	u.dragging = false
	u.syncPet()
}

func (u *ui) toggleMenu() {
	if u.mode == modeMenu {
		u.closePanel()
	} else {
		u.openMenu()
	}
}

func (u *ui) quit() {
	saveState(u.pet, u.born)
	u.onQuit()
	u.app.Send(sel("terminate:"), objc.ID(0))
}

func registerClasses() (petView, panelView objc.Class) {
	type ev = objc.ID
	must := func(c objc.Class, err error) objc.Class {
		if err != nil {
			panic(err)
		}
		return c
	}
	yes := func(_ objc.ID, _ objc.SEL, _ ev) bool { return true }
	petView = must(objc.RegisterClass("MansurView", objc.GetClass("NSView"), nil, nil, []objc.MethodDef{
		{Cmd: sel("acceptsFirstMouse:"), Fn: yes},
		{Cmd: sel("mouseDown:"), Fn: func(_ objc.ID, _ objc.SEL, e ev) { u.mouseDown(e) }},
		{Cmd: sel("mouseDragged:"), Fn: func(_ objc.ID, _ objc.SEL, _ ev) { u.mouseDragged() }},
		{Cmd: sel("mouseUp:"), Fn: func(_ objc.ID, _ objc.SEL, _ ev) { u.mouseUp() }},
		{Cmd: sel("rightMouseDown:"), Fn: func(_ objc.ID, _ objc.SEL, _ ev) { u.toggleMenu() }},
		{Cmd: sel("tick:"), Fn: func(_ objc.ID, _ objc.SEL, _ ev) { u.tick() }},
	}))
	must(objc.RegisterClass("MansurPanel", objc.GetClass("NSWindow"), nil, nil, []objc.MethodDef{
		{Cmd: sel("canBecomeKeyWindow"), Fn: func(_ objc.ID, _ objc.SEL) bool { return true }},
	}))
	panelView = must(objc.RegisterClass("MansurPanelView", objc.GetClass("NSView"), nil, nil, []objc.MethodDef{
		{Cmd: sel("acceptsFirstResponder"), Fn: func(_ objc.ID, _ objc.SEL) bool { return true }},
		{Cmd: sel("acceptsFirstMouse:"), Fn: yes},
		{Cmd: sel("mouseDown:"), Fn: func(_ objc.ID, _ objc.SEL, _ ev) { u.panelClick() }},
		{Cmd: sel("keyDown:"), Fn: func(_ objc.ID, _ objc.SEL, e ev) { u.panelKey(e) }},
	}))
	return
}

func runApp(scale float64, onQuit func()) {
	loadLibs()
	pool := cls("NSAutoreleasePool").Send(sel("new"))
	colorSpace = cgColorSpaceCreateDeviceRGB()

	app := cls("NSApplication").Send(sel("sharedApplication"))
	app.Send(sel("setActivationPolicy:"), 1) // Accessory: no Dock icon and not in Cmd+Tab

	petView, panelView := registerClasses()
	area := objc.Send[NSRect](cls("NSScreen").Send(sel("mainScreen")), sel("visibleFrame"))
	pet := newPet(scale, area)

	u = &ui{
		app: app, pet: pet, fs: math.Max(2, math.Round(scale/2)),
		petS:    newSurface("NSWindow", petView, true),
		bubS:    newSurface("NSWindow", objc.GetClass("NSView"), false),
		panS:    newSurface("MansurPanel", panelView, true),
		replies: make(chan reply, 1),
		upd:     make(chan string, 1),
		sig:     make(chan os.Signal, 1),
		keys:    map[string]objc.ID{},
		onQuit:  onQuit,
	}
	u.born = loadState(pet)
	if m, err := brain.Default(); err == nil {
		u.brain = m
	}
	signal.Notify(u.sig, syscall.SIGTERM, syscall.SIGINT)

	u.syncPet()
	u.petS.show()
	pet.say(pet.bond.greeting(time.Now()), 1)
	pet.bond.LastSeen = time.Now()

	u.startTimer()

	pool.Send(sel("drain"))
	app.Send(sel("run"))
}
