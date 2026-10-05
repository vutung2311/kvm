package usbgadget

import (
	"errors"
	"time"
)

// Input delivery model:
//
// Every producer submits reports to a per-device inbox. One persistent writer
// goroutine per device drains its inbox into an outbox and delivers to /dev/hidgN:
//
// - Submitting never waits on the host: the writer keeps draining the inbox
//   even while a delivery is being retried, so the network reader never stalls.
// - Healthy path adds no latency and loses nothing: the host polls about
//   every 1 ms; coalescing only happens when a backlog already exists.
// - If the host stops polling (sleep, BIOS, hung), the pending report is
//   retried with backoff instead of crashing or freezing the server.
// - Reports that waited longer than the device's stale window are dropped,
//   and the latest state is always delivered (no stuck keys or ghost clicks).

const (
	inboxSize = 1024

	writerRetryMin = 10 * time.Millisecond
	writerRetryMax = 100 * time.Millisecond
)

var (
	// ErrReportStale means the report waited longer than the stale window and
	// was superseded by newer state.
	ErrReportStale = errors.New("hid report dropped: host not accepting reports")
	// ErrReportTimeout means the caller stopped waiting; the report may still
	// be delivered later.
	ErrReportTimeout = errors.New("hid report not accepted in time")
)

type hidMsgKind uint8

const (
	hidMsgKeyboard hidMsgKind = iota
	hidMsgAbsMouse
	hidMsgRelMouse
	hidMsgAbsWheel
)

// hidMsg is one inbox item. Fixed [8]byte array ensures zero heap allocations on queue.
type hidMsg struct {
	kind    hidMsgKind
	data    [8]byte
	length  uint8
	release bool
	done    chan error
}

func notifyDone(done chan error, err error) {
	if done != nil {
		select {
		case done <- err:
		default:
		}
	}
}

// outbox is a device-specific buffer of not-yet-delivered reports.
type outbox interface {
	add(msg hidMsg)
	dropStale()
	flush(g *UsbGadget) error
	full() bool
}

func (u *UsbGadget) startWriters() {
	u.writersOnce.Do(func() {
		u.kbInbox = make(chan hidMsg, inboxSize)
		u.mouseInbox = make(chan hidMsg, inboxSize)
		go u.runWriter(u.kbInbox, newKeyboardOutbox())
		go u.runWriter(u.mouseInbox, newMouseOutbox())
	})
}

// InitTestInboxes initializes inboxes with a no-op drainer for benchmarks and unit tests.
func (u *UsbGadget) InitTestInboxes() {
	u.writersOnce.Do(func() {
		u.kbInbox = make(chan hidMsg, inboxSize)
		u.mouseInbox = make(chan hidMsg, inboxSize)
		go func() {
			for range u.kbInbox {
			}
		}()
		go func() {
			for range u.mouseInbox {
			}
		}()
	})
}

// runWriter is the per-device delivery loop. It returns when inbox is closed.
func (u *UsbGadget) runWriter(inbox <-chan hidMsg, ob outbox) {
	backoff := writerRetryMin
	var retryC <-chan time.Time

	for {
		if retryC == nil {
			msg, ok := <-inbox
			if !ok {
				return
			}
			ob.add(msg)
		} else {
			var in <-chan hidMsg
			if !ob.full() {
				in = inbox
			}
			select {
			case msg, ok := <-in:
				if !ok {
					_ = ob.flush(u)
					return
				}
				ob.add(msg)
				continue
			case <-retryC:
				retryC = nil
			}
		}

		// Coalesce what is already queued before writing to the device
		closed := false
	drain:
		for !ob.full() {
			select {
			case msg, ok := <-inbox:
				if !ok {
					closed = true
					break drain
				}
				ob.add(msg)
			default:
				break drain
			}
		}

		ob.dropStale()
		if err := ob.flush(u); err != nil {
			retryC = time.After(backoff)
			if backoff*2 < writerRetryMax {
				backoff *= 2
			} else {
				backoff = writerRetryMax
			}
		} else {
			backoff = writerRetryMin
		}

		if closed {
			return
		}
	}
}

func clampInt8(val int) int8 {
	if val < -127 {
		return -127
	}
	if val > 127 {
		return 127
	}
	return int8(val)
}
