// Package audioout lists what the web page offers for the audio output: the
// ALSA playback device and the stereo channel pair on it. Plain Go, tested
// without hardware.
package audioout

import (
	"fmt"

	"github.com/federico-pepe/ableton-push-hack/core/alsapcm"
)

// LoopbackChannels is push-audio-loopback's channel count; it is used until
// Live has negotiated the real one.
const LoopbackChannels = 32

// Option is one row of a dropdown.
type Option struct {
	Value   string `json:"value"`
	Label   string `json:"label"`
	Current bool   `json:"current"`
}

// Devices lists the playback devices. A current device the system does not
// list (set by hand in the config file) is added, so the dropdown never shows
// a value the module is not using.
func Devices(list []alsapcm.PlaybackDevice, current string) []Option {
	out := make([]Option, 0, len(list)+1)
	found := false
	for _, d := range list {
		hw := d.HWDevice()
		out = append(out, Option{Value: hw, Label: fmt.Sprintf("%s (%s)", d.Name, hw), Current: hw == current})
		found = found || hw == current
	}
	if !found && current != "" {
		out = append(out, Option{Value: current, Label: current + " (not found)", Current: true})
	}
	return out
}

// HasDevice says whether device is one of the listed playback devices.
func HasDevice(list []alsapcm.PlaybackDevice, device string) bool {
	for _, d := range list {
		if d.HWDevice() == device {
			return true
		}
	}
	return false
}

// Pairs lists the stereo pairs of a card with the given channel count; the
// value is the 0-based offset of the first channel.
func Pairs(channels, current int) []Option {
	if channels < 2 {
		channels = LoopbackChannels
	}
	var out []Option
	for off := 0; off+1 < channels; off += 2 {
		out = append(out, Option{Value: fmt.Sprint(off), Label: fmt.Sprintf("Ch %d-%d", off+1, off+2), Current: off == current})
	}
	return out
}

// ValidPair says whether offset starts a pair that Pairs lists.
func ValidPair(channels, offset int) bool {
	if channels < 2 {
		channels = LoopbackChannels
	}
	return offset >= 0 && offset%2 == 0 && offset+1 < channels
}
