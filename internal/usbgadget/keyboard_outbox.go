package usbgadget

import (
	"errors"
	"io/fs"
	"time"
)

const (
	keyboardReportLen  = 8
	keyboardStaleAfter = 3 * time.Second
	keyboardOutboxMax  = 1024
)

type kbEntry struct {
	report [keyboardReportLen]byte
	done   chan error
	since  time.Time
}

// keyboardOutbox delivers keyboard reports strictly in order.
// Owned exclusively by the keyboard writer goroutine.
type keyboardOutbox struct {
	entries []kbEntry
	last    [keyboardReportLen]byte
	now     func() time.Time
}

func newKeyboardOutbox() *keyboardOutbox {
	return &keyboardOutbox{
		entries: make([]kbEntry, 0, keyboardOutboxMax),
		now:     time.Now,
	}
}

func (o *keyboardOutbox) add(msg hidMsg) {
	if msg.release {
		var zero [keyboardReportLen]byte
		if o.last != zero {
			o.append(zero, nil)
		}
		return
	}

	r := msg.data

	// Suppress exact consecutive duplicates if there's no waiting caller
	if n := len(o.entries); n > 0 && msg.done == nil {
		if last := &o.entries[n-1]; last.done == nil && last.report == r {
			return
		}
	}
	o.append(r, msg.done)
}

func (o *keyboardOutbox) append(r [keyboardReportLen]byte, done chan error) {
	now := o.now()
	if len(o.entries) >= keyboardOutboxMax {
		o.collapse()
	}
	o.entries = append(o.entries, kbEntry{report: r, done: done, since: now})
	o.last = r
}

func (o *keyboardOutbox) collapse() {
	n := len(o.entries)
	if n == 0 {
		return
	}
	for i := 0; i < n; i++ {
		notifyDone(o.entries[i].done, ErrReportStale)
	}
	o.entries = o.entries[:0]
	var zero [keyboardReportLen]byte
	if o.last != zero {
		o.entries = append(o.entries, kbEntry{report: zero, since: o.now()})
		o.last = zero
	}
}

func (o *keyboardOutbox) dropStale() {
	if len(o.entries) == 0 {
		return
	}
	now := o.now()
	firstFresh := 0
	for firstFresh < len(o.entries) && now.Sub(o.entries[firstFresh].since) >= keyboardStaleAfter {
		firstFresh++
	}

	switch {
	case firstFresh == 0:
		return
	case firstFresh == len(o.entries):
		var zero [keyboardReportLen]byte
		if o.last != zero {
			for i := range o.entries {
				notifyDone(o.entries[i].done, ErrReportStale)
			}
			o.entries = o.entries[:0]
			o.append(zero, nil)
			return
		}
		o.collapse()
	default:
		for i := 0; i < firstFresh; i++ {
			notifyDone(o.entries[i].done, ErrReportStale)
		}
		n := copy(o.entries, o.entries[firstFresh:])
		o.entries = o.entries[:n]
	}
}

func (o *keyboardOutbox) full() bool {
	return len(o.entries) >= keyboardOutboxMax
}

func (o *keyboardOutbox) pop(err error) {
	if len(o.entries) > 0 {
		notifyDone(o.entries[0].done, err)
		n := copy(o.entries, o.entries[1:])
		o.entries = o.entries[:n]
	}
}

func (o *keyboardOutbox) flush(u *UsbGadget) error {
	for len(o.entries) > 0 {
		e := &o.entries[0]
		err := u.rawKeyboardWrite(e.report[:])
		if err != nil {
			o.pop(err)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return err
		}
		o.pop(nil)
	}
	return nil
}
