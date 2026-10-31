package engine

import "math"

// track is the live state of one of the 16 tracks: its voice, the value of
// every parameter as it is being played (kit value, parameter lock, slide,
// LFO), and the effect chain after the voice.
type track struct {
	machine int
	v       voice
	active  bool // the voice is still sounding
	tail    int  // samples of filter tail left after the voice ends
	mute    bool

	// What is playing now, normalised 0..1. Updated every render chunk.
	cur [NParams]float32
	// A parameter lock held from the last trig that had one.
	lockMask    uint32
	lockVal     [NParams]float32
	slideRemain int // samples left in a parameter slide
	hasCur      bool

	// LFO
	lfoPhase float32
	lfoHold  float32
	lfoRn    rng

	// effect state
	hp, lp         svf
	eq             biquad
	hpOn, lpOn     bool
	eqOn           bool
	amPhase        float32
	srrAcc, srrVal float32

	peak float32 // output level since the last read, for the meters

	pbuf [NSynth]float32 // scratch for the voice call (an interface call would heap-allocate a local)
}

func (t *track) setMachine(idx int, sr float64, src waveSource) {
	if idx < 0 || idx >= len(Machines) {
		idx = 0
	}
	t.machine = idx
	t.v = Machines[idx].build(sr, src)
	t.active = false
	t.tail = 0
	t.lfoRn.seed(uint32(idx)*7919 + 17)
}

// trig starts the voice. vel is 0..1.
func (t *track) trig(vel float32, p *[NSynth]float32) {
	if t.mute || t.v == nil {
		return
	}
	t.v.trig(vel, p)
	t.active = true
	t.lfoPhase = 0
	t.lfoHold = t.lfoRn.next()
}

// resolve fills t.cur from the kit values (base), the lock held by the last
// trig and any slide in progress, then returns the values after the LFO.
// n is the chunk length in samples; sr the sample rate.
func (t *track) resolve(base *[NParams]uint8, n int, sr float64, eff *[NParams]float32) {
	slideF := float32(1)
	if t.slideRemain > 0 {
		slideF = clampf(float32(n)/float32(t.slideRemain), 0, 1)
		t.slideRemain -= n
		if t.slideRemain < 0 {
			t.slideRemain = 0
		}
	}
	for i := 0; i < NParams; i++ {
		target := float32(base[i]) / 127
		locked := t.lockMask&(1<<uint(i)) != 0
		if locked {
			target = t.lockVal[i]
		}
		if !t.hasCur || !locked || slideF >= 1 {
			t.cur[i] = target
		} else {
			t.cur[i] += (target - t.cur[i]) * slideF
		}
	}
	t.hasCur = true
	*eff = t.cur

	// LFO: one per track, aimed at any single parameter.
	depth := t.cur[LFOD]
	if depth > 0.003 {
		dest := int(t.cur[LFOP] * float32(NParams) * 0.9999)
		if dest < LFOP { // the LFO does not modulate itself
			var l float32
			switch int(t.cur[LFOM] * 3.9999) {
			case 0:
				l = fsin(t.lfoPhase)
			case 1:
				ph := t.lfoPhase - float32(math.Floor(float64(t.lfoPhase)))
				if ph < 0.5 {
					l = ph*4 - 1
				} else {
					l = 3 - ph*4
				}
			case 2:
				ph := t.lfoPhase - float32(math.Floor(float64(t.lfoPhase)))
				l = 1 - 2*ph
			default:
				l = t.lfoHold
			}
			eff[dest] = clampf(eff[dest]+l*depth, 0, 1)
		}
	}
	hz := expMap(t.cur[LFOS], 0.03, 40)
	prev := t.lfoPhase
	t.lfoPhase += hz * float32(n) / float32(sr)
	if int(t.lfoPhase) != int(prev) {
		t.lfoHold = t.lfoRn.next()
	}
	if t.lfoPhase > 1e6 {
		t.lfoPhase -= 1e6
	}
}

// Level of the track's signal that goes to the mix: volume 100 of 127 is
// about -4 dB, 127 is +4 dB.
func volGain(v float32) float32 { return v * v * 1.6 }

// process renders n samples of the track into the mix buses. buf is scratch
// of at least n samples. mixL/R get the dry signal, sendD the delay send and
// sendR the reverb send (mono each; the global effects make them stereo).
func (t *track) process(eff *[NParams]float32, n int, sr float64, buf, mixL, mixR, sendD, sendR []float32) {
	if t.v == nil || t.mute {
		t.active = false
		return
	}
	if !t.active && t.tail <= 0 {
		return
	}
	for i := 0; i < n; i++ {
		buf[i] = 0
	}
	if t.active {
		copy(t.pbuf[:], eff[:NSynth])
		t.active = t.v.render(buf[:n], &t.pbuf)
		if t.active {
			t.tail = int(sr * 0.8)
		}
	}
	if !t.active {
		t.tail -= n
		if t.tail <= 0 {
			t.hp.reset()
			t.lp.reset()
			t.eq.reset()
			return
		}
	}

	// ---- effect coefficients for this chunk
	amDepth := eff[AMD]
	amInc := expMap(eff[AMF], 0.3, 3000) / float32(sr)
	amOn := amDepth > 0.003

	eqG := (eff[EQG] - 0.5) * 36
	t.eqOn = absf(eqG) > 0.25
	if t.eqOn {
		t.eq.peaking(expMap(eff[EQF], 40, 12000), 0.9, eqG, sr)
	}

	fltF, fltW := eff[FLTF], eff[FLTW]
	q := lerp(0.707, 9, eff[FLTQ]*eff[FLTQ])
	t.hpOn = fltF > 0.008
	hpFc := expMap(fltF, 20, 14000)
	lpBase := float32(20)
	if t.hpOn {
		lpBase = hpFc
		t.hp.set(hpFc, lerp(0.707, 4, eff[FLTQ]*eff[FLTQ]), sr)
	}
	lpFc := lpBase * float32(math.Exp2(float64(fltW*10)))
	t.lpOn = lpFc < 16500
	if t.lpOn {
		t.lp.set(lpFc, q, sr)
	}
	_ = q

	srrStep := 1 + eff[SRR]*eff[SRR]*47
	srrOn := srrStep > 1.02

	vol := eff[VOL]
	g := volGain(vol)
	pan := eff[PAN] * (math.Pi / 2)
	gl := g * float32(math.Cos(float64(pan)))
	gr := g * float32(math.Sin(float64(pan)))
	delAmt := eff[DEL] * g
	revAmt := eff[REV] * g

	var peak float32
	for i := 0; i < n; i++ {
		x := buf[i]
		if amOn {
			x *= 1 - amDepth*(0.5-0.5*fsin(t.amPhase))
			t.amPhase += amInc
			if t.amPhase >= 1 {
				t.amPhase--
			}
		}
		if t.eqOn {
			x = t.eq.tick(x)
		}
		if t.hpOn {
			_, _, x = t.hp.tick(x)
		}
		if t.lpOn {
			x, _, _ = t.lp.tick(x)
		}
		if srrOn {
			t.srrAcc++
			if t.srrAcc >= srrStep {
				t.srrAcc -= srrStep
				t.srrVal = x
			}
			x = t.srrVal
		}
		if isBad(x) {
			x = 0
			t.hp.reset()
			t.lp.reset()
			t.eq.reset()
		}
		mixL[i] += x * gl
		mixR[i] += x * gr
		sendD[i] += x * delAmt
		sendR[i] += x * revAmt
		if a := absf(x * g); a > peak {
			peak = a
		}
	}
	if peak > t.peak {
		t.peak = peak
	}
}
