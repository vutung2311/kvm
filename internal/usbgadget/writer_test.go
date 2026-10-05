package usbgadget

import (
	"errors"
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

func TestKeyboardOutboxCollapseEnsuresRelease(t *testing.T) {
	now := time.Now()
	clk := func() time.Time { return now }

	ob := &keyboardOutbox{now: clk}

	// Press key 'A'
	var m1 hidMsg
	m1.kind = hidMsgKeyboard
	m1.data = [8]byte{0, 0, 0x04, 0, 0, 0, 0, 0}
	ob.add(m1)

	// Simulate filling outbox to max
	for i := 1; i < keyboardOutboxMax; i++ {
		var m hidMsg
		m.kind = hidMsgKeyboard
		m.data = [8]byte{0, 0, byte(i % 50), 0, 0, 0, 0, 0}
		ob.add(m)
	}

	// Next add should trigger collapse
	var mOver hidMsg
	mOver.kind = hidMsgKeyboard
	mOver.data = [8]byte{0, 0, 0x05, 0, 0, 0, 0, 0}
	ob.add(mOver)

	// Collapse should have emitted an all-zero release report, plus the newly added report
	if len(ob.entries) != 2 {
		t.Fatalf("expected 2 entries after collapse, got %d", len(ob.entries))
	}
	var zero [keyboardReportLen]byte
	if ob.entries[0].report != zero {
		t.Fatalf("expected first entry after collapse to be zero release report, got %v", ob.entries[0].report)
	}
	if ob.entries[1].report[2] != 0x05 {
		t.Fatalf("expected second entry to be new report 0x05, got %v", ob.entries[1].report)
	}
}

func TestKeyboardOutboxStaleDrop(t *testing.T) {
	now := time.Now()
	clk := func() time.Time { return now }

	ob := &keyboardOutbox{now: clk}

	// Press key 'A'
	var m1 hidMsg
	m1.kind = hidMsgKeyboard
	m1.data = [8]byte{0, 0, 0x04, 0, 0, 0, 0, 0}
	ob.add(m1)

	// Advance time past keyboardStaleAfter
	now = now.Add(keyboardStaleAfter + time.Second)
	ob.dropStale()

	// Should drop stale report and replace with an all-zero release report
	if len(ob.entries) != 1 {
		t.Fatalf("expected 1 entry (all-zero release), got %d", len(ob.entries))
	}
	var zero [keyboardReportLen]byte
	if ob.entries[0].report != zero {
		t.Fatalf("expected zero release report on stale drop, got %v", ob.entries[0].report)
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

func BenchmarkMouseOutboxAddAbsolute(b *testing.B) {
	now := time.Now()
	ob := &mouseOutbox{
		entries: make([]mouseEntry, 0, mouseOutboxMax),
		now:     func() time.Time { return now },
	}
	var msg hidMsg
	msg.kind = hidMsgAbsMouse
	msg.data = [8]byte{1, 0, 10, 0, 20, 0, 0, 0}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ob.add(msg)
	}
}

func BenchmarkMouseOutboxAddRelative(b *testing.B) {
	now := time.Now()
	ob := &mouseOutbox{
		entries: make([]mouseEntry, 0, mouseOutboxMax),
		now:     func() time.Time { return now },
	}
	var msg hidMsg
	msg.kind = hidMsgRelMouse
	msg.data = [8]byte{0, 5, 10, 1, 0, 0, 0, 0}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ob.add(msg)
	}
}

func BenchmarkKeyboardOutboxAdd(b *testing.B) {
	now := time.Now()
	ob := &keyboardOutbox{
		entries: make([]kbEntry, 0, keyboardOutboxMax),
		now:     func() time.Time { return now },
	}
	var msg hidMsg
	msg.kind = hidMsgKeyboard
	msg.data = [8]byte{0, 0, 0x04, 0, 0, 0, 0, 0}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ob.add(msg)
	}
}

func TestKeyboardOutboxPopOnError(t *testing.T) {
	ob := newKeyboardOutbox()
	done := make(chan error, 1)
	ob.append([8]byte{0, 0, 4, 0, 0, 0, 0, 0}, done)
	if len(ob.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(ob.entries))
	}
	testErr := errors.New("write failed")
	ob.pop(testErr)
	if len(ob.entries) != 0 {
		t.Fatalf("expected 0 entries after pop, got %d", len(ob.entries))
	}
	select {
	case err := <-done:
		if err != testErr {
			t.Fatalf("expected testErr, got %v", err)
		}
	default:
		t.Fatal("expected done to be signaled with error")
	}
}

func TestMouseOutboxPopOnError(t *testing.T) {
	ob := newMouseOutbox()
	ob.appendEntry(mouseEntry{
		kind: mouseEntryAbs,
		x:    100,
		y:    200,
	}, time.Now())
	if len(ob.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(ob.entries))
	}
	ob.pop()
	if len(ob.entries) != 0 {
		t.Fatalf("expected 0 entries after pop, got %d", len(ob.entries))
	}
}
