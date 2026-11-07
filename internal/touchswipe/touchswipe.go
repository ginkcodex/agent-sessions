// Package touchswipe spots a two-finger sideways swipe in the contacts a
// Precision Touchpad reports (Windows Raw Input, read in
// cmd/agent-sessions/mousenav_windows.go). Unlike the wheel events a page
// sees, the contacts say when the fingers touch and lift, so one touch, from
// the first finger down to the last one up, swipes once at most.
package touchswipe

import (
	"math"
	"time"
)

const (
	// SwipeDistanceMM is how far (mm) two fingers must move sideways to
	// swipe. Moving that far up or down first makes the touch a scroll.
	SwipeDistanceMM = 10
	// LostTouch ends a touch that sent no frame for this long: its lift was lost.
	LostTouch = time.Second
	// DefaultWidthMM is the touchpad's width when it reports no usable size.
	DefaultWidthMM = 100
)

// Direction is a swipe's step, or None.
type Direction int

const (
	None Direction = iota
	Back
	Forward
)

func (d Direction) String() string {
	switch d {
	case Back:
		return "back"
	case Forward:
		return "forward"
	}
	return "none"
}

// Contact is one finger in a report: its contact ID, whether it touches the
// pad (the tip switch), and where, in mm.
type Contact struct {
	ID   uint32
	Tip  bool
	X, Y float64
}

// Frames joins reports into frames. A frame lists every contact on the pad,
// with a lifting one once more without its tip. A report holds a few finger
// slots; in hybrid mode a frame spans several reports, and only the first
// carries the frame's contact count.
type Frames struct {
	want  int
	frame []Contact
}

// Report takes one report: its contact count (0 in a frame's later reports)
// and its finger slots in order. It returns the frame once it is complete;
// the frame is only valid until the next call. A count of 0 with no frame
// begun is an empty frame: nothing touches.
func (f *Frames) Report(count int, slots []Contact) ([]Contact, bool) {
	if count > 0 {
		f.want = count
		f.frame = f.frame[:0]
	} else if f.want == 0 {
		return f.frame[:0], true
	}
	n := min(f.want-len(f.frame), len(slots))
	f.frame = append(f.frame, slots[:n]...)
	if len(f.frame) < f.want {
		return nil, false
	}
	f.want = 0
	return f.frame, true
}

// Tracker turns one touchpad's frames into swipes.
type Tracker struct {
	last     time.Time
	touching bool
	// done: this touch swiped, or can no longer swipe.
	done bool
	// The two fingers down and their midpoint when they came down.
	pair    [2]uint32
	paired  bool
	originX float64
	originY float64
	// Fingers down in the last frame, and whether that frame changed it.
	lastDown   int
	downChange bool
}

// Frame takes the frame that came at time at, and returns the touch's
// swipe, once: two fingers, SwipeDistanceMM sideways, more than twice the
// vertical. Fingers moving right go back, as on macOS, whatever the scroll
// direction setting. Three or more fingers are a system gesture and never
// swipe.
func (t *Tracker) Frame(at time.Time, contacts []Contact) Direction {
	if t.touching && at.Sub(t.last) > LostTouch {
		t.end()
	}
	t.last = at
	var down [2]Contact
	n := 0
	for _, c := range contacts {
		if c.Tip {
			if n < len(down) {
				down[n] = c
			}
			n++
		}
	}
	t.downChange = n != t.lastDown
	t.lastDown = n
	if n == 0 {
		t.end()
		return None
	}
	t.touching = true
	if t.done {
		return None
	}
	if n >= 3 {
		t.done = true
		return None
	}
	if n == 1 {
		t.paired = false
		return None
	}
	a, b := down[0], down[1]
	pair := [2]uint32{min(a.ID, b.ID), max(a.ID, b.ID)}
	x, y := (a.X+b.X)/2, (a.Y+b.Y)/2
	if !t.paired || pair != t.pair {
		t.pair, t.paired, t.originX, t.originY = pair, true, x, y
		return None
	}
	dx, dy := x-t.originX, y-t.originY
	if math.Abs(dy) >= SwipeDistanceMM && math.Abs(dx) <= 2*math.Abs(dy) {
		t.done = true // a scroll up or down
		return None
	}
	if math.Abs(dx) < SwipeDistanceMM || math.Abs(dx) <= 2*math.Abs(dy) {
		return None
	}
	t.done = true
	if dx > 0 {
		return Back
	}
	return Forward
}

// Down is how many fingers the last frame had down, and whether that
// changed with it (for tracing).
func (t *Tracker) Down() (int, bool) { return t.lastDown, t.downChange }

func (t *Tracker) end() {
	t.touching, t.done, t.paired = false, false, false
}

// HIDAxis is a HID value's range and unit, as in HIDP_VALUE_CAPS.
type HIDAxis struct {
	LogicalMin, LogicalMax   int32
	PhysicalMin, PhysicalMax int32
	Units, UnitsExp          uint32
}

// HID length units: SI linear (cm) and English linear (inch).
const (
	unitCM   = 0x11
	unitInch = 0x13
)

// mmPerUnit is how many mm one logical unit is, or 0 if the axis does not
// say. Physical 0 to 0 means the physical range is the logical one.
func (h HIDAxis) mmPerUnit() float64 {
	logical := float64(h.LogicalMax) - float64(h.LogicalMin)
	if logical <= 0 {
		return 0
	}
	pmin, pmax := h.PhysicalMin, h.PhysicalMax
	if pmin == 0 && pmax == 0 {
		pmin, pmax = h.LogicalMin, h.LogicalMax
	}
	var mm float64
	switch h.Units {
	case unitCM:
		mm = 10
	case unitInch:
		mm = 25.4
	default:
		return 0
	}
	exp := int(h.UnitsExp & 0xF) // a 4-bit two's complement exponent
	if exp > 7 {
		exp -= 16
	}
	scale := (float64(pmax) - float64(pmin)) * mm * math.Pow10(exp) / logical
	// A pad 20 mm to 400 mm across is believable; anything else is a bad descriptor.
	if size := scale * logical; size < 20 || size > 400 {
		return 0
	}
	return scale
}

// Scales gives mm per logical unit on X and Y. A pad that reports no usable
// size is taken as DefaultWidthMM wide, with square units.
func Scales(x, y HIDAxis) (sx, sy float64) {
	sx, sy = x.mmPerUnit(), y.mmPerUnit()
	if sx <= 0 {
		sx = 1
		if w := float64(x.LogicalMax) - float64(x.LogicalMin); w > 0 {
			sx = DefaultWidthMM / w
		}
	}
	if sy <= 0 {
		sy = sx
	}
	return sx, sy
}
