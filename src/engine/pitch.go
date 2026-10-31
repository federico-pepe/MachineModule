package engine

import "math"

// PitchPerSemitone is about how many steps of the PTCH parameter make one
// semitone on a machine. Exact for the UW sampler and the sine generator;
// the drum machines have ranges of a few octaves, so it is a close guess.
func PitchPerSemitone(machine int) float64 {
	if machine < 0 || machine >= len(Machines) {
		return 2
	}
	switch Machines[machine].Group {
	case "UW":
		return 1 / 0.375
	}
	if Machines[machine].Name == "GND-SIN" {
		return 127 / (12 * math.Log2(3520/27.5))
	}
	return 2
}

// HitPitched plays a track at a given PTCH value (0..127), as the pitch keys
// do. In record mode it writes the trig with a PTCH lock.
func (e *Engine) HitPitched(track int, vel float32, ptch int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if track < 0 || track >= NumTracks {
		return
	}
	if vel <= 0 {
		vel = velNormal
	}
	ptch = clampi(ptch, 0, 127)
	t := &e.tracks[track]
	t.lockMask = 1
	t.lockVal[0] = float32(ptch) / 127
	t.slideRemain = 0
	e.fireLocked(track, vel, 0)
	if e.rec && e.playing {
		pat := e.proj.Pattern(e.curPat)
		st := float64(StepTicks(pat.Scale))
		k := int(math.Floor(e.tick/st + 0.5))
		if k >= pat.Len {
			k = 0
		}
		pat.Tracks[track].Trig[k] = true
		l := Lock{Mask: 1}
		l.Val[0] = uint8(ptch)
		pat.Tracks[track].Lock[k] = &l
		e.dirty = true
	}
}

// EffectiveParam is the value a parameter has right now on a track: the kit
// value, or the lock of the trig that is sounding.
func (e *Engine) EffectiveParam(track, p int) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	if track < 0 || track >= NumTracks || p < 0 || p >= NParams {
		return 0
	}
	return int(e.kit.Track[track].P[p])
}
