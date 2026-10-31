package engine

import "math"

// The TRX family: analog-style drum voices. Every voice is mono and renders
// additively into the block it is given. p holds the eight synth parameters
// of the machine, normalised 0..1, and is re-read every call, so a parameter
// can be moved, locked, slid or modulated while a note rings.

// ---------------------------------------------------------------- kick, tom

type kickCfg struct {
	fLo, fHi     float32 // PTCH range, Hz
	decLo, decHi float32 // DEC range, seconds to -60 dB
	rampOct      float32 // pitch drop at RAMP = 1, octaves
	pDecay       float32 // pitch envelope length, seconds to -60 dB
	holdMax      float32 // HOLD at 1, seconds
	clickHz      float32 // click high-pass corner
	noiseHz      float32 // noise low-pass corner
}

type kickVoice struct {
	cfg                   kickCfg
	sr                    float64
	rn                    rng
	phase                 float32
	amp, pEnv, nEnv, cEnv float32
	hold, age             int
	clickLP               onePoleLP
	nf                    svf
	on                    bool
}

func newKick(cfg kickCfg, sr float64) *kickVoice {
	v := &kickVoice{cfg: cfg, sr: sr}
	v.rn.seed(0x1234567)
	v.nf.set(cfg.noiseHz, 0.7, sr)
	return v
}

func (v *kickVoice) trig(vel float32, p *[NSynth]float32) {
	v.on = true
	v.amp, v.pEnv, v.nEnv, v.cEnv = vel, 1, vel, vel
	v.hold = int(p[3] * v.cfg.holdMax * float32(v.sr))
	v.phase, v.age = 0.25, 0
}

func (v *kickVoice) render(out []float32, p *[NSynth]float32) bool {
	if !v.on {
		return false
	}
	c := &v.cfg
	invSR := float32(1 / v.sr)
	base := expMap(p[0], c.fLo, c.fHi)
	ac := decayCoef(v.sr, expMap(p[1], c.decLo, c.decHi))
	pc := decayCoef(v.sr, c.pDecay)
	nc := decayCoef(v.sr, 0.07)
	cc := decayCoef(v.sr, 0.005)
	ramp := p[2] * c.rampOct
	clickAmt, noiseAmt, dirt, dist := p[4], p[5], p[6], p[7]
	clickA := onePoleCoef(c.clickHz, v.sr)
	var inc float32
	for i := range out {
		if i&7 == 0 {
			inc = base * invSR * float32(math.Exp2(float64(ramp*v.pEnv)))
		}
		ph := v.phase
		s := fsin(ph+dirt*0.5*fsin(ph)) * v.amp
		n := v.rn.next()
		s += (n - v.clickLP.tick(n, clickA)) * v.cEnv * clickAmt * 0.8
		lp, _, _ := v.nf.tick(v.rn.next())
		s += lp * v.nEnv * noiseAmt * 1.2
		if v.age < 32 {
			s *= float32(v.age) / 32
			v.age++
		}
		out[i] += drive(s*0.9, dist)

		if v.hold > 0 {
			v.hold--
		} else {
			v.amp *= ac
		}
		v.pEnv *= pc
		v.nEnv *= nc
		v.cEnv *= cc
		v.phase += inc
		if v.phase >= 1 {
			v.phase--
		}
	}
	if v.amp < 0.0003 && v.hold == 0 {
		v.on = false
	}
	return v.on
}

// -------------------------------------------------------------------- snare

type snareVoice struct {
	sr                            float64
	rn                            rng
	ph1, ph2                      float32
	bodyEnv, noiseEnv, snapEnv, pEnv float32
	nf                            svf
	on                            bool
}

func newSnare(sr float64) *snareVoice {
	v := &snareVoice{sr: sr}
	v.rn.seed(0x7654321)
	return v
}

func (v *snareVoice) trig(vel float32, p *[NSynth]float32) {
	v.on = true
	v.bodyEnv, v.noiseEnv, v.snapEnv, v.pEnv = vel, vel, vel, 1
	v.ph1, v.ph2 = 0.25, 0.25
}

func (v *snareVoice) render(out []float32, p *[NSynth]float32) bool {
	if !v.on {
		return false
	}
	invSR := float32(1 / v.sr)
	base := expMap(p[0], 110, 420)
	dec := expMap(p[1], 0.06, 0.75)
	bc := decayCoef(v.sr, maxf(dec*0.5, 0.04))
	nc := decayCoef(v.sr, dec)
	sc := decayCoef(v.sr, 0.004)
	pc := decayCoef(v.sr, 0.09)
	ratio := lerp(1.5, 2.6, p[2])
	v.nf.set(expMap(p[3], 700, 7000), 0.9, v.sr)
	noiseAmt, snap, ramp, dist := p[4], p[5], p[6], p[7]
	var inc1 float32
	for i := range out {
		if i&7 == 0 {
			inc1 = base * invSR * float32(math.Exp2(float64(ramp*v.pEnv)))
		}
		body := (fsin(v.ph1) + 0.55*fsin(v.ph2)) * v.bodyEnv * 0.5
		n := v.rn.next()
		_, _, hp := v.nf.tick(n)
		s := body + hp*v.noiseEnv*noiseAmt*0.8 + n*v.snapEnv*snap*0.5
		out[i] += drive(s, dist)

		v.bodyEnv *= bc
		v.noiseEnv *= nc
		v.snapEnv *= sc
		v.pEnv *= pc
		v.ph1 += inc1
		v.ph2 += inc1 * ratio
		if v.ph1 >= 1 {
			v.ph1--
		}
		if v.ph2 >= 1 {
			v.ph2--
		}
	}
	if v.noiseEnv < 0.0003 && v.bodyEnv < 0.0003 {
		v.on = false
	}
	return v.on
}

// --------------------------------------------------------------------- clap

type clapVoice struct {
	sr                        float64
	rn                        rng
	t, nextBurst, burstsLeft  int
	burstEnv, tailEnv, velocity float32
	bp                        svf
	on                        bool
}

func newClap(sr float64) *clapVoice {
	v := &clapVoice{sr: sr}
	v.rn.seed(0x2468ACE)
	return v
}

func (v *clapVoice) trig(vel float32, p *[NSynth]float32) {
	v.on = true
	v.t, v.nextBurst = 0, 0
	v.burstsLeft = 1 + int(p[2]*7+0.5)
	v.burstEnv, v.tailEnv, v.velocity = 0, 0, vel
}

func (v *clapVoice) render(out []float32, p *[NSynth]float32) bool {
	if !v.on {
		return false
	}
	spread := int(lerp(0.004, 0.03, p[3]) * float32(v.sr))
	bc := decayCoef(v.sr, 0.012)
	tc := decayCoef(v.sr, expMap(p[1], 0.05, 0.9))
	tailLevel := p[4] * 0.8
	v.bp.set(expMap(p[0], 600, 3200), lerp(0.7, 6, p[5]), v.sr)
	dirt, dist := p[6], p[7]
	for i := range out {
		if v.burstsLeft > 0 && v.t >= v.nextBurst {
			v.burstEnv = v.velocity
			v.burstsLeft--
			v.nextBurst += spread
			if v.burstsLeft == 0 {
				v.tailEnv = v.velocity * tailLevel
			}
		}
		n := v.rn.next()
		if dirt > 0 { // dirt: coarse, crackly noise
			n = float32(math.Round(float64(n*(2+(1-dirt)*14)))) / (2 + (1-dirt)*14)
		}
		_, bp, _ := v.bp.tick(n)
		out[i] += drive(bp*(v.burstEnv+v.tailEnv)*1.6, dist)
		v.burstEnv *= bc
		v.tailEnv *= tc
		v.t++
	}
	if v.burstsLeft == 0 && v.tailEnv < 0.0003 && v.burstEnv < 0.0003 {
		v.on = false
	}
	return v.on
}

// ---------------------------------------------------------------- rimshot

type rimVoice struct {
	sr                    float64
	rn                    rng
	ph1, ph2              float32
	e1, e2, ce            float32
	bp                    svf
	on                    bool
}

func newRim(sr float64) *rimVoice {
	v := &rimVoice{sr: sr}
	v.rn.seed(0x1357BDF)
	return v
}

func (v *rimVoice) trig(vel float32, p *[NSynth]float32) {
	v.on = true
	v.e1, v.e2, v.ce = vel, vel, vel
	v.ph1, v.ph2 = 0.25, 0.25
}

func (v *rimVoice) render(out []float32, p *[NSynth]float32) bool {
	if !v.on {
		return false
	}
	invSR := float32(1 / v.sr)
	f1 := expMap(p[0], 250, 1600)
	f2 := f1 * lerp(1.3, 2.7, p[2])
	dec := expMap(p[1], 0.012, 0.35)
	c1 := decayCoef(v.sr, dec)
	c2 := decayCoef(v.sr, dec*lerp(0.4, 1.4, p[6]))
	cc := decayCoef(v.sr, 0.003)
	v.bp.set(expMap(p[5], 800, 6000), 1.4, v.sr)
	click, noise, dist := p[3], p[4], p[7]
	for i := range out {
		n := v.rn.next()
		_, bp, _ := v.bp.tick(n)
		s := fsin(v.ph1)*v.e1*0.6 + fsin(v.ph2)*v.e2*0.4
		s += n*v.ce*click*0.7 + bp*v.e1*noise*0.9
		out[i] += drive(s, dist)
		v.e1 *= c1
		v.e2 *= c2
		v.ce *= cc
		v.ph1 += f1 * invSR
		v.ph2 += f2 * invSR
		if v.ph1 >= 1 {
			v.ph1--
		}
		if v.ph2 >= 1 {
			v.ph2--
		}
	}
	if v.e1 < 0.0003 && v.e2 < 0.0003 {
		v.on = false
	}
	return v.on
}

// ----------------------------------------------------------------- cowbell

type cowbellVoice struct {
	sr             float64
	rn             rng
	o1, o2         sqOsc
	main, punch    float32
	bp             svf
	on             bool
}

func newCowbell(sr float64) *cowbellVoice {
	v := &cowbellVoice{sr: sr}
	v.rn.seed(0x0F0F0F1)
	return v
}

func (v *cowbellVoice) trig(vel float32, p *[NSynth]float32) {
	v.on = true
	v.main, v.punch = vel, vel
	v.o1.phase, v.o2.phase = 0, 0
}

func (v *cowbellVoice) render(out []float32, p *[NSynth]float32) bool {
	if !v.on {
		return false
	}
	invSR := float32(1 / v.sr)
	f1 := expMap(p[0], 300, 1000)
	f2 := f1 * lerp(1.25, 1.75, p[2])
	mc := decayCoef(v.sr, expMap(p[1], 0.04, 0.9))
	pc := decayCoef(v.sr, 0.02)
	v.bp.set(expMap(p[3], 500, 6500), lerp(0.8, 5, 1-p[5]), v.sr)
	punch, noise, dist := p[4], p[6], p[7]
	for i := range out {
		sq := (v.o1.tick(f1*invSR) + v.o2.tick(f2*invSR)) * 0.5
		sq += v.rn.next() * noise * 0.4
		_, bp, _ := v.bp.tick(sq)
		env := v.main*0.55 + v.punch*punch*0.9
		out[i] += drive(bp*env*1.5, dist)
		v.main *= mc
		v.punch *= pc
	}
	if v.main < 0.0003 {
		v.on = false
	}
	return v.on
}

// -------------------------------------------------- hi-hat, cymbal (metal)

type hatCfg struct {
	fLo, fHi     float32 // PTCH range: scales the six oscillators
	decLo, decHi float32
	hpLo, hpHi   float32 // TONE range: high-pass corner
}

// The six oscillator ratios of the classic metallic hat circuit.
var hatRatios = [6]float32{1, 1.4829, 1.8002, 2.5463, 2.6307, 3.8970}

type hatVoice struct {
	cfg     hatCfg
	sr      float64
	rn      rng
	osc     [6]sqOsc
	env, a  float32
	age     int
	hp, bp  svf
	on      bool
}

func newHat(cfg hatCfg, sr float64) *hatVoice {
	v := &hatVoice{cfg: cfg, sr: sr}
	v.rn.seed(0x5A5A5A5)
	return v
}

func (v *hatVoice) trig(vel float32, p *[NSynth]float32) {
	v.on = true
	v.env, v.age = vel, 0
	for i := range v.osc {
		v.osc[i].phase = float32(i) * 0.137
	}
}

func (v *hatVoice) render(out []float32, p *[NSynth]float32) bool {
	if !v.on {
		return false
	}
	c := &v.cfg
	invSR := float32(1 / v.sr)
	f0 := expMap(p[0], c.fLo, c.fHi)
	spread := lerp(0.85, 1.25, p[6])
	dc := decayCoef(v.sr, expMap(p[1], c.decLo, c.decHi))
	v.hp.set(expMap(p[2], c.hpLo, c.hpHi), 0.9, v.sr)
	v.bp.set(f0*lerp(8, 14, p[3]), lerp(0.6, 2.5, p[3]), v.sr)
	atkSamples := int(p[4] * 0.006 * float32(v.sr))
	noiseAmt, dist := p[5], p[7]
	var incs [6]float32
	for k := range incs {
		incs[k] = f0 * invSR * (1 + (hatRatios[k]-1)*spread)
	}
	for i := range out {
		var sum float32
		for k := range v.osc {
			sum += v.osc[k].tick(incs[k])
		}
		s := sum*(1.0/6) + v.rn.next()*noiseAmt*0.6
		_, bp, _ := v.bp.tick(s)
		_, _, hp := v.hp.tick(s*0.5 + bp*0.9)
		g := v.env
		if v.age < atkSamples {
			g *= float32(v.age) / float32(atkSamples)
			v.age++
		}
		out[i] += drive(hp*g*3.2, dist)
		v.env *= dc
	}
	if v.env < 0.0003 {
		v.on = false
	}
	return v.on
}

// ----------------------------------------------------------------- maracas

type maracaVoice struct {
	sr         float64
	rn         rng
	t          int
	env, atk   float32
	second     bool
	svfN       svf
	on         bool
	vel        float32
	shakeAt    int
}

func newMaraca(sr float64) *maracaVoice {
	v := &maracaVoice{sr: sr}
	v.rn.seed(0x3C3C3C3)
	return v
}

func (v *maracaVoice) trig(vel float32, p *[NSynth]float32) {
	v.on = true
	v.t, v.vel, v.env, v.second = 0, vel, 0, false
	v.shakeAt = int(lerp(0.012, 0.03, p[5]) * float32(v.sr))
}

func (v *maracaVoice) render(out []float32, p *[NSynth]float32) bool {
	if !v.on {
		return false
	}
	atkN := maxi(1, int(lerp(0.0008, 0.03, p[0])*float32(v.sr)))
	dc := decayCoef(v.sr, expMap(p[1], 0.012, 0.2))
	v.svfN.set(expMap(p[2], 2500, 10000), lerp(0.7, 4, p[3]), v.sr)
	colr, shake, dirt, dist := p[4], p[5], p[6], p[7]
	for i := range out {
		// Attack ramp: a rattle swells, it does not click.
		if v.t < atkN && !v.second {
			v.env = v.vel * float32(v.t) / float32(atkN)
		} else if v.t >= atkN {
			v.env *= dc
		}
		if shake > 0.05 && !v.second && v.t >= v.shakeAt {
			v.second = true
			v.env = v.vel * shake * 0.9
		}
		n := v.rn.next()
		if dirt > 0 {
			q := 2 + (1-dirt)*14
			n = float32(math.Round(float64(n*q))) / q
		}
		lp, bp, hp := v.svfN.tick(n)
		s := hp*(1-colr) + bp*colr*1.5 + lp*0*colr
		out[i] += drive(s*v.env*1.3, dist)
		v.t++
	}
	if v.t > atkN && v.env < 0.0003 && (v.second || shake <= 0.05) {
		v.on = false
	}
	return v.on
}

// ------------------------------------------------------------------ claves

type claveVoice struct {
	sr           float64
	rn           rng
	ph           float32
	env, cenv, pEnv float32
	bp           svf
	on           bool
}

func newClave(sr float64) *claveVoice {
	v := &claveVoice{sr: sr}
	v.rn.seed(0x6B6B6B6)
	return v
}

func (v *claveVoice) trig(vel float32, p *[NSynth]float32) {
	v.on = true
	v.env, v.cenv, v.pEnv = vel, vel, 1
	v.ph = 0.25
}

func (v *claveVoice) render(out []float32, p *[NSynth]float32) bool {
	if !v.on {
		return false
	}
	invSR := float32(1 / v.sr)
	base := expMap(p[0], 700, 4200)
	dc := decayCoef(v.sr, expMap(p[1], 0.01, 0.25))
	cc := decayCoef(v.sr, 0.004)
	pc := decayCoef(v.sr, 0.05)
	v.bp.set(expMap(p[2], 1200, 8000), 2.2, v.sr)
	click, detu, ramp, noise, dist := p[3], p[4], p[5]*0.8, p[6], p[7]
	var inc float32
	for i := range out {
		if i&7 == 0 {
			inc = base * invSR * float32(math.Exp2(float64(ramp*v.pEnv)))
		}
		n := v.rn.next()
		_, bp, _ := v.bp.tick(n)
		s := fsin(v.ph) * v.env * 0.7
		s += fsin(v.ph*(1.5+detu*0.9)) * v.env * 0.2 * detu
		s += n*v.cenv*click*0.6 + bp*v.cenv*noise*0.8
		out[i] += drive(s, dist)
		v.env *= dc
		v.cenv *= cc
		v.pEnv *= pc
		v.ph += inc
		if v.ph >= 1 {
			v.ph--
		}
	}
	if v.env < 0.0003 {
		v.on = false
	}
	return v.on
}

func maxf(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func maxi(a, b int) int {
	if a > b {
		return a
	}
	return b
}
