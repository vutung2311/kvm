package usbgadget

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRebindUsbSequence verifies that RebindUsb uses strictly ConfigFS UDC unbind/bind
// with the correct dependency order, and never touches platform driver unbind.
func TestRebindUsbSequence(t *testing.T) {
	tx := &UsbGadgetTransaction{
		c:             &ChangeSet{},
		udc:           "ff800000.usb",
		kvmGadgetPath: "/sys/kernel/config/usb_gadget/kvm",
	}

	tx.RebindUsb(false)

	if len(tx.c.Changes) != 2 {
		t.Fatalf("expected exactly 2 changes for RebindUsb, got %d", len(tx.c.Changes))
	}

	// Step 1: Unbind from ConfigFS UDC (writing empty string)
	unbind := tx.c.Changes[0]
	if unbind.Key != "udc-unbind" {
		t.Errorf("expected first change key to be 'udc-unbind', got %q", unbind.Key)
	}
	expectedUdcPath := "/sys/kernel/config/usb_gadget/kvm/UDC"
	if unbind.Path != expectedUdcPath {
		t.Errorf("expected unbind path %q, got %q", expectedUdcPath, unbind.Path)
	}
	if string(unbind.ExpectedContent) != "" {
		t.Errorf("expected unbind to write empty string, got %q", string(unbind.ExpectedContent))
	}
	if unbind.ExpectedState != FileStateFileWrite {
		t.Errorf("expected unbind FileStateFileWrite, got %v", unbind.ExpectedState)
	}

	// Step 2: Re-bind to ConfigFS UDC with the UDC controller name
	rebind := tx.c.Changes[1]
	if rebind.Key != "udc-rebind" {
		t.Errorf("expected second change key to be 'udc-rebind', got %q", rebind.Key)
	}
	if rebind.Path != expectedUdcPath {
		t.Errorf("expected rebind path %q, got %q", expectedUdcPath, rebind.Path)
	}
	if string(rebind.ExpectedContent) != "ff800000.usb" {
		t.Errorf("expected rebind to write UDC name 'ff800000.usb', got %q", string(rebind.ExpectedContent))
	}
	if len(rebind.DependsOn) != 1 || rebind.DependsOn[0] != "udc-unbind" {
		t.Errorf("expected rebind to depend on 'udc-unbind', got %v", rebind.DependsOn)
	}
}

// TestRebindUsbDoesNotTouchPlatformDriver verifies that none of the generated file changes
// touch the unsafe platform driver path /sys/bus/platform/drivers/dwc3.
func TestRebindUsbDoesNotTouchPlatformDriver(t *testing.T) {
	tx := &UsbGadgetTransaction{
		c:             &ChangeSet{},
		udc:           "ff800000.usb",
		kvmGadgetPath: "/sys/kernel/config/usb_gadget/kvm",
	}

	tx.RebindUsb(true)

	for _, change := range tx.c.Changes {
		if strings.Contains(change.Path, "drivers/dwc3") || strings.Contains(change.Path, "/sys/bus/platform") {
			t.Fatalf("CRITICAL SAFETY VIOLATION: RebindUsb must not touch platform driver path: %s", change.Path)
		}
		if strings.Contains(string(change.ExpectedContent), "dwc3") {
			t.Fatalf("CRITICAL SAFETY VIOLATION: Unexpected dwc3 in change content: %s", string(change.ExpectedContent))
		}
	}
}

// TestArchitecturalSafetyNoDwc3PlatformDriverUnbind scans the entire usbgadget codebase
// to ensure no developer reintroduces direct unbinding of the DWC3 platform device driver,
// which causes f_mass_storage kernel panics and ESHUTDOWN on HID devices.
func TestArchitecturalSafetyNoDwc3PlatformDriverUnbind(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("failed to glob go files: %v", err)
	}

	forbiddenPatterns := []string{
		"/sys/bus/platform/drivers/dwc3",
		"dwc3/unbind",
		"dwc3Path",
	}

	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue // skip test files themselves
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("failed to read %s: %v", file, err)
		}
		content := string(data)
		for _, pattern := range forbiddenPatterns {
			if strings.Contains(content, pattern) {
				t.Fatalf("Architectural Safety Failure in %s: found forbidden pattern %q. Direct DWC3 driver unbind crashes the RV1106 kernel. Always use ConfigFS UDC unbind instead.", file, pattern)
			}
		}
	}
}
