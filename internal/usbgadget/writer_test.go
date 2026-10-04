package usbgadget

import (
	"testing"
	"time"
)

func TestMouseOutboxAbsoluteCoalesce(t *testing.T) {
	now := time.Now()
	clk := func() time.Time { return now }

	ob := &mouseOutbox{now: clk}

	// Send 3 mouse movements with same button (0)
	var m1, m2, m3 hidMsg
	m1.kind = hidMsgAbsMouse
	m1.data = [8]byte{1, 0, 10, 0, 20, 0, 0, 0} // (10, 20)

	m2.kind = hidMsgAbsMouse
	m2.data = [8]byte{1, 0, 50, 0, 60, 0, 0, 0} // (50, 60)

	m3.kind = hidMsgAbsMouse
	m3.data = [8]byte{1, 0, 100, 0, 120, 0, 0, 0} // (100, 120)

	ob.add(m1)
	ob.add(m2)
	ob.add(m3)

	// Since buttons are identical, all 3 should be coalesced into a single entry with latest coords
	if len(ob.entries) != 1 {
		t.Fatalf("expected 1 coalesced entry, got %d", len(ob.entries))
	}
	if ob.entries[0].x != 100 || ob.entries[0].y != 120 {
		t.Fatalf("expected (100, 120), got (%d, %d)", ob.entries[0].x, ob.entries[0].y)
	}

	// Now send a click (button 1)
	var mClick hidMsg
	mClick.kind = hidMsgAbsMouse
	mClick.data = [8]byte{1, 1, 100, 0, 120, 0, 0, 0}
	ob.add(mClick)

	// Button changed: must create a new entry to preserve click order!
	if len(ob.entries) != 2 {
		t.Fatalf("expected 2 entries after button press, got %d", len(ob.entries))
	}
	if ob.entries[1].buttons != 1 {
		t.Fatalf("expected button 1, got %d", ob.entries[1].buttons)
	}
}

func TestMouseOutboxRelativeCoalesce(t *testing.T) {
	now := time.Now()
	clk := func() time.Time { return now }

	ob := &mouseOutbox{now: clk}

	var m1, m2, m3 hidMsg
	m1.kind = hidMsgRelMouse
	m1.data = [8]byte{0, 5, 10, 1, 0, 0, 0, 0} // dx=5, dy=10, wheel=1

	m2.kind = hidMsgRelMouse
	m2.data = [8]byte{0, 15, 20, 2, 0, 0, 0, 0} // dx=15, dy=20, wheel=2

	m3.kind = hidMsgRelMouse
	m3.data = [8]byte{0, 30, 40, 0xFF, 0, 0, 0, 0} // dx=30, dy=40, wheel=-1 (0xFF as int8 is -1)

	ob.add(m1)
	ob.add(m2)
	ob.add(m3)

	if len(ob.entries) != 1 {
		t.Fatalf("expected 1 coalesced relative entry, got %d", len(ob.entries))
	}
	e := ob.entries[0]
	if e.dx != 50 || e.dy != 70 || e.wheel != 2 {
		t.Fatalf("expected dx=50, dy=70, wheel=2, got dx=%d, dy=%d, wheel=%d", e.dx, e.dy, e.wheel)
	}
}

func TestKeyboardOutboxDeduplication(t *testing.T) {
	now := time.Now()
	clk := func() time.Time { return now }

	ob := &keyboardOutbox{now: clk}

	// Press key 'A'
	var m1 hidMsg
	m1.kind = hidMsgKeyboard
	m1.data = [8]byte{0, 0, 0x04, 0, 0, 0, 0, 0}

	// Send duplicate of key 'A'
	var m2 hidMsg
	m2.kind = hidMsgKeyboard
	m2.data = [8]byte{0, 0, 0x04, 0, 0, 0, 0, 0}

	// Release key 'A'
	var m3 hidMsg
	m3.kind = hidMsgKeyboard
	m3.data = [8]byte{0, 0, 0, 0, 0, 0, 0, 0}

	ob.add(m1)
	ob.add(m2)
	ob.add(m3)

	// Duplicate m2 should be suppressed, so we only have m1 (press) and m3 (release)
	if len(ob.entries) != 2 {
		t.Fatalf("expected 2 entries (press, release), got %d", len(ob.entries))
	}
	if ob.entries[0].report[2] != 0x04 {
		t.Fatalf("expected key 0x04 in first entry")
	}
	if ob.entries[1].report[2] != 0x00 {
		t.Fatalf("expected key 0x00 in second entry")
	}
}

func TestMouseOutboxStaleDrop(t *testing.T) {
	now := time.Now()
	clk := func() time.Time { return now }

	ob := &mouseOutbox{now: clk}

	var m1 hidMsg
	m1.kind = hidMsgAbsMouse
	m1.data = [8]byte{1, 1, 10, 0, 20, 0, 0, 0} // button 1
	ob.add(m1)

	// Advance time by 2 seconds (> mouseStaleAfter)
	now = now.Add(2 * time.Second)

	ob.dropStale()

	// Should collapse to 1 state-sync entry with button 1 preserved
	if len(ob.entries) != 1 {
		t.Fatalf("expected 1 collapsed entry, got %d", len(ob.entries))
	}
	if ob.entries[0].buttons != 1 {
		t.Fatalf("expected buttons=1 preserved on stale collapse, got %d", ob.entries[0].buttons)
	}
}
