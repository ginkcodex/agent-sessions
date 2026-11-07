package touchswipe

import (
	"math"
	"testing"
	"time"
)

// touch drives a Tracker with frames 8 ms apart, as a touchpad at 125 Hz.
type touch struct {
	t   *testing.T
	tr  Tracker
	now time.Time
	got []Direction
}

func newTouch(t *testing.T) *touch { return &touch{t: t, now: time.Unix(1000, 0)} }

func (h *touch) frame(contacts ...Contact) {
	h.now = h.now.Add(8 * time.Millisecond)
	if d := h.tr.Frame(h.now, contacts); d != None {
		h.got = append(h.got, d)
	}
}

// two moves two fingers from (x, y) by (dx, dy) mm over steps frames.
func (h *touch) two(x, y, dx, dy float64, steps int) {
	for i := 0; i <= steps; i++ {
		f := float64(i) / float64(steps)
		h.frame(Contact{ID: 1, Tip: true, X: x + dx*f, Y: y + dy*f}, Contact{ID: 2, Tip: true, X: x + 20 + dx*f, Y: y + dy*f})
	}
}

func (h *touch) lift() {
	h.frame(Contact{ID: 1}, Contact{ID: 2})
}

func (h *touch) want(want ...Direction) {
	h.t.Helper()
	if len(h.got) != len(want) {
		h.t.Fatalf("swipes %v, want %v", h.got, want)
	}
	for i := range want {
		if h.got[i] != want[i] {
			h.t.Fatalf("swipes %v, want %v", h.got, want)
		}
	}
}

func TestOneSwipeIsOneStep(t *testing.T) {
	h := newTouch(t)
	h.two(30, 30, 40, 2, 30) // fingers right, far past the distance
	h.want(Back)
	h.two(70, 32, -60, 0, 30) // and back left without lifting
	h.lift()
	h.want(Back)

	h.two(70, 30, -15, 1, 10)
	h.lift()
	h.want(Back, Forward)

	// Momentum does not exist here: lifting and swiping again is the next step.
	h.two(30, 30, 15, 0, 5)
	h.lift()
	h.two(30, 30, 15, 0, 5)
	h.lift()
	h.want(Back, Forward, Back, Back)
}

func TestTouchesThatNeverSwipe(t *testing.T) {
	h := newTouch(t)
	h.two(30, 30, 9, 0, 10) // too short
	h.lift()
	h.two(30, 30, 12, 8, 10) // not twice the vertical
	h.lift()
	// A scroll down, then sideways in the same touch: still a scroll.
	h.two(30, 30, 0, 12, 10)
	h.two(30, 42, 30, -12, 10)
	h.lift()
	// One finger moves the pointer.
	for i := 0; i <= 10; i++ {
		h.frame(Contact{ID: 1, Tip: true, X: 30 + float64(i)*3, Y: 30})
	}
	h.frame(Contact{ID: 1})
	// Three fingers are the system's gesture, also when one lifts.
	for i := 0; i <= 10; i++ {
		x := 30 + float64(i)*3
		h.frame(Contact{ID: 1, Tip: true, X: x, Y: 30}, Contact{ID: 2, Tip: true, X: x + 15, Y: 30}, Contact{ID: 3, Tip: true, X: x + 30, Y: 30})
	}
	h.two(60, 30, 30, 0, 10)
	h.lift()
	h.want()
}

func TestTouchEnds(t *testing.T) {
	h := newTouch(t)
	// A second finger comes down late: the swipe counts from then.
	h.frame(Contact{ID: 1, Tip: true, X: 30, Y: 30})
	h.frame(Contact{ID: 1, Tip: true, X: 38, Y: 30})
	h.frame(Contact{ID: 1, Tip: true, X: 38, Y: 30}, Contact{ID: 2, Tip: true, X: 58, Y: 30})
	h.want()
	h.two(38, 30, 12, 0, 5)
	h.want(Back)
	// The lift was lost: a second later it is a new touch.
	h.now = h.now.Add(LostTouch)
	h.two(30, 30, 12, 0, 5)
	h.want(Back, Back)
	// An empty frame ends the touch too.
	h.frame()
	h.two(30, 30, -12, 0, 5)
	h.want(Back, Back, Forward)
}

func TestFramesJoinHybridReports(t *testing.T) {
	var f Frames
	a, b, c := Contact{ID: 1, Tip: true}, Contact{ID: 2, Tip: true}, Contact{ID: 3, Tip: true}
	// Parallel: the whole frame in one report; unused slots are ignored.
	if got, ok := f.Report(2, []Contact{a, b, c}); !ok || len(got) != 2 || got[1].ID != 2 {
		t.Fatalf("parallel frame %v %v", got, ok)
	}
	// Hybrid: three contacts over two reports of two slots.
	if _, ok := f.Report(3, []Contact{a, b}); ok {
		t.Fatal("frame complete after its first report")
	}
	if got, ok := f.Report(0, []Contact{c, a}); !ok || len(got) != 3 || got[2].ID != 3 {
		t.Fatalf("hybrid frame %v %v", got, ok)
	}
	// A new count drops a frame left unfinished.
	f.Report(3, []Contact{a})
	if got, ok := f.Report(1, []Contact{b}); !ok || len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("restarted frame %v %v", got, ok)
	}
	// A count of 0 with no frame begun: nothing touches.
	if got, ok := f.Report(0, []Contact{a}); !ok || len(got) != 0 {
		t.Fatalf("empty frame %v %v", got, ok)
	}
}

func TestScales(t *testing.T) {
	near := func(got, want float64) bool { return math.Abs(got-want) < 1e-9 }
	// The usual descriptor: 0..1200 over 0..1200 hundredths of a cm (exponent -2): 0.1 mm a unit.
	x := HIDAxis{LogicalMax: 1200, PhysicalMax: 1200, Units: unitCM, UnitsExp: 0xE}
	y := HIDAxis{LogicalMax: 700, PhysicalMax: 700, Units: unitCM, UnitsExp: 0xE}
	if sx, sy := Scales(x, y); !near(sx, 0.1) || !near(sy, 0.1) {
		t.Fatalf("cm scales %v %v", sx, sy)
	}
	// Inches, physical given as 0 to 0 (the logical range).
	x = HIDAxis{LogicalMax: 400, Units: unitInch, UnitsExp: 0xE} // 4 in
	if sx, _ := Scales(x, x); !near(sx, 0.254) {
		t.Fatalf("inch scale %v", sx)
	}
	// No unit, or a size no touchpad has: DefaultWidthMM across, square units.
	x = HIDAxis{LogicalMax: 2000}
	if sx, sy := Scales(x, HIDAxis{LogicalMax: 1000}); !near(sx, 0.05) || !near(sy, 0.05) {
		t.Fatalf("unitless scales %v %v", sx, sy)
	}
	x = HIDAxis{LogicalMax: 1000, PhysicalMax: 1000, Units: unitCM} // 10 m
	if sx, _ := Scales(x, x); !near(sx, 0.1) {
		t.Fatalf("absurd size scale %v", sx)
	}
}
