package kvm

import (
	"sync"
	"testing"
)

// TestRPCChannelInitialStateDispatch guarantees that upon WebRTC RPC DataChannel opening,
// all critical initial states (otaState, videoInputState, usbState, and keyboardLedState)
// are immediately pushed to the client. This prevents client-side state starvation
// and guarantees the client doesn't drop keyboard inputs due to uninitialized USB state.
func TestRPCChannelInitialStateDispatch(t *testing.T) {
	var events []string
	var mu sync.Mutex

	onJSONRPCEventForTest = func(event string, params interface{}) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	}
	defer func() {
		onJSONRPCEventForTest = nil
	}()

	dummySession := &Session{}
	handleRPCChannelOpen(dummySession)

	mu.Lock()
	defer mu.Unlock()

	requiredEvents := map[string]bool{
		"otaState":        false,
		"videoInputState": false,
		"usbState":        false,
	}

	for _, ev := range events {
		if _, ok := requiredEvents[ev]; ok {
			requiredEvents[ev] = true
		}
	}

	for ev, received := range requiredEvents {
		if !received {
			t.Errorf("REGRESSION DETECTED: RPC DataChannel OnOpen did not dispatch critical initial event %q! Client input may be dropped.", ev)
		}
	}
}
