package engine

import (
	"errors"
	"io"
	"math"
	"sync"
	"time"
)

// maxChunk is the largest piece Render works on at once; scratch buffers are
// this long.
const maxChunk = 512

// Hit levels (voice velocity) of a plain trig and of an accented one.
const (
	velNormal = 0.72
	velFull   = 1.0
)

// SyncMode says what drives the sequencer clock.
type SyncMode int

const (
	SyncInternal SyncMode = iota // own tempo
	SyncMIDI                     // MIDI clock from Live or a drum machine
)

// Position is where the sequencer is, for the screens.
type Position struct {
	Playing bool
	Step    int // 0-based step of the pattern playing (the last one fired)
	Pattern int // pattern index 0..127
	Next    int // queued pattern, -1 none
	Row     int // song row, -1 when not in song mode
	Repeat  int // repeats of the row played so far
	Tick    float64
}

// Engine is the whole instrument. One mutex guards it: Render holds it for a
// block, every other method for a few microseconds.
type Engine struct {
	mu sync.Mutex
	sr float64

	proj *Project
	kit  *Kit // the working kit (what you hear and edit)
	kitN int  // slot the working kit came from, -1 none

	tracks [NumTracks]track
	waves  [NumWaves]*UserWave
	fx     *globalFX

	// transport
	bpm      float64
	playing  bool
	tick     float64 // ticks since the pattern started
	nextStep int
	playStep int
	curPat   int
	queued   int
	songMode bool
	song     int
	songRow  int
	songRep  int
	rec      bool

	// external sync
	sync     SyncMode
	extTicks float64
	extTarget float64
	extLast  time.Time
	extBPM   float64

	// scratch
	buf, mixL, mixR, sendD, sendR []float32
	meter                         float32

	// a hook the host may set to hear about changes it should redraw or save
	dirty bool
}

// New makes an engine at the given sample rate with a fresh project: the
// default kit, empty patterns, no waves.
func New(sr float64) *Engine {
	if sr < 8000 {
		sr = 44100
	}
	e := &Engine{
		sr: sr, bpm: 120, queued: -1, kitN: 0, songRow: -1,
		fx:    newGlobalFX(sr),
		buf:   make([]float32, maxChunk),
		mixL:  make([]float32, maxChunk),
		mixR:  make([]float32, maxChunk),
		sendD: make([]float32, maxChunk),
		sendR: make([]float32, maxChunk),
	}
	e.SetProject(NewProject())
	return e
}

// SampleRate is the rate the engine renders at.
func (e *Engine) SampleRate() float64 { return e.sr }

// SetProject replaces everything: kits, patterns, songs and waves. The first
// kit is loaded and the sequencer rewound.
func (e *Engine) SetProject(p *Project) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.proj = p
	e.bpm = p.Tempo
	if e.bpm < 30 || e.bpm > 300 {
		e.bpm = 120
	}
	for i := range e.waves {
		e.waves[i] = p.Waves[i]
	}
	e.playing = false
	e.curPat, e.queued, e.songRow = 0, -1, -1
	e.loadKitLocked(0)
	if pk := p.Pattern(0).Kit; pk >= 0 {
		e.loadKitLocked(pk)
	}
	if w := p.Working; w != nil {
		c := *w
		e.kit, e.kitN = &c, p.WorkingN
		for t := 0; t < NumTracks; t++ {
			e.tracks[t].setMachine(c.Track[t].Machine, e.sr, e)
		}
	}
	e.rewindLocked()
	e.dirty = true
}

// Project is the data the engine plays. The host saves it; do not change it
// while the engine runs except through Engine's methods.
func (e *Engine) Project() *Project { return e.proj }

// waveAt makes Engine a waveSource for the UW machines.
func (e *Engine) waveAt(slot int) *UserWave {
	if slot < 0 || slot >= NumWaves {
		return nil
	}
	return e.waves[slot]
}

// ---------------------------------------------------------------- kits

func (e *Engine) loadKitLocked(n int) {
	if n < 0 || n >= NumKits {
		return
	}
	k := e.proj.Kit(n)
	c := *k
	e.kit = &c
	e.kitN = n
	for t := 0; t < NumTracks; t++ {
		e.tracks[t].setMachine(c.Track[t].Machine, e.sr, e)
		e.tracks[t].lockMask = 0
		e.tracks[t].hasCur = false
	}
}

// LoadKit copies kit n into the working kit.
func (e *Engine) LoadKit(n int) {
	e.mu.Lock()
	e.loadKitLocked(n)
	e.dirty = true
	e.mu.Unlock()
}

// SaveKit stores the working kit in slot n (and makes n the current slot).
func (e *Engine) SaveKit(n int, name string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if n < 0 || n >= NumKits {
		return
	}
	c := *e.kit
	if name != "" {
		c.Name = name
	}
	e.proj.Kits[n] = &c
	e.kit.Name = c.Name
	e.kitN = n
	e.dirty = true
}

// KitNumber is the slot the working kit was loaded from or saved to.
func (e *Engine) KitNumber() int { e.mu.Lock(); defer e.mu.Unlock(); return e.kitN }

// KitName is the working kit's name.
func (e *Engine) KitName() string { e.mu.Lock(); defer e.mu.Unlock(); return e.kit.Name }

// ---------------------------------------------------------- parameters

// Param is a parameter of the working kit, 0..127.
func (e *Engine) Param(track, p int) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	if track < 0 || track >= NumTracks || p < 0 || p >= NParams {
		return 0
	}
	return int(e.kit.Track[track].P[p])
}

// SetParam sets a parameter of the working kit.
func (e *Engine) SetParam(track, p, v int) {
	e.mu.Lock()
	e.kit.Set(track, p, v)
	e.dirty = true
	e.mu.Unlock()
}

// Machine is the machine index on a track.
func (e *Engine) Machine(track int) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	if track < 0 || track >= NumTracks {
		return 0
	}
	return e.kit.Track[track].Machine
}

// SetMachine puts a machine on a track (its synth parameters reset to the
// machine's defaults).
func (e *Engine) SetMachine(track, machine int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if track < 0 || track >= NumTracks || machine < 0 || machine >= len(Machines) {
		return
	}
	e.kit.SetMachine(track, machine)
	e.tracks[track].setMachine(machine, e.sr, e)
	e.dirty = true
}

// Global is a global (master effect) parameter of the working kit.
func (e *Engine) Global(i int) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	if i < 0 || i >= NGlobal {
		return 0
	}
	return int(e.kit.Global[i])
}

// SetGlobal sets a global effect parameter.
func (e *Engine) SetGlobal(i, v int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if i < 0 || i >= NGlobal {
		return
	}
	e.kit.Global[i] = uint8(clampi(v, 0, 127))
	e.dirty = true
}

// Mute and Muted.
func (e *Engine) SetMute(track int, m bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if track >= 0 && track < NumTracks {
		e.tracks[track].mute = m
	}
}

func (e *Engine) Muted(track int) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return track >= 0 && track < NumTracks && e.tracks[track].mute
}

// MuteMask is the mute state of the 16 tracks, bit 0 is track 1.
func (e *Engine) MuteMask() uint16 {
	e.mu.Lock()
	defer e.mu.Unlock()
	var m uint16
	for i := range e.tracks {
		if e.tracks[i].mute {
			m |= 1 << uint(i)
		}
	}
	return m
}

// Peak returns the loudest sample the track made since the last call.
func (e *Engine) Peak(track int) float32 {
	e.mu.Lock()
	defer e.mu.Unlock()
	if track < 0 || track >= NumTracks {
		return 0
	}
	p := e.tracks[track].peak
	e.tracks[track].peak = 0
	return p
}

// MasterPeak is the loudest output sample since the last call.
func (e *Engine) MasterPeak() float32 {
	e.mu.Lock()
	defer e.mu.Unlock()
	p := e.meter
	e.meter = 0
	return p
}

// TakeDirty reports (and clears) whether the project changed since the last
// call, so the host knows when to save.
func (e *Engine) TakeDirty() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	d := e.dirty
	e.dirty = false
	return d
}

// ---------------------------------------------------------------- waves

// ErrWaveMemory means the UserWave bank is full.
var ErrWaveMemory = errors.New("UserWave memory is full")

func (e *Engine) waveSamplesLocked(except int) int {
	n := 0
	for i, w := range e.waves {
		if w != nil && i != except {
			n += len(w.Data)
		}
	}
	return n
}

// SetWave puts a decoded sample in a slot. data is mono float.
func (e *Engine) SetWave(slot int, name string, rate int, data []float32) error {
	if slot < 0 || slot >= NumWaves {
		return errors.New("bad wave slot")
	}
	if len(data) < 32 {
		return errors.New("sample is too short")
	}
	if len(data) > MaxWaveSeconds*rate {
		return ErrWaveTooLong
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.waveSamplesLocked(slot)+len(data) > MaxWaveSamples {
		return ErrWaveMemory
	}
	w := &UserWave{Name: CleanWaveName(name), Rate: rate, Data: data}
	e.waves[slot] = w
	e.proj.Waves[slot] = w
	e.dirty = true
	return nil
}

// LoadWAV decodes a WAV stream into a slot.
func (e *Engine) LoadWAV(slot int, name string, r io.Reader) error {
	data, rate, err := DecodeWAV(r)
	if err != nil {
		return err
	}
	return e.SetWave(slot, name, rate, data)
}

// ClearWave empties a slot.
func (e *Engine) ClearWave(slot int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if slot >= 0 && slot < NumWaves {
		e.waves[slot] = nil
		e.proj.Waves[slot] = nil
		e.dirty = true
	}
}

// Wave returns the wave in a slot (or nil). The wave is immutable.
func (e *Engine) Wave(slot int) *UserWave {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.waveAt(slot)
}

// WaveMemory is the used and total UserWave samples.
func (e *Engine) WaveMemory() (used, total int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.waveSamplesLocked(-1), MaxWaveSamples
}

// ---------------------------------------------------------------- hits

// Hit plays a track now, as a pad press does (not from the sequencer). If
// the sequencer is playing in record mode, the hit is written to the pattern.
func (e *Engine) Hit(track int, vel float32) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if track < 0 || track >= NumTracks {
		return
	}
	if vel <= 0 {
		vel = velNormal
	}
	t := &e.tracks[track]
	t.lockMask = 0
	e.fireLocked(track, vel, 0)
	if e.rec && e.playing {
		pat := e.proj.Pattern(e.curPat)
		st := float64(StepTicks(pat.Scale))
		k := int(math.Floor(e.tick/st + 0.5))
		if k >= pat.Len {
			k = 0
		}
		pat.Tracks[track].Trig[k] = true
		e.dirty = true
	}
}

// fireLocked starts a track's voice with its current effective parameters.
func (e *Engine) fireLocked(track int, vel float32, _ int) {
	t := &e.tracks[track]
	var eff [NParams]float32
	t.resolve(&e.kit.Track[track].P, 0, e.sr, &eff)
	copy(t.pbuf[:], eff[:NSynth])
	t.trig(vel, &t.pbuf)
}

// ------------------------------------------------------------- render

// Render fills outL and outR with the next block of audio. The slices must
// have the same length.
func (e *Engine) Render(outL, outR []float32) {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := len(outL)
	if len(outR) < n {
		n = len(outR)
	}
	for off := 0; off < n; {
		chunk := n - off
		if chunk > maxChunk {
			chunk = maxChunk
		}
		var tps float64
		var ft float64
		if e.playing {
			tps = e.ticksPerSample()
			pat := e.proj.Pattern(e.curPat)
			ft = e.fireTime(pat, e.nextStep)
			if need := math.Ceil((ft - e.tick) / tps); need <= 0 {
				chunk = 0
			} else if int(need) < chunk {
				chunk = int(need)
			}
		}
		if chunk > 0 {
			e.renderChunk(outL[off:off+chunk], outR[off:off+chunk])
			if e.playing {
				e.advanceTicks(float64(chunk), tps)
			}
			off += chunk
		}
		if e.playing && e.tick+1e-6 >= ft {
			e.stepEventLocked()
		}
	}
}

func (e *Engine) renderChunk(outL, outR []float32) {
	n := len(outL)
	for i := 0; i < n; i++ {
		e.mixL[i], e.mixR[i], e.sendD[i], e.sendR[i] = 0, 0, 0, 0
	}
	for i := range e.tracks {
		t := &e.tracks[i]
		var eff [NParams]float32
		t.resolve(&e.kit.Track[i].P, n, e.sr, &eff)
		t.process(&eff, n, e.sr, e.buf, e.mixL, e.mixR, e.sendD, e.sendR)
	}
	e.fx.process(&e.kit.Global, e.bpm, n, e.mixL, e.mixR, e.sendD, e.sendR, outL, outR)
	for i := 0; i < n; i++ {
		if a := absf(outL[i]); a > e.meter {
			e.meter = a
		}
		if a := absf(outR[i]); a > e.meter {
			e.meter = a
		}
	}
}

// ticksPerSample is how many sequencer ticks one audio sample lasts.
func (e *Engine) ticksPerSample() float64 {
	bpm := e.bpm
	if e.sync == SyncMIDI && e.extBPM > 0 {
		bpm = e.extBPM
	}
	return bpm * TicksPerBeat / (60 * e.sr)
}

func (e *Engine) advanceTicks(samples, tps float64) {
	d := samples * tps
	e.tick += d
	if e.sync == SyncMIDI {
		// follow the incoming clock: ease towards where the pulses say we are
		e.extTicks += d
		if e.playing && e.extBPM > 0 {
			corr := (e.extTarget - e.extTicks) * 0.05
			if math.Abs(corr) > 24 { // lost the clock: jump
				corr = (e.extTarget - e.extTicks)
			}
			e.tick += corr
			e.extTicks += corr
		}
	}
}

// -------------------------------------------------------------- helpers

func clampi(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// SetSampleRate changes the rate the engine renders at (Live may open the
// card at 44.1 or 48 kHz). Voices and global effects are rebuilt, so call it
// when the audio stream (re)starts, not while sound matters.
func (e *Engine) SetSampleRate(sr float64) {
	if sr < 8000 || sr > 192000 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if sr == e.sr {
		return
	}
	e.sr = sr
	e.fx = newGlobalFX(sr)
	for t := 0; t < NumTracks; t++ {
		e.tracks[t].setMachine(e.kit.Track[t].Machine, sr, e)
		e.tracks[t].hasCur = false
	}
}
