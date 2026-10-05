package kvm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFrontendKeyboardInputNotBlockedByUsbState verifies that useKeyboard.ts
// does NOT drop keyboard events (like Ctrl+C) based on usbState.
// Dropping keystrokes when usbState is initializing or momentarily unread
// causes silent failure of terminal interrupt signals and keystroke drops.
func TestFrontendKeyboardInputNotBlockedByUsbState(t *testing.T) {
	kbHookPath := filepath.Join("ui", "src", "hooks", "useKeyboard.ts")
	data, err := os.ReadFile(kbHookPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", kbHookPath, err)
	}
	content := string(data)

	forbiddenPatterns := []string{
		`usbState !== "configured"`,
		`usbState !== 'configured'`,
		`usbState === "configured"`,
		`usbState === 'configured'`,
	}

	for _, pattern := range forbiddenPatterns {
		if strings.Contains(content, pattern) {
			t.Fatalf("REGRESSION DETECTED in %s: found forbidden condition %q. Keystroke propagation must not be gated by client-side usbState!", kbHookPath, pattern)
		}
	}
}

// TestFrontendPasteHandlerGuaranteesReleaseAndNotification verifies that
// usePasteHandler.ts:
// 1. Clears modifiers prior to typing to prevent stuck Ctrl/Shift keys.
// 2. Uses non-blocking sendNotification for keyboard reports during paste.
// 3. Guarantees an all-zero release report in a finally block.
func TestFrontendPasteHandlerGuaranteesReleaseAndNotification(t *testing.T) {
	pasteHookPath := filepath.Join("ui", "src", "layout", "core", "desktop", "hooks", "usePasteHandler.ts")
	data, err := os.ReadFile(pasteHookPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", pasteHookPath, err)
	}
	content := string(data)

	// Invariant 1: Modifiers cleared before paste
	if !strings.Contains(content, `sendNotification("keyboardReport", { keys: [], modifier: 0 })`) {
		t.Fatalf("REGRESSION DETECTED in %s: missing pre-paste modifier release or final release report!", pasteHookPath)
	}

	// Invariant 2: Finally block guarantees cleanup
	if !strings.Contains(content, "finally") {
		t.Fatalf("REGRESSION DETECTED in %s: missing finally block to guarantee release on paste error or abort!", pasteHookPath)
	}

	// Invariant 3: Paced typing must not use blocking send with callback
	if strings.Contains(content, `send("keyboardReport"`) {
		t.Fatalf("REGRESSION DETECTED in %s: paste loop must use sendNotification rather than blocking send to avoid 3-second round-trip fallback timeouts!", pasteHookPath)
	}
}

// TestFrontendClipboardGuaranteesRelease verifies that Clipboard.tsx guarantees
// release of keys in finally blocks and uses sendNotification.
func TestFrontendClipboardGuaranteesRelease(t *testing.T) {
	clipboardPath := filepath.Join("ui", "src", "layout", "components_side", "Clipboard", "Clipboard.tsx")
	data, err := os.ReadFile(clipboardPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", clipboardPath, err)
	}
	content := string(data)

	if !strings.Contains(content, "finally") {
		t.Fatalf("REGRESSION DETECTED in %s: missing finally block ensuring all keys are released!", clipboardPath)
	}
	if strings.Contains(content, `send("keyboardReport"`) {
		t.Fatalf("REGRESSION DETECTED in %s: must use sendNotification to avoid round-trip timeouts!", clipboardPath)
	}
}
