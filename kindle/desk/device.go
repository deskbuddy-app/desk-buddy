package main

// The Kindle's hardware: the e-ink screen (through fbink), the touchscreen and the power button.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	fbinkBin = "/var/local/kmc/bin/fbink"
	touchDev = "/dev/input/event1"
	powerDev = "/dev/input/event0"

	evSyn, evKey, evAbs = 0, 1, 3
	absX, absY          = 0x00, 0x01
	absMTSlot           = 0x2f
	absMTX, absMTY      = 0x35, 0x36
	absMTTrack          = 0x39
	btnTouch            = 330
	keyPower            = 116

	eviocgrab    = 0x40044590 // _IOW('E', 0x90, int)
	eviocgabs    = 0x80184540 // _IOR('E', 0x40 + abs, struct input_absinfo)
	eviocgbitKey = 0x80204521 // _IOC(_IOC_READ, 'E', 0x20 + EV_KEY, 32)
	fbiogetVinfo = 0x4600     // FBIOGET_VSCREENINFO
)

// ---------- screen ----------

type Waveform int

const (
	Fast  Waveform = iota // DU: black/white only, quickest; for tap feedback
	Clean                 // partial, no flash: REAGL (low ghosting on the PW2), else GC16
	Flash                 // GC16 with a black/white flash, whole screen; clears ghosting
	Tidy                  // flash only the area that changed
)

type Screen struct {
	rot       int // 90: top of the picture is the Kindle's right-hand edge; 270: its left-hand edge
	fullEvery int
	shown     *image.Gray // landscape picture currently on the panel
	partials  int
	tmp       string
	Dark      bool  // white on black ghosts much more, so flash more readily
	Quiet     bool  // never flash by itself (the standby clock: digits barely ghost)
	Splits    []int // see View.Splits
	Level     int   // Settings › Screen refresh: 0 gentle, 1 normal, 2 clean (how readily it flashes)
	SplitY    []int
	afterTap  bool // a Fast (tap feedback) update is on the panel and needs cleaning
	fresh     bool // Forget was called: the next Show redraws the whole picture
	noREAGL   bool // fbink refused REAGL once; use GC16
}

// PW × PH: the layout, portrait (as the panel sees it): touches are mapped into it. The
// panel's pixels are ch × cw (gfx.go); on a Paperwhite 2 both are 758 × 1024.
var PW, PH = 758, 1024

func NewScreen(rot, fullEvery int) *Screen {
	return &Screen{rot: rot, fullEvery: fullEvery, tmp: "/tmp/desk-frame.png"}
}

// Show puts a landscape picture on the panel, redrawing only the areas that changed.
// Separate changes (a tapped tile and a status line, say) are redrawn separately, so
// whatever lies between them doesn't flash.
func (s *Screen) Show(img *image.Gray, w Waveform) {
	if s.shown == nil {
		w = Flash // a fresh start (or Refresh): always a proper clean-up
	} else if s.fresh || w == Flash {
		s.fresh = false
		if s.Level == 0 { // gentle: the whole picture again, without flashing
			s.partials = 0
			s.push(img, img.Rect, Clean)
			s.remember(img)
			return
		}
		w = Flash
	}
	if w == Flash {
		s.partials = 0
		s.afterTap = false
		s.push(img, img.Rect, Flash)
		s.remember(img)
		return
	}
	regions := changed(s.shown, img, pxLines(s.Splits), pxLines(s.SplitY))
	if len(regions) == 0 {
		return
	}
	cleanUp := false // flash each changed area rather than redraw it gently
	switch w {
	case Tidy:
		cleanUp = s.Level == 2 // only Clean flashes the bits that changed
	case Clean:
		s.partials++
		// redraws between full clean-ups: gentle 3×, normal 1.5×, clean 1× full_refresh_every
		every := s.fullEvery * []int{6, 3, 2}[s.Level] / 2
		if s.Dark {
			every = max(1, every/2)
		}
		if s.partials >= every && !s.Quiet { // regular full clean-up
			s.Show(img, Flash)
			return
		}
		cleanUp = s.afterTap && s.Dark && s.Level == 2 // tidying up after a tap on black
	}
	for _, r := range regions {
		rw := w
		big := r.Dx()*r.Dy() > img.Rect.Dx()*img.Rect.Dy()/4 && !s.Quiet && s.Level > 0 // gentle: never
		if w != Fast && (cleanUp || big) {
			rw = Flash // flashes just r
		} else if w == Tidy {
			rw = Clean
		}
		s.push(img, r, rw)
	}
	s.afterTap = w == Fast
	s.remember(img)
}

func (s *Screen) remember(img *image.Gray) {
	if s.shown == nil {
		s.shown = image.NewGray(img.Rect)
	}
	copy(s.shown.Pix, img.Pix)
}

// Forget makes the next Show a full flashing redraw (after something else drew on the panel).
// Every flash goes through the Screen refresh level (Settings › Screen):
//
//	Forget     a whole new picture: Normal / Clean flash it, Gentle redraws it without
//	DeepClean  wipe to white first (black backgrounds): only Clean; the others as Forget
//	ForceFlash a proper clean-up whatever the level (the Refresh button, a fresh start)
func (s *Screen) Forget() { s.fresh = true }

func (s *Screen) DeepClean() {
	if s.Level == 2 {
		exec.Command(fbinkBin, "-q", "-c", "-f", "-W", "GC16").Run()
		s.shown = nil
		return
	}
	s.fresh = true
}

func (s *Screen) ForceFlash() { s.shown = nil }

// pxLines: layout positions (View.Splits) in canvas pixels.
func pxLines(ls []int) []int {
	if sc == 1 {
		return ls
	}
	out := make([]int, len(ls))
	for i, v := range ls {
		out[i] = px(v, sc)
	}
	return out
}

// changed finds the areas that differ between two pictures: the screen is cut into
// 32-unit blocks (32 pixels on a Paperwhite 2), and touching (or nearly touching) changed
// blocks form one area.
func changed(a, b *image.Gray, splits, splitY []int) []image.Rectangle {
	bs := px(32, sc)
	sideOf := func(i int, lines []int) int { // which side of the lines block i (a column or row) is on
		n := 0
		for _, p := range lines {
			if i*bs+bs/2 >= p {
				n++
			}
		}
		return n
	}
	side := func(col int) int { return sideOf(col, splits) }
	band := func(row int) int { return sideOf(row, splitY) }
	gw, gh := (b.Rect.Dx()+bs-1)/bs, (b.Rect.Dy()+bs-1)/bs
	dirty := make([]bool, gw*gh)
	any := false
	for y := 0; y < b.Rect.Dy(); y++ {
		ra, rb := a.Pix[y*a.Stride:y*a.Stride+a.Rect.Dx()], b.Pix[y*b.Stride:y*b.Stride+b.Rect.Dx()]
		if bytes.Equal(ra, rb) {
			continue
		}
		for x := 0; x < len(rb); x++ {
			if ra[x] != rb[x] {
				dirty[(y/bs)*gw+x/bs] = true
				any = true
				x = (x/bs+1)*bs - 1 // this block is dirty; skip to the next one
			}
		}
	}
	if !any {
		return nil
	}
	// Group dirty blocks; blocks one apart count as touching.
	seen := make([]bool, len(dirty))
	var out []image.Rectangle
	for i, d := range dirty {
		if !d || seen[i] {
			continue
		}
		r := image.Rectangle{}
		stack := []int{i}
		seen[i] = true
		for len(stack) > 0 {
			c := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			cx, cy := c%gw, c/gw
			r = r.Union(image.Rect(cx*bs, cy*bs, (cx+1)*bs, (cy+1)*bs))
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					nx, ny := cx+dx, cy+dy
					if nx < 0 || ny < 0 || nx >= gw || ny >= gh || side(nx) != side(cx) || band(ny) != band(cy) {
						continue
					}
					n := ny*gw + nx
					if dirty[n] && !seen[n] {
						seen[n] = true
						stack = append(stack, n)
					}
				}
			}
		}
		out = append(out, r.Intersect(b.Rect))
	}
	if len(out) > 6 { // lots of little changes: one redraw is quicker than many
		u := image.Rectangle{}
		for _, r := range out {
			u = u.Union(r)
		}
		out = []image.Rectangle{u}
	}
	return out
}

// toPortrait maps a landscape rectangle of canvas pixels to the panel's portrait pixels.
func (s *Screen) toPortrait(r image.Rectangle) image.Rectangle {
	if s.rot == 270 {
		return image.Rect(r.Min.Y, cw-r.Max.X, r.Max.Y, cw-r.Min.X)
	}
	return image.Rect(ch-r.Max.Y, r.Min.X, ch-r.Min.Y, r.Max.X)
}

// toLandscape maps a touch point on the portrait panel to the landscape picture.
func (s *Screen) toLandscape(px, py int) (int, int) {
	if s.rot == 270 {
		return PH - 1 - py, px
	}
	return py, PW - 1 - px
}

// isBasicKindle is true on the older Basic Kindles (600 × 800, set in setPanel), so Compact
// mode is on by default on first run. The 2022/2024 Basics share 1072 × 1448 with the
// Paperwhite 3/4 and Voyage, so there it stays a choice in Settings › Screen.
var isBasicKindle bool

// panelSize asks FBInk and the Linux framebuffer (/dev/fb0) for the panel's size in pixels,
// portrait (a Paperwhite 2: 758 × 1024); the Paperwhite 2's when it can't tell.
func panelSize() (int, int) {
	b, _ := exec.Command(fbinkBin, "-e").CombinedOutput()
	w, h := parseFBInkEnv(string(b))
	if w == PW && h == PH && !strings.Contains(string(b), "758") {
		if fw, fh := fb0Size(); fw >= 580 && fh >= 780 {
			w, h = fw, fh
			log.Printf("panel (from fb0) %d × %d", w, h)
		}
	}
	return w, h
}

func fb0Size() (int, int) {
	if f, err := os.Open("/dev/fb0"); err == nil {
		var vinfo [40]uint32
		_, _, e := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), fbiogetVinfo, uintptr(unsafe.Pointer(&vinfo[0])))
		f.Close()
		if e == 0 {
			w, h := int(vinfo[0]), int(vinfo[1])
			if w > h {
				w, h = h, w
			}
			if w >= 580 && h >= 780 {
				return w, h
			}
		}
	}
	if b, err := os.ReadFile("/sys/class/graphics/fb0/modes"); err == nil {
		// e.g. "U:1072x1448p-0\n"
		s := string(b)
		if _, after, ok := strings.Cut(s, ":"); ok {
			s = after
		}
		var w, h int
		if n, _ := fmt.Sscanf(s, "%dx%d", &w, &h); n == 2 {
			if w > h {
				w, h = h, w
			}
			if w >= 580 && h >= 780 {
				return w, h
			}
		}
	}
	if b, err := os.ReadFile("/sys/class/graphics/fb0/virtual_size"); err == nil {
		var w, h int
		if n, _ := fmt.Sscanf(strings.TrimSpace(string(b)), "%d,%d", &w, &h); n == 2 {
			if w > h {
				w, h = h, w
			}
			if h >= w*2 {
				h /= 2
			}
			if w >= 580 && h >= 780 {
				return w, h
			}
		}
	}
	return 0, 0
}

func parseFBInkEnv(env string) (int, int) {
	var w, h, sw, sh int
	for _, kv := range strings.FieldsFunc(env, func(r rune) bool { return r == ';' || r == ' ' || r == '\n' || r == '\r' || r == ',' }) {
		if v, ok := strings.CutPrefix(kv, "viewWidth="); ok {
			w, _ = strconv.Atoi(strings.Trim(v, "'\""))
		}
		if v, ok := strings.CutPrefix(kv, "viewHeight="); ok {
			h, _ = strconv.Atoi(strings.Trim(v, "'\""))
		}
		if v, ok := strings.CutPrefix(kv, "screenWidth="); ok {
			sw, _ = strconv.Atoi(strings.Trim(v, "'\""))
		}
		if v, ok := strings.CutPrefix(kv, "screenHeight="); ok {
			sh, _ = strconv.Atoi(strings.Trim(v, "'\""))
		}
	}
	if w == 0 || h == 0 {
		w, h = sw, sh
	}
	if w > h { // FBInk tells the way the panel is turned now; we want portrait
		w, h = h, w
	}
	if w < 580 || h < 780 {
		return PW, PH
	}
	log.Printf("panel %d × %d", w, h)
	return w, h
}

// pushHook, when set (tests), sees each update instead of the panel.
var pushHook func(r image.Rectangle, w Waveform)

func (s *Screen) push(img *image.Gray, r image.Rectangle, w Waveform) {
	if pushHook != nil {
		pushHook(r, w)
		return
	}
	pr := s.toPortrait(r)
	out := image.NewGray(image.Rect(0, 0, pr.Dx(), pr.Dy()))
	for ly := r.Min.Y; ly < r.Max.Y; ly++ {
		row := img.Pix[ly*img.Stride:]
		for lx := r.Min.X; lx < r.Max.X; lx++ {
			var px, py int
			if s.rot == 270 {
				px, py = ly, cw-1-lx
			} else {
				px, py = ch-1-ly, lx
			}
			out.Pix[(py-pr.Min.Y)*out.Stride+(px-pr.Min.X)] = row[lx]
		}
	}
	f, err := os.Create(s.tmp)
	if err != nil {
		log.Println("frame:", err)
		return
	}
	enc := png.Encoder{CompressionLevel: png.NoCompression}
	err = enc.Encode(f, out)
	f.Close()
	if err != nil {
		log.Println("frame:", err)
		return
	}
	args := []string{"-q", "-g", fmt.Sprintf("file=%s,x=%d,y=%d", s.tmp, pr.Min.X, pr.Min.Y)}
	switch w {
	case Fast:
		args = append(args, "-W", "DU")
	case Clean:
		if s.noREAGL {
			args = append(args, "-W", "GC16")
		} else {
			args = append(args, "-W", "REAGL")
		}
	case Flash:
		args = append(args, "-W", "GC16", "-f")
	}
	if out, err := exec.Command(fbinkBin, args...).CombinedOutput(); err != nil {
		log.Printf("fbink %v: %v %s", args, err, out)
		if w == Clean && !s.noREAGL {
			s.noREAGL = true // this fbink or panel doesn't do REAGL; redo it with GC16
			s.push(img, r, w)
		}
	}
}

// battery asks the Kindle's power service for the charge level (0–100) and whether it's
// charging. Returns -1 when it can't tell (e.g. on the Mac).
func battery() (int, bool) {
	get := func(prop string) (int, error) {
		out, err := exec.Command("lipc-get-prop", "-i", "com.lab126.powerd", prop).Output()
		if err != nil {
			return 0, err
		}
		return strconv.Atoi(strings.TrimSpace(string(out)))
	}
	level, err := get("battLevel")
	if err != nil || level < 0 || level > 100 {
		return -1, false
	}
	charging, _ := get("isCharging")
	return level, charging == 1
}

// Front light: the Kindle's own levels are 0 (off) to 24.
var lightLevels = []int{0, 4, 8, 12, 18, 24}

func frontLight() int {
	out, err := exec.Command("lipc-get-prop", "-i", "com.lab126.powerd", "flIntensity").Output()
	if err != nil {
		return -1
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return -1
	}
	return n
}

func setFrontLight(n int) {
	if err := exec.Command("lipc-set-prop", "-i", "com.lab126.powerd", "flIntensity", strconv.Itoa(n)).Run(); err != nil {
		log.Println("front light:", err)
	}
}

func clearScreen() { exec.Command(fbinkBin, "-q", "-c", "-f").Run() }

// pauseKindleUI freezes the Kindle's own UI processes (both older cvm and newer 2022/2024
// kppMainApp / mesquite / webreader) so they cannot draw over Desk Buddy.
func pauseKindleUI() {
	if _, err := os.Stat("/mnt/us"); err != nil {
		return
	}
	exec.Command("lipc-set-prop", "com.lab126.pillow", "disableEnablePillow", "disable").Run()
	exec.Command("killall", "-STOP", "cvm", "kppMainApp", "mesquite", "webreader").Run()
}

// resumeKindleUI wakes the Kindle's own UI processes when leaving Desk Buddy.
func resumeKindleUI() {
	if _, err := os.Stat("/mnt/us"); err != nil {
		return
	}
	exec.Command("killall", "-CONT", "cvm", "kppMainApp", "mesquite", "webreader").Run()
	exec.Command("lipc-set-prop", "com.lab126.pillow", "disableEnablePillow", "enable").Run()
}

// ---------- input ----------

type Tap struct {
	X, Y     int // portrait panel pixels, where the finger went down
	UpX, UpY int // and where it lifted (for swipes)
	Hold     time.Duration
	Fingers  int // most fingers on the screen at once during this touch (several: a tab swipe)
}

// touchFingers is how many fingers the touchscreen can follow at once (from the driver).
var touchFingers int

type absinfo struct{ Value, Min, Max, Fuzz, Flat, Res int32 }

func getAbs(fd uintptr, code int) (absinfo, bool) {
	var a absinfo
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(eviocgabs)+uintptr(code), uintptr(unsafe.Pointer(&a)))
	return a, e == 0 && a.Max > a.Min
}

func scale(v int32, a absinfo, size int) int {
	return int(int64(v-a.Min) * int64(size-1) / int64(a.Max-a.Min))
}

// openGrabbed opens an input device and takes it for ourselves, so the Kindle's own UI
// doesn't also react. The grab ends by itself when this process exits, however it exits.
func openGrabbed(path string) (*os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), eviocgrab, 1); e != 0 {
		log.Printf("grab %s: %v", path, e)
	}
	return f, nil
}

type inputEvent struct {
	typ, code uint16
	val       int32
}

func readEvents(f *os.File, out func(inputEvent)) error {
	buf := make([]byte, 16) // 32-bit struct input_event: timeval(8) + type(2) + code(2) + value(4)
	for {
		if _, err := f.Read(buf); err != nil {
			return err
		}
		out(inputEvent{binary.LittleEndian.Uint16(buf[8:]), binary.LittleEndian.Uint16(buf[10:]), int32(binary.LittleEndian.Uint32(buf[12:]))})
	}
}

func findTouchDev() string {
	for i := 0; i < 10; i++ {
		p := fmt.Sprintf("/dev/input/event%d", i)
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		_, okMT := getAbs(f.Fd(), absMTX)
		_, okX := getAbs(f.Fd(), absX)
		f.Close()
		if okMT || okX {
			return p
		}
	}
	return touchDev
}

func hasPowerKey(fd uintptr) bool {
	var bits [32]byte
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(eviocgbitKey), uintptr(unsafe.Pointer(&bits[0])))
	if e != 0 {
		return false
	}
	return bits[keyPower/8]&(1<<(keyPower%8)) != 0
}

// readTouch sends one Tap per lift of the first finger, with how many fingers were down
// at most meanwhile (the others' positions are ignored).
func readTouch(taps chan<- Tap) {
	dev := findTouchDev()
	f, err := openGrabbed(dev)
	if err != nil {
		log.Println("touch:", err)
		return
	}
	ax, okx := getAbs(f.Fd(), absMTX)
	ay, oky := getAbs(f.Fd(), absMTY)
	if !okx || !oky {
		ax, _ = getAbs(f.Fd(), absX)
		ay, _ = getAbs(f.Fd(), absY)
	}
	log.Printf("touch (%s) ranges x %+v y %+v", dev, ax, ay)
	if sl, ok := getAbs(f.Fd(), absMTSlot); ok {
		touchFingers = int(sl.Max) + 1
	}
	log.Printf("touch: up to %d fingers", touchFingers)
	active := map[int32]bool{} // slots with a finger on the screen
	most := 0

	var x, y int32 = -1, -1
	slot := int32(0)
	down, placed := false, false
	var start time.Time
	var dx, dy int
	lift := func() {
		if down {
			down = false
			if placed {
				ux, uy := dx, dy
				if x >= 0 && y >= 0 {
					ux, uy = scale(x, ax, PW), scale(y, ay, PH)
				}
				taps <- Tap{X: dx, Y: dy, UpX: ux, UpY: uy, Hold: time.Since(start), Fingers: max(1, most)}
			}
		}
	}
	err = readEvents(f, func(e inputEvent) {
		switch {
		case e.typ == evAbs && e.code == absMTSlot:
			slot = e.val
		case e.typ == evAbs && e.code == absMTTrack && slot != 0: // another finger down or up
			if e.val >= 0 {
				active[slot] = true
				most = max(most, len(active))
			} else {
				delete(active, slot)
			}
		case slot != 0:
			// another finger's position: not used
		case e.typ == evAbs && (e.code == absMTX || e.code == absX):
			x = e.val
		case e.typ == evAbs && (e.code == absMTY || e.code == absY):
			y = e.val
		case e.typ == evAbs && e.code == absMTTrack:
			if e.val >= 0 {
				active[0] = true
				if len(active) == 1 { // a new touch: count its fingers from here
					most = 0
				}
				most = max(most, len(active))
				down, placed, start = true, false, time.Now()
			} else {
				lift()
				delete(active, 0)
			}
		case e.typ == evKey && e.code == btnTouch:
			if e.val == 1 && !down {
				down, placed, start = true, false, time.Now()
			} else if e.val == 0 {
				lift()
			}
		case e.typ == evSyn && down && !placed && x >= 0 && y >= 0:
			dx, dy, placed = scale(x, ax, PW), scale(y, ay, PH), true
		}
	})
	log.Println("touch:", err)
}

// readPower signals each press of the power button across all input devices that report KEY_POWER.
func readPower(press chan<- struct{}) {
	tdev := findTouchDev()
	var devs []string
	for i := 0; i < 10; i++ {
		p := fmt.Sprintf("/dev/input/event%d", i)
		if p == tdev {
			continue
		}
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		ok := hasPowerKey(f.Fd())
		f.Close()
		if ok {
			devs = append(devs, p)
		}
	}
	if len(devs) == 0 {
		devs = []string{powerDev}
	}
	for _, dev := range devs {
		go func(d string) {
			f, err := openGrabbed(d)
			if err != nil {
				log.Println("power button:", err)
				return
			}
			log.Printf("power button: listening on %s", d)
			err = readEvents(f, func(e inputEvent) {
				if e.typ == evKey && e.code == keyPower && e.val == 1 {
					select {
					case press <- struct{}{}:
					default:
					}
				}
			})
			log.Println("power button:", err)
		}(dev)
	}
}

// syncFS flushes writes to the Kindle's storage (after installing an update).
func syncFS() { syscall.Sync() }
