package usbgadget

import (
	"fmt"
	"os"
	"path"
	"strings"
	"time"
)

func getUdcs() []string {
	var udcs []string

	files, err := os.ReadDir("/sys/devices/platform/usbdrd")
	if err != nil {
		return nil
	}

	for _, file := range files {
		if !file.IsDir() || !strings.HasSuffix(file.Name(), ".usb") {
			continue
		}
		udcs = append(udcs, file.Name())
	}

	return udcs
}

func rebindUsb(udc string, ignoreUnbindError bool) error {
	_ = os.WriteFile(udcPath, []byte(""), 0644)
	time.Sleep(100 * time.Millisecond)
	err := os.WriteFile(udcPath, []byte(udc), 0644)
	if err != nil && !ignoreUnbindError {
		return err
	}
	return nil
}

func (u *UsbGadget) rebindUsb(ignoreUnbindError bool) error {
	u.log.Info().Str("udc", u.udc).Msg("rebinding USB gadget to UDC")
	return rebindUsb(u.udc, ignoreUnbindError)
}

// RebindUsb rebinds the USB gadget to the UDC.
func (u *UsbGadget) RebindUsb(ignoreUnbindError bool) error {
	u.configLock.Lock()
	defer u.configLock.Unlock()

	return u.rebindUsb(ignoreUnbindError)
}

// GetUsbState returns the current state of the USB gadget
func (u *UsbGadget) GetUsbState(enhancedDetection bool) (state string) {
	if enhancedDetection {
		discFile := "/sys/devices/platform/ff3e0000.usb2-phy/disc"
		discBytes, err := os.ReadFile(discFile)
		if err == nil {
			discState := strings.TrimSpace(string(discBytes))
			if discState == "DISCONNECTED" {
				return "not attached"
			}
		}
	}

	stateFile := path.Join("/sys/class/udc", u.udc, "state")
	stateBytes, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "not attached"
		} else {
			u.log.Trace().Err(err).Msg("failed to read usb state")
		}
		return "unknown"
	}
	return strings.TrimSpace(string(stateBytes))
}

// IsUDCBound checks if the UDC state is bound.
func (u *UsbGadget) IsUDCBound() (bool, error) {
	udcFilePath := path.Join("/sys/class/udc", u.udc)
	_, err := os.Stat(udcFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("error checking USB emulation state: %w", err)
	}
	return true, nil
}

// BindUDC binds the gadget to the UDC.
func (u *UsbGadget) BindUDC() error {
	err := os.WriteFile(udcPath, []byte(u.udc), 0644)
	if err != nil {
		return fmt.Errorf("error binding UDC: %w", err)
	}
	return nil
}

func (u *UsbGadget) BindUDCToDWC3() error {
	return u.BindUDC()
}

// UnbindUDC unbinds the gadget from the UDC.
func (u *UsbGadget) UnbindUDC() error {
	err := os.WriteFile(udcPath, []byte("none"), 0644)
	if err != nil {
		return fmt.Errorf("error unbinding UDC: %w", err)
	}
	return nil
}

func (u *UsbGadget) UnbindUDCToDWC3() error {
	return u.UnbindUDC()
}
