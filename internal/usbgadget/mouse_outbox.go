package usbgadget

import (
	"errors"
	"io/fs"
	"time"
)

const (
	mouseStaleAfter = 100 * time.Millisecond
	mouseOutboxMax  = 32
)

type mouseEntryKind uint8

const (
	mouseEntryAbs mouseEntryKind = iota
	mouseEntryRel
	mouseEntryAbsWheel
)

type mouseEntry struct {
	kind    mouseEntryKind
	buttons byte

	// Absolute coordinates
	x, y int

	// Relative deltas (can accumulate beyond ±127 before write)
	dx, dy, wheel int

	since time.Time
}

// mouseOutbox holds reports not yet accepted by the gadget.
// It is owned exclusively by the mouse writer goroutine, so it needs no mutex locks.
type mouseOutbox struct {
	entries []mouseEntry
	now     func() time.Time

	lastAbsButtons byte
	lastAbsX       int
	lastAbsY       int
	absKnown       bool

	relButtons byte
}

func newMouseOutbox() *mouseOutbox {
	return &mouseOutbox{
		entries: make([]mouseEntry, 0, mouseOutboxMax),
		now:     time.Now,
	}
}

func (o *mouseOutbox) add(msg hidMsg) {
	if msg.release {
		o.release()
		return
	}

	now := o.now()
	switch msg.kind {
	case hidMsgAbsMouse:
		buttons := msg.data[1]
		x := int(uint16(msg.data[2]) | (uint16(msg.data[3]) << 8))
		y := int(uint16(msg.data[4]) | (uint16(msg.data[5]) << 8))

		o.lastAbsButtons = buttons
		o.lastAbsX = x
		o.lastAbsY = y
		o.absKnown = true

		if n := len(o.entries); n > 0 {
			last := &o.entries[n-1]
			// If consecutive absolute reports have identical button state,
			// update coordinates in-place (latest position wins with zero lag).
			if last.kind == mouseEntryAbs && last.buttons == buttons {
				last.x = x
				last.y = y
				return
			}
		}

		o.appendEntry(mouseEntry{
			kind:    mouseEntryAbs,
			buttons: buttons,
			x:       x,
			y:       y,
			since:   now,
		}, now)

	case hidMsgAbsWheel:
		wheel := int(int8(msg.data[1]))
		if wheel == 0 {
			return
		}

		if n := len(o.entries); n > 0 {
			last := &o.entries[n-1]
			if last.kind == mouseEntryAbsWheel {
				last.wheel += wheel
				return
			}
		}

		o.appendEntry(mouseEntry{
			kind:  mouseEntryAbsWheel,
			wheel: wheel,
			since: now,
		}, now)

	case hidMsgRelMouse:
		buttons := msg.data[0]
		dx := int(int8(msg.data[1]))
		dy := int(int8(msg.data[2]))
		wheel := int(int8(msg.data[3]))
		o.relButtons = buttons

		if n := len(o.entries); n > 0 {
			last := &o.entries[n-1]
			// Relative movements with identical button state merge by summing deltas
			if last.kind == mouseEntryRel && last.buttons == buttons {
				last.dx += dx
				last.dy += dy
				last.wheel += wheel
				return
			}
		}

		o.appendEntry(mouseEntry{
			kind:    mouseEntryRel,
			buttons: buttons,
			dx:      dx,
			dy:      dy,
			wheel:   wheel,
			since:   now,
		}, now)
	}
}

func (o *mouseOutbox) release() {
	now := o.now()
	if o.relButtons != 0 {
		o.appendEntry(mouseEntry{
			kind:    mouseEntryRel,
			buttons: 0,
			since:   now,
		}, now)
		o.relButtons = 0
	}
	if o.absKnown && o.lastAbsButtons != 0 {
		o.appendEntry(mouseEntry{
			kind:    mouseEntryAbs,
			buttons: 0,
			x:       o.lastAbsX,
			y:       o.lastAbsY,
			since:   now,
		}, now)
		o.lastAbsButtons = 0
	}
}

func (o *mouseOutbox) appendEntry(e mouseEntry, now time.Time) {
	if len(o.entries) >= mouseOutboxMax {
		o.collapse(now)
	}
	o.entries = append(o.entries, e)
}

func (o *mouseOutbox) collapse(now time.Time) {
	if len(o.entries) == 0 {
		return
	}
	last := o.entries[len(o.entries)-1]
	last.dx = 0
	last.dy = 0
	last.wheel = 0
	last.since = now
	o.entries = append(o.entries[:0], last)
}

func (o *mouseOutbox) dropStale() {
	if len(o.entries) == 0 {
		return
	}
	now := o.now()
	firstFresh := 0
	for firstFresh < len(o.entries) && now.Sub(o.entries[firstFresh].since) >= mouseStaleAfter {
		firstFresh++
	}

	switch {
	case firstFresh == 0:
		return
	case firstFresh == len(o.entries):
		o.collapse(now)
	default:
		n := copy(o.entries, o.entries[firstFresh:])
		o.entries = o.entries[:n]
	}
}

func (o *mouseOutbox) full() bool {
	return len(o.entries) >= mouseOutboxMax-1
}

func (o *mouseOutbox) pop() {
	if len(o.entries) > 0 {
		n := copy(o.entries, o.entries[1:])
		o.entries = o.entries[:n]
	}
}

func (o *mouseOutbox) flush(u *UsbGadget) error {
	for len(o.entries) > 0 {
		e := &o.entries[0]
		var err error

		switch e.kind {
		case mouseEntryAbs:
			// Coalesce consecutive abs mouse entries with identical buttons
			for len(o.entries) > 1 && o.entries[1].kind == mouseEntryAbs && o.entries[1].buttons == e.buttons {
				o.pop()
				e = &o.entries[0]
			}
			var report [6]byte
			report[0] = 1 // Report ID 1
			report[1] = e.buttons
			report[2] = uint8(e.x)
			report[3] = uint8(e.x >> 8)
			report[4] = uint8(e.y)
			report[5] = uint8(e.y >> 8)
			err = u.absMouseWriteHidFile(report[:])

		case mouseEntryAbsWheel:
			var report [2]byte
			report[0] = 2 // Report ID 2
			report[1] = byte(clampInt8(e.wheel))
			err = u.absMouseWriteHidFile(report[:])

		case mouseEntryRel:
			err = u.flushRelativeEntry(e)
		}

		if err != nil {
			o.pop()
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return err
		}
		o.pop()
	}
	return nil
}

func (u *UsbGadget) flushRelativeEntry(e *mouseEntry) error {
	var report [4]byte
	report[0] = e.buttons

	if e.dx == 0 && e.dy == 0 && e.wheel == 0 {
		return u.relMouseWriteHidFile(report[:])
	}

	for e.dx != 0 || e.dy != 0 || e.wheel != 0 {
		cx := clampInt8(e.dx)
		cy := clampInt8(e.dy)
		cw := clampInt8(e.wheel)

		report[1] = byte(cx)
		report[2] = byte(cy)
		report[3] = byte(cw)

		if err := u.relMouseWriteHidFile(report[:]); err != nil {
			return err
		}

		e.dx -= int(cx)
		e.dy -= int(cy)
		e.wheel -= int(cw)
	}
	return nil
}
