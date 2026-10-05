package kvm

import (
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
)

type FrameBuffer struct {
	data []byte
}

type VideoFrame struct {
	fb   *FrameBuffer
	data []byte
	refs atomic.Int32
}

func (f *VideoFrame) Data() []byte {
	return f.data
}

func (f *VideoFrame) Release() {
	if f.refs.Add(-1) == 0 {
		if f.fb != nil {
			framePool.Put(f.fb)
			f.fb = nil
		}
		f.data = nil
		videoFrameStructPool.Put(f)
	}
}

var framePool = sync.Pool{
	New: func() any {
		return &FrameBuffer{data: make([]byte, maxFrameSize)}
	},
}

var videoFrameStructPool = sync.Pool{
	New: func() any {
		return new(VideoFrame)
	},
}

type VideoBroadcaster struct {
	subscribers       map[string]chan *VideoFrame
	subscriberList    []chan *VideoFrame // cached flat slice, rebuilt on Subscribe/Unsubscribe
	count             atomic.Int32       // len(subscribers) as atomic for fast Broadcast check
	lock              sync.RWMutex
	onFirstSubscribe  func()
	onLastUnsubscribe func()
}

var videoBroadcaster = &VideoBroadcaster{
	subscribers: make(map[string]chan *VideoFrame),
}

func (b *VideoBroadcaster) rebuildList() {
	list := make([]chan *VideoFrame, 0, len(b.subscribers))
	for _, ch := range b.subscribers {
		list = append(list, ch)
	}
	b.subscriberList = list
}

func (b *VideoBroadcaster) Subscribe() (string, chan *VideoFrame) {
	b.lock.Lock()
	defer b.lock.Unlock()
	id := uuid.New().String()
	ch := make(chan *VideoFrame, 200)
	wasEmpty := len(b.subscribers) == 0
	b.subscribers[id] = ch
	b.rebuildList()
	b.count.Store(int32(len(b.subscribers)))
	if wasEmpty && b.onFirstSubscribe != nil {
		b.onFirstSubscribe()
	}
	return id, ch
}

func (b *VideoBroadcaster) Unsubscribe(id string) {
	b.lock.Lock()
	defer b.lock.Unlock()
	if ch, ok := b.subscribers[id]; ok {
		close(ch)
		delete(b.subscribers, id)
		b.rebuildList()
		b.count.Store(int32(len(b.subscribers)))
		if len(b.subscribers) == 0 && b.onLastUnsubscribe != nil {
			b.onLastUnsubscribe()
		}
	}
}

func (b *VideoBroadcaster) Broadcast(data []byte) {
	// atomic check avoids acquiring RLock on every video frame when no HTTP clients are connected
	if b.count.Load() == 0 {
		return
	}

	b.lock.RLock()
	subscribers := b.subscriberList
	subscriberCount := len(subscribers)
	if subscriberCount == 0 {
		b.lock.RUnlock()
		return
	}

	fb := framePool.Get().(*FrameBuffer)
	if cap(fb.data) < len(data) {
		fb.data = make([]byte, len(data))
	}
	n := copy(fb.data, data)

	frame := videoFrameStructPool.Get().(*VideoFrame)
	frame.fb = fb
	frame.data = fb.data[:n]
	frame.refs.Store(int32(subscriberCount + 1))

	for _, ch := range subscribers {
		select {
		case ch <- frame:
		default:
			frame.Release()
		}
	}
	b.lock.RUnlock()
	frame.Release()
}
