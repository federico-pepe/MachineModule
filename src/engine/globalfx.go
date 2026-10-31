package engine

import "math"

// Indices of the 16 global effect parameters (stored in each kit).
const (
	GDTIM = iota // delay time (free), ms
	GDSYN        // delay sync: 0 free, 1..7 = tempo divisions
	GDFB         // delay feedback
	GDLP         // delay feedback damping (low-pass)
	GDLEV        // delay return level
	GRDEC        // reverb decay
	GRDMP        // reverb damping
	GRLEV        // reverb return level
	GCTHR        // compressor threshold
	GCRAT        // compressor ratio
	GCREL        // compressor release
	GCMKP        // compressor make-up gain
	GCMIX        // compressor dry/wet
	GELG         // master EQ low shelf gain
	GEHG         // master EQ high shelf gain
	GMVOL        // master volume
)

// GlobalNames are the display names of the global parameters.
var GlobalNames = [NGlobal]string{
	"DTIM", "DSYN", "DFB", "DLP", "DLEV", "RDEC", "RDMP", "RLEV",
	"CTHR", "CRAT", "CREL", "CMKP", "CMIX", "ELG", "EHG", "MVOL",
}

// DefaultGlobal is the starting point of every kit's global effects.
var DefaultGlobal = [NGlobal]uint8{
	50, 48, 52, 90, 90, 80, 60, 90,
	90, 40, 50, 40, 90, 64, 64, 88,
}

// delaySyncBeats is what DSYN 1..7 mean, in beats (quarter notes).
var delaySyncBeats = [8]float32{0, 0.25, 0.5, 0.75, 1, 1.5, 2, 3}

// freeverb constants (Jezar, public domain), for 44.1 kHz.
var (
	combTuning    = [8]int{1116, 1188, 1277, 1356, 1422, 1491, 1557, 1617}
	allpassTuning = [4]int{556, 441, 341, 225}
	stereoSpread  = 23
)

type comb struct {
	buf   []float32
	i     int
	store float32
}

func (c *comb) tick(x, fb, damp float32) float32 {
	y := c.buf[c.i]
	c.store = y*(1-damp) + c.store*damp
	c.buf[c.i] = x + c.store*fb
	c.i++
	if c.i == len(c.buf) {
		c.i = 0
	}
	return y
}

type allpass struct {
	buf []float32
	i   int
}

func (a *allpass) tick(x float32) float32 {
	b := a.buf[a.i]
	y := -x + b
	a.buf[a.i] = x + b*0.5
	a.i++
	if a.i == len(a.buf) {
		a.i = 0
	}
	return y
}

// globalFX is the shared delay, reverb, compressor, master EQ and limiter.
type globalFX struct {
	sr float64

	// delay
	dl, dr       *delayLine
	delayTime    float32 // smoothed, samples
	dLP          [2]onePoleLP
	dHP          [2]onePoleLP
	dHPcoef      float32
	// reverb
	combL, combR [8]comb
	apL, apR     [4]allpass
	pre          *delayLine
	// compressor
	env float32
	// master EQ
	lo, hi [2]biquad
	loOn   bool
	hiOn   bool
}

func newGlobalFX(sr float64) *globalFX {
	g := &globalFX{sr: sr}
	g.dl = newDelayLine(int(sr*3.2) + 8)
	g.dr = newDelayLine(int(sr*3.2) + 8)
	g.dHPcoef = onePoleCoef(130, sr)
	scale := sr / 44100
	for i := range g.combL {
		g.combL[i].buf = make([]float32, int(float64(combTuning[i])*scale))
		g.combR[i].buf = make([]float32, int(float64(combTuning[i]+stereoSpread)*scale))
	}
	for i := range g.apL {
		g.apL[i].buf = make([]float32, int(float64(allpassTuning[i])*scale))
		g.apR[i].buf = make([]float32, int(float64(allpassTuning[i]+stereoSpread)*scale))
	}
	g.pre = newDelayLine(int(sr*0.12) + 8)
	return g
}

func (g *globalFX) reset() {
	g.dl.clear()
	g.dr.clear()
	g.pre.clear()
	for i := range g.combL {
		for j := range g.combL[i].buf {
			g.combL[i].buf[j] = 0
		}
		for j := range g.combR[i].buf {
			g.combR[i].buf[j] = 0
		}
		g.combL[i].store, g.combR[i].store = 0, 0
	}
	for i := range g.apL {
		for j := range g.apL[i].buf {
			g.apL[i].buf[j] = 0
		}
		for j := range g.apR[i].buf {
			g.apR[i].buf[j] = 0
		}
	}
	g.env = 0
	for i := 0; i < 2; i++ {
		g.lo[i].reset()
		g.hi[i].reset()
		g.dLP[i].y, g.dHP[i].y = 0, 0
	}
}

// limiter is a soft knee at 0.7: linear below, saturating to 1 above.
func limiter(x float32) float32 {
	a := absf(x)
	if a < 0.7 {
		return x
	}
	y := 0.7 + 0.3*softClip((a-0.7)/0.3)
	if x < 0 {
		return -y
	}
	return y
}

// process mixes the dry buses with the delay and reverb returns, then
// compresses, shapes and limits. All slices have n samples; the result is
// written to outL/outR (which may alias dryL/dryR).
func (g *globalFX) process(gp *[NGlobal]uint8, bpm float64, n int,
	dryL, dryR, sendD, sendR, outL, outR []float32) {

	f := func(i int) float32 { return float32(gp[i]) / 127 }

	// delay
	target := expMap(f(GDTIM), 5, 1500) * float32(g.sr) / 1000
	if syn := int(gp[GDSYN]) / 16; syn > 0 && syn < len(delaySyncBeats) {
		target = delaySyncBeats[syn] * float32(60/bpm) * float32(g.sr)
	}
	fb := f(GDFB) * 0.92
	dlpA := onePoleCoef(expMap(f(GDLP), 500, 16000), g.sr)
	dlev := f(GDLEV)

	// reverb
	room := 0.7 + f(GRDEC)*0.28
	damp := f(GRDMP) * 0.5
	rlev := f(GRLEV)
	preSamp := float32(g.sr * 0.012)

	// compressor
	thrDB := -(1 - f(GCTHR)) * 40
	ratio := 1 + f(GCRAT)*f(GCRAT)*19
	atkC := float32(1 - math.Exp(-1/(0.004*g.sr)))
	relC := float32(1 - math.Exp(-1/(float64(expMap(f(GCREL), 0.03, 1.0))*g.sr)))
	makeup := dbToLin(f(GCMKP) * 12)
	cmix := f(GCMIX)

	// master EQ
	loG := (f(GELG) - 0.5) * 24
	hiG := (f(GEHG) - 0.5) * 24
	g.loOn, g.hiOn = absf(loG) > 0.2, absf(hiG) > 0.2
	if g.loOn {
		g.lo[0].lowShelf(120, loG, g.sr)
		g.lo[1] = g.lo[0].withState(g.lo[1])
	}
	if g.hiOn {
		g.hi[0].highShelf(8000, hiG, g.sr)
		g.hi[1] = g.hi[0].withState(g.hi[1])
	}
	mvol := f(GMVOL)
	mgain := mvol * mvol * 1.4

	for i := 0; i < n; i++ {
		// smooth the delay time so changes glide instead of click
		g.delayTime += (target - g.delayTime) * 0.0008
		dTime := g.delayTime

		l := g.dl.read(dTime)
		r := g.dr.read(dTime)
		fl := g.dLP[0].tick(l, dlpA)
		fr := g.dLP[1].tick(r, dlpA)
		fl -= g.dHP[0].tick(fl, g.dHPcoef)
		fr -= g.dHP[1].tick(fr, g.dHPcoef)
		in := sendD[i]
		g.dl.write(in + fr*fb) // ping-pong: each side feeds the other
		g.dr.write(fl * fb)
		dOutL := l * dlev
		dOutR := r * dlev

		// reverb
		g.pre.write(sendR[i])
		rin := g.pre.read(preSamp) * 0.015
		var rl, rr float32
		for c := 0; c < 8; c++ {
			rl += g.combL[c].tick(rin, room, damp)
			rr += g.combR[c].tick(rin, room, damp)
		}
		for a := 0; a < 4; a++ {
			rl = g.apL[a].tick(rl)
			rr = g.apR[a].tick(rr)
		}
		rl *= rlev * 3
		rr *= rlev * 3

		L := dryL[i] + dOutL + rl
		R := dryR[i] + dOutR + rr

		// compressor (linked)
		det := absf(L)
		if a := absf(R); a > det {
			det = a
		}
		if det > g.env {
			g.env += (det - g.env) * atkC
		} else {
			g.env += (det - g.env) * relC
		}
		if cmix > 0.002 && g.env > 1e-5 {
			envDB := 20 * float32(math.Log10(float64(g.env)))
			over := envDB - thrDB
			gain := float32(1)
			if over > 0 {
				gain = dbToLin(-over * (1 - 1/ratio))
			}
			gain *= makeup
			L = L + (L*gain-L)*cmix
			R = R + (R*gain-R)*cmix
		}

		if g.loOn {
			L = g.lo[0].tick(L)
			R = g.lo[1].tick(R)
		}
		if g.hiOn {
			L = g.hi[0].tick(L)
			R = g.hi[1].tick(R)
		}
		L = limiter(L * mgain)
		R = limiter(R * mgain)
		if isBad(L) || isBad(R) {
			L, R = 0, 0
			g.reset()
		}
		outL[i], outR[i] = L, R
	}
}

// withState copies the coefficients of b into a fresh biquad that keeps the
// filter memory of old (so the right channel keeps its own state).
func (b biquad) withState(old biquad) biquad {
	b.z1, b.z2 = old.z1, old.z2
	return b
}
