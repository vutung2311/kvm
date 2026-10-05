package kvm

import (
	"testing"
)

func BenchmarkVideoBroadcasterBroadcastNoSubscribers(b *testing.B) {
	bc := &VideoBroadcaster{
		subscribers: make(map[string]chan *VideoFrame),
	}
	sampleData := make([]byte, 1024)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bc.Broadcast(sampleData)
	}
}

func BenchmarkVideoBroadcasterBroadcastWithSubscriber(b *testing.B) {
	bc := &VideoBroadcaster{
		subscribers: make(map[string]chan *VideoFrame),
	}
	id, ch := bc.Subscribe()
	defer bc.Unsubscribe(id)

	done := make(chan struct{})
	go func() {
		for {
			select {
			case f, ok := <-ch:
				if !ok {
					return
				}
				if f != nil {
					f.Release()
				}
			case <-done:
				return
			}
		}
	}()
	defer close(done)

	sampleData := make([]byte, 1024)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bc.Broadcast(sampleData)
	}
}
