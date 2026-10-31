package engine

import "math"

// ------------------------------------------------------------------ EFM
//
// EFM: a two-operator FM voice. One carrier, one modulator with feedback, an
// index envelope that closes faster than the amplitude envelope (that is what
// makes FM drums snap). Every EFM machine is this voice with different ranges
// and defaults; the variants differ only in the numbers in efmCfg.

type efmCfg struct {
	fLo, fHi     float32 // PTCH range, Hz
	decLo, decHi float32
	rampOct      float32 // pitch drop at start, octaves
	pDecay       float32 // pitch envelope, seconds to -60 dB
	bursts       int     // > 1 retriggers the envelope (clap)
	burstGap     float32 // seconds between bursts
	hpHz         float32 // output high-pass; 0 = off (hats and cymbals)
}

type efmVoice struct {
	cfg                  efmCfg
	sr                   float64
	rn                   rng
	cph, mph, fbState    float32
	amp, idx, pEnv       float32
	vel                  float32
	burstsLeft, sinceGap int
	hp                   svf
	on                   bool
}

func newEFM(cfg efmCfg, sr float64) *efmVoice {
	v := &efmVoice{cfg: cfg, sr: sr}
	v.rn.seed(0x4D2A8E1)
	if cfg.hpHz > 0 {
		v.hp.set(cfg.hpHz, 0.8, sr)
	}
	return v
}

func (v *efmVoice) trig(vel float32, p *[NSynth]float32) {
	v.on = true
	v.vel = vel
	v.amp, v.idx, v.pEnv = vel, 1, 1
	v.cph, v.mph, v.fbState = 0, 0, 0
	v.burstsLeft = v.cfg.bursts - 1
	v.sinceGap = 0
}

func (v *efmVoice) render(out []float32, p *[NSynth]float32) bool {
	if !v.on {
		return false
	}
	c := &v.cfg
	invSR := float32(1 / v.sr)
	base := expMap(p[0], c.fLo, c.fHi)
	dec := expMap(p[1], c.decLo, c.decHi)
	ac := decayCoef(v.sr, dec)
	ratio := expMap(p[2], 0.25, 8)
	index := p[3] * 6
	ic := decayCoef(v.sr, maxf(dec*expMap(p[4], 0.05, 1.2), 0.004))
	feed := p[5] * 0.85
	noise, dist := p[6], p[7]
	pc := decayCoef(v.sr, c.pDecay)
	gap := int(c.burstGap * float32(v.sr))
	var finc float32
	for i := range out {
		if i&7 == 0 {
			finc = base * invSR * float32(math.Exp2(float64(c.rampOct*v.pEnv)))
		}
		if v.burstsLeft > 0 {
			v.sinceGap++
			if v.sinceGap >= gap {
				v.sinceGap = 0
				v.burstsLeft--
				v.amp, v.idx = v.vel, 1
			}
		}
		mod := fsin(v.mph + feed*v.fbState)
		v.fbState = mod
		s := fsin(v.cph+index*v.idx*mod*0.5) * v.amp
		if noise > 0 {
			s += v.rn.next() * v.amp * noise * 0.5
		}
		if c.hpHz > 0 {
			_, _, s = v.hp.tick(s)
		}
		out[i] += drive(s*0.85, dist)

		v.amp *= ac
		v.idx *= ic
		v.pEnv *= pc
		v.cph += finc
		v.mph += finc * ratio
		if v.cph >= 1 {
			v.cph--
		}
		if v.mph >= 1 {
			v.mph--
		}
	}
	if v.amp < 0.0003 && v.burstsLeft == 0 {
		v.on = false
	}
	return v.on
}

// ------------------------------------------------------------------- PI
//
// PI: physical-model flavoured. A bank of decaying resonators (one
// two-pole filter per mode) is struck by an impulse and, optionally, a
// burst of noise. MATR morphs the mode ratios from a drum membrane, through
// a metal bar, to a cymbal-like plate. DAMP shortens the upper modes, HITP
// moves the strike point, TENS tightens the "skin" so the pitch falls after
// the hit.

const piModes = 10

var (
	piMembrane = [piModes]float32{1, 1.594, 2.136, 2.296, 2.653, 2.918, 3.156, 3.501, 3.600, 3.652}
	piBar      = [piModes]float32{1, 2.756, 5.404, 8.933, 13.344, 18.64, 24.8, 31.9, 40, 49}
	piPlate    = [piModes]float32{1, 1.47, 1.83, 2.34, 2.76, 3.31, 4.12, 4.85, 5.67, 6.9}
)

type piCfg struct {
	fLo, fHi     float32
	decLo, decHi float32
	rampOct      float32 // pitch fall at TENS = 1, octaves
	buzz         float32 // snare-wire noise level (PI-SD); 0 = none
	modes        int
	gain         float32
}

type piVoice struct {
	cfg        piCfg
	sr         float64
	rn         rng
	y1, y2     [piModes]float32
	b1, b2, w  [piModes]float32 // coefficients, refreshed every 32 samples
	pEnv       float32
	nEnv, bEnv float32
	vel        float32
	energy     float32
	on         bool
	pend       bool // impulse not yet delivered
	snapshotP  [NSynth]float32
	buzzHP     svf
}

func newPI(cfg piCfg, sr float64) *piVoice {
	v := &piVoice{cfg: cfg, sr: sr}
	v.rn.seed(0x91C3E57)
	v.buzzHP.set(2500, 0.8, sr)
	return v
}

func (v *piVoice) trig(vel float32, p *[NSynth]float32) {
	v.on, v.pend = true, true
	v.vel = vel
	v.pEnv, v.nEnv, v.bEnv = 1, vel, vel
	v.y1 = [piModes]float32{}
	v.y2 = [piModes]float32{}
}

func (v *piVoice) coeffs(p *[NSynth]float32) {
	c := &v.cfg
	base := expMap(p[0], c.fLo, c.fHi) * float32(math.Exp2(float64(p[3]*c.rampOct*v.pEnv)))
	dec := expMap(p[1], c.decLo, c.decHi)
	mat := p[2]
	damp := p[4]
	nyq := float32(v.sr) * 0.45
	for i := 0; i < c.modes; i++ {
		var r float32
		if mat < 0.5 {
			r = lerp(piMembrane[i], piBar[i], mat*2)
		} else {
			r = lerp(piBar[i], piPlate[i], (mat-0.5)*2)
		}
		f := base * r
		if f > nyq {
			v.b1[i], v.b2[i], v.w[i] = 0, 0, 0
			continue
		}
		d := dec / (1 + damp*float32(i)*2.2)
		rr := decayCoef(v.sr, d)
		w := 2 * math.Pi * float64(f) / v.sr
		v.b1[i] = 2 * rr * float32(math.Cos(w))
		v.b2[i] = -rr * rr
		v.w[i] = float32(math.Sin(w))
	}
}

func (v *piVoice) render(out []float32, p *[NSynth]float32) bool {
	if !v.on {
		return false
	}
	c := &v.cfg
	pc := decayCoef(v.sr, 0.18)
	nc := decayCoef(v.sr, 0.01)
	bc := decayCoef(v.sr, expMap(p[1], c.decLo, c.decHi)*0.9)
	hit, noise, dist := p[5], p[6], p[7]
	var weight [piModes]float32
	for i := 0; i < c.modes; i++ {
		weight[i] = (0.35 + 0.65*absf(fsin(float32(i+1)*(0.07+0.4*hit)))) / (1 + 0.3*float32(i))
	}
	for i := range out {
		if i&31 == 0 {
			v.coeffs(p)
		}
		var sum float32
		var x0 float32
		if v.pend {
			x0 = v.vel
		}
		nIn := v.rn.next() * v.nEnv * noise * 0.35
		for m := 0; m < c.modes; m++ {
			x := nIn * weight[m]
			if v.pend {
				x += x0 * weight[m] * v.w[m]
			}
			y := v.b1[m]*v.y1[m] + v.b2[m]*v.y2[m] + x
			v.y2[m], v.y1[m] = v.y1[m], y
			sum += y
		}
		v.pend = false
		s := sum * c.gain
		if c.buzz > 0 {
			_, _, hp := v.buzzHP.tick(v.rn.next())
			s += hp * v.bEnv * c.buzz * 0.5
		}
		out[i] += drive(s, dist)
		v.pEnv *= pc
		v.nEnv *= nc
		v.bEnv *= bc
		e := absf(sum)
		v.energy += (e - v.energy) * 0.0005
	}
	if v.energy < 0.00008 && v.bEnv < 0.0003 && !v.pend {
		v.on = false
	}
	return v.on
}

// ------------------------------------------------------------------ GND

// gndSin: a plain sine tone with a pitch drop, harmonics, folding and a
// detuned twin. Useful for sub, tonal blips, FM-less bells.
type gndSinVoice struct {
	sr           float64
	ph, ph2      float32
	amp, pEnv    float32
	age          int
	vel          float32
	on           bool
}

func newGndSin(sr float64) *gndSinVoice { return &gndSinVoice{sr: sr} }

func (v *gndSinVoice) trig(vel float32, p *[NSynth]float32) {
	v.on, v.vel, v.amp, v.pEnv, v.age = true, vel, vel, 1, 0
	v.ph, v.ph2 = 0, 0
}

func (v *gndSinVoice) render(out []float32, p *[NSynth]float32) bool {
	if !v.on {
		return false
	}
	invSR := float32(1 / v.sr)
	base := expMap(p[0], 20, 2200)
	ac := decayCoef(v.sr, expMap(p[1], 0.03, 4))
	atk := maxi(1, int(p[2]*0.08*float32(v.sr)))
	ramp := p[3] * 4
	pc := decayCoef(v.sr, 0.2)
	harm, fold, detu, dist := p[4], p[5], p[6], p[7]
	detuRatio := float32(math.Pow(2, float64(detu)*0.35/12*2))
	var inc float32
	for i := range out {
		if i&7 == 0 {
			inc = base * invSR * float32(math.Exp2(float64(ramp*v.pEnv)))
		}
		s := fsin(v.ph)
		if harm > 0 {
			s += harm * (0.5*fsin(v.ph*2) + 0.33*fsin(v.ph*3))
			s /= 1 + harm*0.6
		}
		if detu > 0 {
			s = (s + fsin(v.ph2)) * 0.6
		}
		if fold > 0 {
			s = foldWave(s * (1 + fold*5))
		}
		g := v.amp
		if v.age < atk {
			g *= float32(v.age) / float32(atk)
			v.age++
		}
		out[i] += drive(s*g*0.9, dist)
		v.amp *= ac
		v.pEnv *= pc
		v.ph += inc
		v.ph2 += inc * detuRatio
		if v.ph >= 1 {
			v.ph--
		}
		if v.ph2 >= 1 {
			v.ph2--
		}
	}
	if v.amp < 0.0003 && v.age >= atk {
		v.on = false
	}
	return v.on
}

// foldWave folds a signal back into -1..1.
func foldWave(x float32) float32 {
	for x > 1 || x < -1 {
		if x > 1 {
			x = 2 - x
		}
		if x < -1 {
			x = -2 - x
		}
	}
	return x
}

// gndNoise: shaped noise. TYPE sweeps low-pass, band-pass, high-pass.
type gndNoiseVoice struct {
	sr         float64
	rn         rng
	amp        float32
	age        int
	f          svf
	pink       onePoleLP
	on         bool
	vel        float32
}

func newGndNoise(sr float64) *gndNoiseVoice {
	v := &gndNoiseVoice{sr: sr}
	v.rn.seed(0x2B7E151)
	return v
}

func (v *gndNoiseVoice) trig(vel float32, p *[NSynth]float32) {
	v.on, v.vel, v.amp, v.age = true, vel, vel, 0
}

func (v *gndNoiseVoice) render(out []float32, p *[NSynth]float32) bool {
	if !v.on {
		return false
	}
	ac := decayCoef(v.sr, expMap(p[1], 0.02, 4))
	atk := maxi(1, int(p[2]*0.2*float32(v.sr)))
	v.f.set(expMap(p[0], 60, 14000), lerp(0.6, 8, p[5]), v.sr)
	colour, typ, dirt, dist := p[3], p[4], p[6], p[7]
	pa := onePoleCoef(1200, v.sr)
	for i := range out {
		n := v.rn.next()
		if dirt > 0 {
			q := 2 + (1-dirt)*14
			n = float32(math.Round(float64(n*q))) / q
		}
		n = lerp(n, v.pink.tick(n, pa)*2.2, colour)
		lp, bp, hp := v.f.tick(n)
		var s float32
		switch {
		case typ < 0.5:
			s = lerp(lp, bp, typ*2)
		default:
			s = lerp(bp, hp, (typ-0.5)*2)
		}
		g := v.amp
		if v.age < atk {
			g *= float32(v.age) / float32(atk)
			v.age++
		}
		out[i] += drive(s*g*0.9, dist)
		v.amp *= ac
	}
	if v.amp < 0.0003 && v.age >= atk {
		v.on = false
	}
	return v.on
}

// gndSq: a PWM pulse synth with a filter envelope. The bass and lead voice.
type gndSqVoice struct {
	sr           float64
	p1, p2       float32
	amp, fenv    float32
	age          int
	f            svf
	on           bool
}

func newGndSq(sr float64) *gndSqVoice { return &gndSqVoice{sr: sr} }

func (v *gndSqVoice) trig(vel float32, p *[NSynth]float32) {
	v.on, v.amp, v.fenv, v.age = true, vel, 1, 0
}

func sawBL(t, dt float32) float32 { return 2*t - 1 - polyBlep(t, dt) }

func (v *gndSqVoice) render(out []float32, p *[NSynth]float32) bool {
	if !v.on {
		return false
	}
	invSR := float32(1 / v.sr)
	base := expMap(p[0], 20, 1800)
	dec := expMap(p[1], 0.04, 3)
	ac := decayCoef(v.sr, dec)
	fc := decayCoef(v.sr, maxf(dec*0.6, 0.03))
	atk := maxi(1, int(p[2]*0.05*float32(v.sr)))
	pw := lerp(0.5, 0.95, p[3])
	cutBase := expMap(p[4], 80, 9000)
	q := lerp(0.7, 9, p[5])
	detu := float32(math.Pow(2, float64(p[6])*0.4/12))
	dist := p[7]
	inc1 := base * invSR
	inc2 := inc1 * detu
	if inc1 > 0.4 {
		inc1 = 0.4
	}
	if inc2 > 0.4 {
		inc2 = 0.4
	}
	for i := range out {
		if i&15 == 0 {
			v.f.set(cutBase*(1+v.fenv*3.5), q, v.sr)
		}
		t2 := v.p1 + pw
		if t2 >= 1 {
			t2--
		}
		t4 := v.p2 + pw
		if t4 >= 1 {
			t4--
		}
		s1 := sawBL(v.p1, inc1) - sawBL(t2, inc1)
		s2 := sawBL(v.p2, inc2) - sawBL(t4, inc2)
		lp, _, _ := v.f.tick((s1 + s2) * 0.5)
		g := v.amp
		if v.age < atk {
			g *= float32(v.age) / float32(atk)
			v.age++
		}
		out[i] += drive(lp*g*0.9, dist)
		v.amp *= ac
		v.fenv *= fc
		v.p1 += inc1
		v.p2 += inc2
		if v.p1 >= 1 {
			v.p1--
		}
		if v.p2 >= 1 {
			v.p2--
		}
	}
	if v.amp < 0.0003 && v.age >= atk {
		v.on = false
	}
	return v.on
}

// ------------------------------------------------------------------- UW

// UserWave is an imported sample: mono, floating point, immutable once made
// (the audio thread may hold a pointer while the bank swaps in a new wave).
type UserWave struct {
	Name string
	Rate int
	Data []float32
}

// waveSource finds a loaded wave by slot. Engine implements it.
type waveSource interface {
	waveAt(slot int) *UserWave
}

// uwVoice plays a UserWave with pitch, start/end, loop, reverse and decay.
type uwVoice struct {
	sr               float64
	src              waveSource
	w                *UserWave
	pos, start, end  float64
	rev, loop        bool
	amp              float32
	hold, age, fade  int
	on               bool
}

func newUW(sr float64, src waveSource) *uwVoice { return &uwVoice{sr: sr, src: src} }

func (v *uwVoice) trig(vel float32, p *[NSynth]float32) {
	slot := int(p[0]*127/2 + 0.001)
	if slot >= NumWaves {
		slot = NumWaves - 1
	}
	v.w = v.src.waveAt(slot)
	if v.w == nil || len(v.w.Data) < 2 {
		v.on = false
		return
	}
	n := float64(len(v.w.Data))
	v.start = float64(p[4]) * n
	v.end = float64(p[5]) * n
	if v.end < v.start+32 {
		v.end = v.start + 32
	}
	if v.end > n {
		v.end = n
		if v.start > v.end-32 {
			v.start = maxf64(0, v.end-32)
		}
	}
	v.rev = p[7] > 0.5
	v.loop = p[6] > 0.5
	if v.rev {
		v.pos = v.end - 1
	} else {
		v.pos = v.start
	}
	v.amp = vel
	v.hold = int(p[3] * float32(v.sr))
	v.age = 0
	v.fade = int(0.002 * v.sr)
	v.on = true
}

func hermite(y0, y1, y2, y3, t float32) float32 {
	c0 := y1
	c1 := 0.5 * (y2 - y0)
	c2 := y0 - 2.5*y1 + 2*y2 - 0.5*y3
	c3 := 0.5*(y3-y0) + 1.5*(y1-y2)
	return ((c3*t+c2)*t+c1)*t + c0
}

func maxf64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func (v *uwVoice) render(out []float32, p *[NSynth]float32) bool {
	if !v.on || v.w == nil {
		return false
	}
	d := v.w.Data
	n := len(d)
	semis := (p[1] - 0.5) * 48
	rate := math.Pow(2, float64(semis)/12) * float64(v.w.Rate) / v.sr
	step := rate
	if v.rev {
		step = -rate
	}
	var ac float32 = 1
	if p[2] < 0.97 {
		ac = decayCoef(v.sr, expMap(p[2]/0.97, 0.005, 6))
	}
	at := func(i int) float32 {
		if i < 0 || i >= n {
			return 0
		}
		return d[i]
	}
	for i := range out {
		if !v.on {
			break
		}
		ip := int(math.Floor(v.pos))
		fr := float32(v.pos - float64(ip))
		s := hermite(at(ip-1), at(ip), at(ip+1), at(ip+2), fr)
		g := v.amp
		if v.age < v.fade {
			g *= float32(v.age) / float32(v.fade)
		}
		v.age++
		// Fade out over the last 2 ms when not looping.
		if !v.loop {
			var left float64
			if v.rev {
				left = v.pos - v.start
			} else {
				left = v.end - v.pos
			}
			if fl := float64(v.fade) * rate; left < fl && fl > 0 {
				g *= float32(left / fl)
			}
		}
		out[i] += s * g
		if v.hold > 0 {
			v.hold--
		} else {
			v.amp *= ac
		}
		v.pos += step
		if v.rev {
			if v.pos < v.start {
				if v.loop {
					v.pos = v.end - 1
				} else {
					v.on = false
				}
			}
		} else if v.pos >= v.end {
			if v.loop {
				v.pos = v.start + (v.pos - v.end)
			} else {
				v.on = false
			}
		}
		if v.amp < 0.0003 {
			v.on = false
		}
	}
	return v.on
}
