// Package store holds MachineModule's files: its settings, the project and the
// UserWave samples. Plain Go, tested without hardware.
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ConfigFile sits next to hack.json and survives updates.
const ConfigFile = "machinemodule-config.json"

// Config is what the user can change from the web page or by hand.
type Config struct {
	// PCMDevice is the loopback side MachineModule writes to.
	PCMDevice string `json:"pcm_device"`
	// ChannelOffset is the first channel (0-based) of the stereo pair on the
	// card; 0 means channels 1/2.
	ChannelOffset int `json:"channel_offset"`
	// BusPair is the pair asked of the audio mixer, or -1 to let the mixer
	// choose and remember one.
	BusPair int `json:"bus_pair"`
	// MidiChannel is the channel external notes and CCs are read on (0-15), or
	// -1 for any.
	MidiChannel int `json:"midi_channel"`
	// PushManagerURL is for development only.
	PushManagerURL string `json:"push_manager_url,omitempty"`
}

// DefaultConfig is a fresh install.
func DefaultConfig() Config {
	return Config{PCMDevice: "hw:PHVAudio,1,0", BusPair: -1, MidiChannel: -1}
}

// LoadConfig reads the config; a missing file is the defaults.
func LoadConfig(dir string) (Config, error) {
	c := DefaultConfig()
	b, err := os.ReadFile(filepath.Join(dir, ConfigFile))
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return DefaultConfig(), err
	}
	if c.ChannelOffset < 0 || c.ChannelOffset > 62 {
		c.ChannelOffset = 0
	}
	if c.BusPair < -1 || c.BusPair > 62 {
		c.BusPair = -1
	}
	if c.MidiChannel < -1 || c.MidiChannel > 15 {
		c.MidiChannel = -1
	}
	if c.PCMDevice == "" {
		c.PCMDevice = DefaultConfig().PCMDevice
	}
	return c, nil
}

// SaveConfig writes the config atomically.
func SaveConfig(dir string, c Config) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, ConfigFile), b)
}

func writeAtomic(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
