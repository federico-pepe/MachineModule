// Package midimap says what incoming MIDI means to MachineModule. Plain Go.
package midimap

import "machinemodule/engine"

// FirstNote is the note of track 1; tracks 1-16 are notes 36-51 (C2-D#3), the
// same layout Push's lower pads use.
const FirstNote = 36

// NoteTrack maps a note to a track.
func NoteTrack(note int) (int, bool) {
	t := note - FirstNote
	return t, t >= 0 && t < engine.NumTracks
}

// Four tracks share one MIDI channel: a track's 24 parameters take CCs 16-111
// in order of parameter index, and tracks 1-4 sit on the base channel, 5-8 on
// the next, and so on.
const (
	FirstCC        = 16
	TracksPerChan  = 4
	ParamsPerTrack = engine.NParams // 24
)

// TrackParam maps a CC on a channel to a track parameter. base is the first
// channel (0-15).
func TrackParam(channel, cc, base int) (track, param int, ok bool) {
	g := channel - base
	if g < 0 || g >= engine.NumTracks/TracksPerChan {
		return
	}
	i := cc - FirstCC
	if i < 0 || i >= TracksPerChan*ParamsPerTrack {
		return
	}
	return g*TracksPerChan + i/ParamsPerTrack, i % ParamsPerTrack, true
}

// Global CCs on the base channel: 112 master volume, 113 tempo (30-157 BPM).
const (
	CCMasterVol = 112
	CCTempo     = 113
	CCMuteAll   = 114 // 127 mutes every track, 0 unmutes
)

// Tempo turns a CC value into BPM.
func Tempo(v int) float64 { return 30 + float64(v) }
