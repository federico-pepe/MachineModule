package engine

// Sizes of the model. They follow the Machinedrum: 16 tracks, up to 64
// steps, 128 patterns, 64 kits.
const (
	NumTracks   = 16
	MaxSteps    = 64
	NumPatterns = 128 // 8 banks (A-H) of 16
	NumKits     = 64
	NumWaves    = 64 // UserWave slots
	NumSongs    = 16
	MaxSongRows = 64

	NSynth  = 8 // synth parameters (the machine's own)
	NFX     = 8 // effect parameters
	NRoute  = 8 // routing and LFO parameters
	NParams = NSynth + NFX + NRoute

	NGlobal = 16 // global effect parameters, stored in the kit
)

// Parameter indices for the effect and routing pages. The synth page uses
// 0..7, named by the machine.
const (
	AMD  = NSynth + iota // amplitude modulation depth
	AMF                  // amplitude modulation frequency
	EQF                  // equaliser frequency
	EQG                  // equaliser gain
	FLTF                 // filter base (high-pass) frequency
	FLTW                 // filter width (low-pass sits this far above)
	FLTQ                 // filter resonance
	SRR                  // sample rate reduction
)

const (
	DEL  = NSynth + NFX + iota // delay send
	REV                        // reverb send
	PAN                        // pan
	VOL                        // volume
	LFOP                       // LFO destination (which parameter)
	LFOS                       // LFO speed
	LFOD                       // LFO depth
	LFOM                       // LFO shape
)

// FXNames are the display names of parameters 8..23.
var FXNames = [NFX + NRoute]string{
	"AMD", "AMF", "EQF", "EQG", "FLTF", "FLTW", "FLTQ", "SRR",
	"DEL", "REV", "PAN", "VOL", "LFOP", "LFOS", "LFOD", "LFOM",
}

// DefaultFXRoute is the neutral value of parameters 8..23: everything off or
// flat, centred, volume at a sensible level.
var DefaultFXRoute = [NFX + NRoute]uint8{
	0, 64, 64, 64, 0, 127, 0, 0,
	0, 0, 64, 100, 0, 64, 0, 0,
}

// Machine is one synth model: a name, the names and defaults of its eight
// parameters, and a way to make a voice.
type Machine struct {
	ID       int
	Name     string // e.g. "TRX-BD"
	Group    string // TRX, EFM, PI, GND or UW
	Params   [NSynth]string
	Defaults [NSynth]uint8
	build    func(sr float64, src waveSource) voice
}

// voice renders one monophonic sound. p is the machine's eight parameters,
// normalised to 0..1.
type voice interface {
	trig(vel float32, p *[NSynth]float32)
	// render adds up to len(out) samples into out and reports whether the
	// voice is still sounding.
	render(out []float32, p *[NSynth]float32) bool
}

// Machines is every machine, in menu order. A machine's ID is its index.
var Machines []Machine

func init() { Machines = buildMachines() }

// MachineIndex finds a machine by name, or returns -1.
func MachineIndex(name string) int {
	for i := range Machines {
		if Machines[i].Name == name {
			return i
		}
	}
	return -1
}

// ParamName is the display name of parameter idx (0..23) on a machine.
func ParamName(machine, idx int) string {
	if idx < NSynth {
		if machine >= 0 && machine < len(Machines) {
			return Machines[machine].Params[idx]
		}
		return "P" + string(rune('1'+idx))
	}
	if idx < NParams {
		return FXNames[idx-NSynth]
	}
	return "?"
}

func buildMachines() []Machine {
	var ms []Machine
	add := func(name, group string, params [NSynth]string, defs [NSynth]uint8,
		build func(float64, waveSource) voice) {
		ms = append(ms, Machine{ID: len(ms), Name: name, Group: group, Params: params, Defaults: defs, build: build})
	}

	// ---- TRX: analog-style drums ----
	kickNames := [NSynth]string{"PTCH", "DEC", "RAMP", "HOLD", "TICK", "NOIS", "DIRT", "DIST"}
	bd := kickCfg{30, 120, 0.08, 1.4, 4, 0.22, 0.08, 2500, 900}
	add("TRX-BD", "TRX", kickNames, [NSynth]uint8{46, 78, 56, 8, 40, 10, 0, 20},
		func(sr float64, _ waveSource) voice { return newKick(bd, sr) })
	b2 := kickCfg{28, 95, 0.06, 1.0, 5, 0.15, 0.04, 3500, 1400}
	add("TRX-B2", "TRX", kickNames, [NSynth]uint8{40, 66, 80, 0, 60, 6, 24, 36},
		func(sr float64, _ waveSource) voice { return newKick(b2, sr) })
	xt := kickCfg{70, 450, 0.08, 0.9, 2.2, 0.12, 0.02, 3000, 1500}
	add("TRX-XT", "TRX", kickNames, [NSynth]uint8{50, 56, 50, 0, 20, 0, 0, 0},
		func(sr float64, _ waveSource) voice { return newKick(xt, sr) })
	add("TRX-SD", "TRX", [NSynth]string{"PTCH", "DEC", "HARM", "TONE", "NOIS", "SNAP", "RAMP", "DIST"},
		[NSynth]uint8{58, 44, 50, 72, 84, 50, 30, 8},
		func(sr float64, _ waveSource) voice { return newSnare(sr) })
	add("TRX-CP", "TRX", [NSynth]string{"FREQ", "DEC", "BURS", "SPRD", "TAIL", "RES", "DIRT", "DIST"},
		[NSynth]uint8{62, 50, 90, 50, 90, 42, 0, 8},
		func(sr float64, _ waveSource) voice { return newClap(sr) })
	add("TRX-RS", "TRX", [NSynth]string{"PTCH", "DEC", "DETU", "CLIK", "NOIS", "TONE", "RING", "DIST"},
		[NSynth]uint8{56, 30, 50, 70, 30, 60, 50, 10},
		func(sr float64, _ waveSource) voice { return newRim(sr) })
	add("TRX-CB", "TRX", [NSynth]string{"PTCH", "DEC", "DETU", "TONE", "PUNC", "WIDE", "NOIS", "DIST"},
		[NSynth]uint8{50, 48, 50, 60, 60, 50, 0, 8},
		func(sr float64, _ waveSource) voice { return newCowbell(sr) })
	hatNames := [NSynth]string{"PTCH", "DEC", "TONE", "BAND", "ATCK", "NOIS", "SPRD", "DIST"}
	ch := hatCfg{180, 520, 0.015, 0.5, 3000, 11000}
	add("TRX-CH", "TRX", hatNames, [NSynth]uint8{62, 24, 62, 60, 0, 20, 50, 0},
		func(sr float64, _ waveSource) voice { return newHat(ch, sr) })
	oh := hatCfg{180, 520, 0.08, 1.8, 3000, 11000}
	add("TRX-OH", "TRX", hatNames, [NSynth]uint8{62, 66, 62, 60, 0, 20, 50, 0},
		func(sr float64, _ waveSource) voice { return newHat(oh, sr) })
	cy := hatCfg{100, 400, 0.3, 4.0, 2500, 9000}
	add("TRX-CY", "TRX", hatNames, [NSynth]uint8{58, 80, 50, 70, 4, 30, 60, 0},
		func(sr float64, _ waveSource) voice { return newHat(cy, sr) })
	add("TRX-MA", "TRX", [NSynth]string{"ATCK", "DEC", "TONE", "RES", "COLR", "SHAK", "DIRT", "DIST"},
		[NSynth]uint8{20, 40, 70, 30, 30, 40, 0, 0},
		func(sr float64, _ waveSource) voice { return newMaraca(sr) })
	add("TRX-CL", "TRX", [NSynth]string{"PTCH", "DEC", "TONE", "CLIK", "DETU", "RAMP", "NOIS", "DIST"},
		[NSynth]uint8{60, 36, 60, 60, 30, 20, 10, 0},
		func(sr float64, _ waveSource) voice { return newClave(sr) })

	// ---- EFM: two-operator FM ----
	efmNames := [NSynth]string{"PTCH", "DEC", "MODR", "MODI", "MODD", "FEED", "NOIS", "DIST"}
	efm := func(name string, cfg efmCfg, defs [NSynth]uint8) {
		add(name, "EFM", efmNames, defs, func(sr float64, _ waveSource) voice { return newEFM(cfg, sr) })
	}
	efm("EFM-BD", efmCfg{fLo: 30, fHi: 130, decLo: 0.1, decHi: 1.2, rampOct: 3, pDecay: 0.15, bursts: 1},
		[NSynth]uint8{44, 70, 51, 30, 50, 10, 0, 20})
	efm("EFM-SD", efmCfg{fLo: 120, fHi: 500, decLo: 0.05, decHi: 0.6, rampOct: 1.2, pDecay: 0.08, bursts: 1},
		[NSynth]uint8{50, 50, 70, 70, 40, 30, 60, 10})
	efm("EFM-XT", efmCfg{fLo: 70, fHi: 420, decLo: 0.08, decHi: 0.8, rampOct: 1.5, pDecay: 0.1, bursts: 1},
		[NSynth]uint8{50, 54, 55, 40, 50, 10, 0, 0})
	efm("EFM-CP", efmCfg{fLo: 400, fHi: 2500, decLo: 0.04, decHi: 0.5, bursts: 4, burstGap: 0.011, hpHz: 800, pDecay: 0.05},
		[NSynth]uint8{60, 40, 80, 90, 40, 60, 70, 10})
	efm("EFM-RS", efmCfg{fLo: 300, fHi: 2200, decLo: 0.01, decHi: 0.3, bursts: 1, pDecay: 0.05},
		[NSynth]uint8{60, 30, 75, 60, 30, 40, 20, 8})
	efm("EFM-CB", efmCfg{fLo: 300, fHi: 1200, decLo: 0.05, decHi: 1.0, bursts: 1, pDecay: 0.05},
		[NSynth]uint8{52, 55, 88, 50, 60, 20, 0, 0})
	efm("EFM-CH", efmCfg{fLo: 600, fHi: 5000, decLo: 0.01, decHi: 0.6, bursts: 1, hpHz: 4000, pDecay: 0.05},
		[NSynth]uint8{70, 22, 100, 100, 30, 80, 60, 0})
	efm("EFM-CY", efmCfg{fLo: 500, fHi: 4000, decLo: 0.2, decHi: 4, bursts: 1, hpHz: 3000, pDecay: 0.05},
		[NSynth]uint8{60, 80, 96, 110, 60, 80, 70, 0})

	// ---- PI: physical-model flavoured ----
	piNames := [NSynth]string{"PTCH", "DEC", "MATR", "TENS", "DAMP", "HITP", "NOIS", "DIST"}
	pi := func(name string, cfg piCfg, defs [NSynth]uint8) {
		add(name, "PI", piNames, defs, func(sr float64, _ waveSource) voice { return newPI(cfg, sr) })
	}
	pi("PI-BD", piCfg{fLo: 30, fHi: 120, decLo: 0.1, decHi: 1.5, rampOct: 1.6, modes: 6, gain: 0.35},
		[NSynth]uint8{44, 66, 10, 70, 60, 50, 20, 10})
	pi("PI-SD", piCfg{fLo: 120, fHi: 400, decLo: 0.06, decHi: 0.8, rampOct: 0.6, buzz: 0.7, modes: 8, gain: 0.3},
		[NSynth]uint8{52, 46, 24, 40, 54, 60, 70, 8})
	pi("PI-LT", piCfg{fLo: 60, fHi: 220, decLo: 0.1, decHi: 1.2, rampOct: 0.8, modes: 8, gain: 0.32},
		[NSynth]uint8{40, 60, 10, 50, 50, 50, 10, 0})
	pi("PI-MT", piCfg{fLo: 90, fHi: 330, decLo: 0.1, decHi: 1.0, rampOct: 0.8, modes: 8, gain: 0.32},
		[NSynth]uint8{50, 56, 10, 50, 50, 50, 10, 0})
	pi("PI-HT", piCfg{fLo: 130, fHi: 480, decLo: 0.08, decHi: 0.8, rampOct: 0.8, modes: 8, gain: 0.32},
		[NSynth]uint8{60, 52, 10, 50, 50, 50, 10, 0})
	pi("PI-RS", piCfg{fLo: 200, fHi: 2000, decLo: 0.02, decHi: 0.4, rampOct: 0.2, modes: 5, gain: 0.3},
		[NSynth]uint8{60, 30, 70, 20, 70, 40, 40, 6})
	pi("PI-RC", piCfg{fLo: 300, fHi: 1500, decLo: 0.4, decHi: 4.5, rampOct: 0, modes: 10, gain: 0.2},
		[NSynth]uint8{50, 80, 118, 0, 50, 40, 30, 0})
	pi("PI-CL", piCfg{fLo: 600, fHi: 3500, decLo: 0.03, decHi: 0.5, rampOct: 0, modes: 4, gain: 0.3},
		[NSynth]uint8{60, 36, 64, 0, 90, 30, 10, 0})
	pi("PI-BL", piCfg{fLo: 200, fHi: 2500, decLo: 0.3, decHi: 5, rampOct: 0, modes: 8, gain: 0.22},
		[NSynth]uint8{60, 78, 90, 0, 30, 50, 5, 0})

	// ---- GND: generators ----
	add("GND-SIN", "GND", [NSynth]string{"PTCH", "DEC", "ATCK", "RAMP", "HARM", "FOLD", "DETU", "DIST"},
		[NSynth]uint8{40, 60, 0, 0, 0, 0, 0, 0},
		func(sr float64, _ waveSource) voice { return newGndSin(sr) })
	add("GND-NS", "GND", [NSynth]string{"FREQ", "DEC", "ATCK", "COLR", "TYPE", "RES", "DIRT", "DIST"},
		[NSynth]uint8{80, 50, 0, 0, 64, 20, 0, 0},
		func(sr float64, _ waveSource) voice { return newGndNoise(sr) })
	add("GND-SQ", "GND", [NSynth]string{"PTCH", "DEC", "ATCK", "PWM", "FILT", "RES", "DETU", "DIST"},
		[NSynth]uint8{36, 56, 0, 40, 40, 50, 20, 10},
		func(sr float64, _ waveSource) voice { return newGndSq(sr) })

	// ---- UW: UserWave sample player ----
	add("UW-SMP", "UW", [NSynth]string{"WAVE", "PTCH", "DEC", "HOLD", "STRT", "END", "LOOP", "REV"},
		[NSynth]uint8{0, 64, 127, 0, 0, 127, 0, 0},
		func(sr float64, src waveSource) voice { return newUW(sr, src) })

	return ms
}
