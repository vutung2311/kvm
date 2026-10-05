package kvm

import (
	"errors"
	"os"
	"strconv"
	"sync"
	"time"
)

var currentScreen = "ui_Boot_Screen"
var backlightState = 0 // 0 - NORMAL, 1 - DIMMED, 2 - OFF

var (
	backlightLock sync.Mutex
	dimTimer      *time.Timer
	offTimer      *time.Timer
)

const (
	touchscreenDevice     string = "/dev/input/event0"
	backlightControlClass string = "/sys/class/backlight/backlight/brightness"
)

func switchToScreen(screen string) {
	_, err := CallDisplayCtrlAction("lv_scr_load", map[string]interface{}{"obj": screen})
	if err != nil {
		displayLogger.Warn().Err(err).Str("screen", screen).Msg("failed to switch to screen")
		return
	}
	currentScreen = screen
}

var displayedTexts = make(map[string]string)

func lvObjSetState(objName string, state string) (*CtrlResponse, error) {
	return CallDisplayCtrlAction("lv_obj_set_state", map[string]interface{}{"obj": objName, "state": state})
}

func lvObjAddFlag(objName string, flag string) (*CtrlResponse, error) {
	return CallDisplayCtrlAction("lv_obj_add_flag", map[string]interface{}{"obj": objName, "flag": flag})
}

func lvObjClearFlag(objName string, flag string) (*CtrlResponse, error) {
	return CallDisplayCtrlAction("lv_obj_clear_flag", map[string]interface{}{"obj": objName, "flag": flag})
}

func lvObjHide(objName string) (*CtrlResponse, error) {
	return lvObjAddFlag(objName, "LV_OBJ_FLAG_HIDDEN")
}

func lvObjShow(objName string) (*CtrlResponse, error) {
	return lvObjClearFlag(objName, "LV_OBJ_FLAG_HIDDEN")
}

func lvObjSetOpacity(objName string, opacity int) (*CtrlResponse, error) { // nolint:unused
	return CallDisplayCtrlAction("lv_obj_set_style_opa_layered", map[string]interface{}{"obj": objName, "opa": opacity})
}

func lvObjFadeIn(objName string, duration uint32) (*CtrlResponse, error) {
	return CallDisplayCtrlAction("lv_obj_fade_in", map[string]interface{}{"obj": objName, "time": duration})
}

func lvObjFadeOut(objName string, duration uint32) (*CtrlResponse, error) {
	return CallDisplayCtrlAction("lv_obj_fade_out", map[string]interface{}{"obj": objName, "time": duration})
}

func lvLabelSetText(objName string, text string) (*CtrlResponse, error) {
	return CallDisplayCtrlAction("lv_label_set_text", map[string]interface{}{"obj": objName, "text": text})
}

func lvImgSetSrc(objName string, src string) (*CtrlResponse, error) {
	return CallDisplayCtrlAction("lv_img_set_src", map[string]interface{}{"obj": objName, "src": src})
}

func lvDispSetRotation(rotation string) (*CtrlResponse, error) {
	return CallDisplayCtrlAction("lv_disp_set_rotation", map[string]interface{}{"rotation": rotation})
}

func lvObjSetStyleBgColor(objName string, color string) (*CtrlResponse, error) {
	return CallDisplayCtrlAction("lv_obj_set_style_bg_color", map[string]interface{}{"obj": objName, "color": color})
}

func updateLabelIfChanged(objName string, newText string) {
	if newText != "" && newText != displayedTexts[objName] {
		_, _ = lvLabelSetText(objName, newText)
		displayedTexts[objName] = newText
	}
}

func switchToScreenIfDifferent(screenName string) {
	if currentScreen != screenName {
		displayLogger.Info().Str("from", currentScreen).Str("to", screenName).Msg("switching screen")
		switchToScreen(screenName)
	}
}

var (
	cloudBlinkLock    sync.Mutex = sync.Mutex{}
	cloudBlinkStopped bool
	cloudBlinkTicker  *time.Ticker
)

func updateDisplay() {
	updateLabelIfChanged("Network_Address_IP_Label", networkState.IPv4String())
	updateLabelIfChanged("Version_Hostname_Label", GetHostname())

	if usbState == "configured" {
		_, _ = lvObjSetState("Main", "USB_CONNECTED")
	} else {
		_, _ = lvObjSetState("Main", "USB_DISCONNECTED")
	}
	_ = os.WriteFile("/userdata/usb_state", []byte(usbState), 0644)

	if lastVideoState.Ready {
		_, _ = lvObjSetState("Main", "HDMI_CONNECTED")
		_ = os.WriteFile("/userdata/hdmi_state", []byte("connected"), 0644)
	} else {
		_, _ = lvObjSetState("Main", "HDMI_DISCONNECTED")
		_ = os.WriteFile("/userdata/hdmi_state", []byte("disconnected"), 0644)
	}

	if networkState.IsUp() {
		_, _ = lvObjSetState("Network", "NETWORK")
	} else {
		_, _ = lvObjSetState("Network", "NO_NETWORK")
	}
}

var (
	displayInited     = false
	displayUpdateLock = sync.Mutex{}
	waitDisplayUpdate = sync.Mutex{}
)

func requestDisplayUpdate() {
	displayUpdateLock.Lock()
	defer displayUpdateLock.Unlock()

	if !displayInited {
		displayLogger.Info().Msg("display not inited, skipping updates")
		return
	}
	go func() {
		displayLogger.Debug().Msg("display updating")
		//TODO: only run once regardless how many pending updates
		updateDisplay()
	}()
}

func waitCtrlAndRequestDisplayUpdate() {
	waitDisplayUpdate.Lock()
	defer waitDisplayUpdate.Unlock()

	waitDisplayCtrlClientConnected()
	requestDisplayUpdate()
}

func updateStaticContents() {
	//contents that never change
	updateLabelIfChanged("Network_Address_Mac_Label", networkState.MACString())
	_, appVersion, err := GetLocalVersion()
	if err == nil {
		updateLabelIfChanged("Version_App_Version_Label", appVersion.String())
	}
}

// setDisplayBrightness sets /sys/class/backlight/backlight/brightness to alter
// the backlight brightness of the KVM hardware's display.
func setDisplayBrightness(brightness int) error {
	if brightness > 255 || brightness < 0 {
		return errors.New("brightness value out of bounds, must be between 0 and 255")
	}

	// Check the display backlight class is available
	if _, err := os.Stat(backlightControlClass); errors.Is(err, os.ErrNotExist) {
		return errors.New("brightness value cannot be set, possibly not running on KVM hardware")
	}

	// Set the value
	bs := []byte(strconv.Itoa(brightness))
	err := os.WriteFile(backlightControlClass, bs, 0644)
	if err != nil {
		return err
	}

	displayLogger.Info().Int("brightness", brightness).Msg("set brightness")
	return nil
}

func onDimTimer() {
	backlightLock.Lock()
	defer backlightLock.Unlock()

	// Only dim if currently normal (state 0) and not turned off in config
	if backlightState != 0 || config.DisplayMaxBrightness == 0 {
		return
	}

	dimBrightness := config.DisplayMaxBrightness / 2
	err := setDisplayBrightness(dimBrightness)
	if err != nil {
		displayLogger.Warn().Err(err).Msg("failed to dim display")
	} else {
		displayLogger.Info().Int("brightness", dimBrightness).Msg("display dimmed due to inactivity")
	}

	backlightState = 1
	dimTimer = nil
}

func onOffTimer() {
	backlightLock.Lock()
	defer backlightLock.Unlock()

	// Only turn off if not already off
	if backlightState == 2 || config.DisplayMaxBrightness == 0 {
		return
	}

	err := setDisplayBrightness(0)
	if err != nil {
		displayLogger.Warn().Err(err).Msg("failed to turn off display")
	} else {
		displayLogger.Info().Msg("display turned off due to inactivity")
	}

	backlightState = 2
	offTimer = nil
}

func resetBacklightTimersLocked() {
	if dimTimer != nil {
		dimTimer.Stop()
		dimTimer = nil
	}
	if offTimer != nil {
		offTimer.Stop()
		offTimer = nil
	}

	if config.DisplayMaxBrightness == 0 {
		return
	}

	if config.DisplayDimAfterSec > 0 {
		dimTimer = time.AfterFunc(time.Duration(config.DisplayDimAfterSec)*time.Second, onDimTimer)
	}

	if config.DisplayOffAfterSec > 0 {
		offTimer = time.AfterFunc(time.Duration(config.DisplayOffAfterSec)*time.Second, onOffTimer)
	}
}

// wakeDisplay restores display brightness to config.DisplayMaxBrightness and resets
// the dim and screen-off inactivity timers.
func wakeDisplay(force bool) {
	backlightLock.Lock()
	defer backlightLock.Unlock()

	displayLogger.Info().Bool("force", force).Int("prevState", backlightState).Msg("wakeDisplay")

	// If max brightness is 0, display is configured to be permanently off
	if config.DisplayMaxBrightness == 0 {
		_ = setDisplayBrightness(0)
		backlightState = 2
		if dimTimer != nil {
			dimTimer.Stop()
			dimTimer = nil
		}
		if offTimer != nil {
			offTimer.Stop()
			offTimer = nil
		}
		return
	}

	// Restore brightness if dimmed or off, or if explicitly forced
	if backlightState != 0 || force {
		err := setDisplayBrightness(config.DisplayMaxBrightness)
		if err != nil {
			displayLogger.Warn().Err(err).Msg("failed to wake display")
		}
		backlightState = 0
	}

	// Always restart inactivity timers on touch or wake event
	resetBacklightTimersLocked()
}

// watchTsEvents monitors the touchscreen for events and calls wakeDisplay() to ensure the
// touchscreen interface wakes or keeps the display active.
func watchTsEvents() {
	buf := make([]byte, 24)
	for {
		ts, err := os.OpenFile(touchscreenDevice, os.O_RDONLY, 0666)
		if err != nil {
			displayLogger.Warn().Err(err).Msg("failed to open touchscreen device, will retry")
			time.Sleep(2 * time.Second)
			continue
		}

		for {
			_, err := ts.Read(buf)
			if err != nil {
				displayLogger.Warn().Err(err).Msg("failed to read from touchscreen device, reconnecting")
				break
			}

			wakeDisplay(false)
		}

		_ = ts.Close()
		time.Sleep(1 * time.Second)
	}
}

// reloadBacklightSettings applies updated backlight configuration without turning on the screen if it is off.
func reloadBacklightSettings() {
	backlightLock.Lock()
	defer backlightLock.Unlock()

	// If max brightness is 0, display is permanently turned off
	if config.DisplayMaxBrightness == 0 {
		_ = setDisplayBrightness(0)
		backlightState = 2
		if dimTimer != nil {
			dimTimer.Stop()
			dimTimer = nil
		}
		if offTimer != nil {
			offTimer.Stop()
			offTimer = nil
		}
		return
	}

	// If the display has turned off due to inactivity, keep it off! Only physical touch should turn it on.
	if backlightState == 2 {
		displayLogger.Info().Msg("backlight settings updated while display is off; remaining off until touched")
		return
	}

	// If currently dimmed, update to half of new max brightness
	if backlightState == 1 {
		dimBrightness := config.DisplayMaxBrightness / 2
		_ = setDisplayBrightness(dimBrightness)
		resetBacklightTimersLocked()
		return
	}

	// If currently normal (on), update to new max brightness and reset timers
	if backlightState == 0 {
		_ = setDisplayBrightness(config.DisplayMaxBrightness)
		resetBacklightTimersLocked()
		return
	}
}

func initDisplay() {
	go func() {
		waitDisplayCtrlClientConnected()
		displayLogger.Info().Msg("setting initial display contents")
		time.Sleep(500 * time.Millisecond)
		_, _ = lvDispSetRotation(config.DisplayRotation)
		updateStaticContents()
		initTimeZone()
		displayInited = true
		displayLogger.Info().Msg("display inited")
		wakeDisplay(true)
		requestDisplayUpdate()
	}()

	go watchTsEvents()
}
