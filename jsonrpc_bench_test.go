package kvm

import (
	"testing"

	"github.com/pion/webrtc/v4"
	"github.com/rs/zerolog"
	"kvm/internal/usbgadget"
)

func init() {
	zerolog.SetGlobalLevel(zerolog.Disabled)
	gadget = usbgadget.NewUsbGadget("test", nil, nil, nil)
	gadget.InitTestInboxes()
}

func BenchmarkRPCWorkerPoolSubmit(b *testing.B) {
	task := rpcTask{
		message: webrtc.DataChannelMessage{Data: []byte(`{"jsonrpc":"2.0","method":"ping"}`)},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		globalRPCWorkerPool.submit(task)
	}
}

func BenchmarkOnRPCMessageAbsMouse(b *testing.B) {
	msg := webrtc.DataChannelMessage{
		IsString: true,
		Data:     []byte(`{"jsonrpc":"2.0","method":"absMouseReport","params":{"x":100,"y":200,"buttons":0}}`),
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		onRPCMessage(msg, nil)
	}
}

func BenchmarkOnRPCMessageRelMouse(b *testing.B) {
	msg := webrtc.DataChannelMessage{
		IsString: true,
		Data:     []byte(`{"jsonrpc":"2.0","method":"relMouseReport","params":{"dx":5,"dy":-10,"buttons":0}}`),
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		onRPCMessage(msg, nil)
	}
}

func BenchmarkOnRPCMessageWheel(b *testing.B) {
	msg := webrtc.DataChannelMessage{
		IsString: true,
		Data:     []byte(`{"jsonrpc":"2.0","method":"wheelReport","params":{"wheelY":1,"mouseMode":"abs"}}`),
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		onRPCMessage(msg, nil)
	}
}

func BenchmarkOnRPCMessageKeyboard(b *testing.B) {
	msg := webrtc.DataChannelMessage{
		IsString: true,
		Data:     []byte(`{"jsonrpc":"2.0","method":"keyboardReport","params":{"modifier":0,"keys":[4]}}`),
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		onRPCMessage(msg, nil)
	}
}

func TestOnRPCMessageKeyboardJSON(t *testing.T) {
	testCases := []string{
		`{"jsonrpc":"2.0","method":"keyboardReport","params":{"modifier":0,"keys":[4]}}`,
		`{"jsonrpc":"2.0","method":"keyboardReport","params":{"modifier":2,"keys":[4,5,6]}}`,
		`{"jsonrpc":"2.0","method":"keyboardReport","params":{"modifier":0,"keys":[]}}`,
		`{"jsonrpc":"2.0","method":"keyboardReport","params":{"keys":[],"modifier":0}}`,
	}
	for _, tc := range testCases {
		msg := webrtc.DataChannelMessage{
			IsString: true,
			Data:     []byte(tc),
		}
		onRPCMessage(msg, nil)
	}
}
