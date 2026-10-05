package kvm

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/pion/webrtc/v4"
)

// TestFastPathRPCResponsesWithID verifies that when a client sends an input event
// with an "id" field (like during remote paste with callback confirmation),
// the fast-path immediately returns a valid JSON-RPC 2.0 response with matching ID
// and does NOT drop the response, which would cause a 3-second UI hang.
func TestFastPathRPCResponsesWithID(t *testing.T) {
	var responses []JSONRPCResponse
	var mu sync.Mutex

	onJSONRPCResponseForTest = func(resp JSONRPCResponse) {
		mu.Lock()
		defer mu.Unlock()
		responses = append(responses, resp)
	}
	defer func() {
		onJSONRPCResponseForTest = nil
	}()

	dummySession := &Session{}

	testCases := []struct {
		name       string
		payload    string
		expectedID any
	}{
		{
			name:       "keyboardReport with integer ID",
			payload:    `{"jsonrpc":"2.0","method":"keyboardReport","params":{"modifier":0,"keys":[4]},"id":101}`,
			expectedID: float64(101),
		},
		{
			name:       "keyboardReport with string ID",
			payload:    `{"jsonrpc":"2.0","method":"keyboardReport","params":{"modifier":0,"keys":[]},"id":"paste-step-1"}`,
			expectedID: "paste-step-1",
		},
		{
			name:       "absMouseReport with ID",
			payload:    `{"jsonrpc":"2.0","method":"absMouseReport","params":{"x":100,"y":200,"buttons":1},"id":102}`,
			expectedID: float64(102),
		},
		{
			name:       "relMouseReport with ID",
			payload:    `{"jsonrpc":"2.0","method":"relMouseReport","params":{"dx":5,"dy":-10,"buttons":0},"id":103}`,
			expectedID: float64(103),
		},
		{
			name:       "wheelReport with ID",
			payload:    `{"jsonrpc":"2.0","method":"wheelReport","params":{"wheelY":1,"mouseMode":"abs"},"id":104}`,
			expectedID: float64(104),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mu.Lock()
			responses = nil
			mu.Unlock()

			msg := webrtc.DataChannelMessage{
				IsString: true,
				Data:     []byte(tc.payload),
			}
			onRPCMessage(msg, dummySession)

			mu.Lock()
			defer mu.Unlock()

			if len(responses) != 1 {
				t.Fatalf("expected 1 response for %s, got %d", tc.name, len(responses))
			}
			resp := responses[0]
			if resp.JSONRPC != "2.0" {
				t.Errorf("expected jsonrpc 2.0, got %v", resp.JSONRPC)
			}
			if resp.Result != true {
				t.Errorf("expected result true, got %v", resp.Result)
			}
			if resp.ID != tc.expectedID {
				t.Errorf("expected id %v, got %v", tc.expectedID, resp.ID)
			}
		})
	}
}

// TestFastPathRPCNotificationsWithoutID verifies that input event notifications
// (messages without an "id") do NOT trigger a response, maintaining zero-allocation
// high frequency throughput.
func TestFastPathRPCNotificationsWithoutID(t *testing.T) {
	var responses []JSONRPCResponse
	var mu sync.Mutex

	onJSONRPCResponseForTest = func(resp JSONRPCResponse) {
		mu.Lock()
		defer mu.Unlock()
		responses = append(responses, resp)
	}
	defer func() {
		onJSONRPCResponseForTest = nil
	}()

	dummySession := &Session{}

	notifications := []string{
		`{"jsonrpc":"2.0","method":"keyboardReport","params":{"modifier":0,"keys":[4]}}`,
		`{"jsonrpc":"2.0","method":"keyboardReport","params":{"modifier":0,"keys":[]}}`,
		`{"jsonrpc":"2.0","method":"absMouseReport","params":{"x":100,"y":200,"buttons":0}}`,
		`{"jsonrpc":"2.0","method":"relMouseReport","params":{"dx":1,"dy":2,"buttons":0}}`,
		`{"jsonrpc":"2.0","method":"wheelReport","params":{"wheelY":-1,"mouseMode":"relative"}}`,
	}

	for _, payload := range notifications {
		msg := webrtc.DataChannelMessage{
			IsString: true,
			Data:     []byte(payload),
		}
		onRPCMessage(msg, dummySession)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(responses) != 0 {
		t.Fatalf("expected 0 responses for notifications, got %d", len(responses))
	}
}

// TestGeneralRPCWorkerPoolDispatch tests normal JSON-RPC request-response cycles
// and notification handling through the worker pool.
func TestGeneralRPCWorkerPoolDispatch(t *testing.T) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "ping",
		ID:      float64(42),
	}
	reqData, _ := json.Marshal(req)

	var capturedResponse JSONRPCResponse
	var hasCaptured bool
	onJSONRPCResponseForTest = func(resp JSONRPCResponse) {
		capturedResponse = resp
		hasCaptured = true
	}
	defer func() {
		onJSONRPCResponseForTest = nil
	}()

	handleGeneralRPCRequest(webrtc.DataChannelMessage{IsString: true, Data: reqData}, &Session{})

	if !hasCaptured {
		t.Fatal("expected general RPC request to produce a response")
	}
	if capturedResponse.ID != float64(42) {
		t.Errorf("expected response ID 42, got %v", capturedResponse.ID)
	}
	if capturedResponse.Result != "pong" {
		t.Errorf("expected result 'pong', got %v", capturedResponse.Result)
	}
}
