package engine

// TicksPerBeat is the engine's internal clock resolution (pulses per quarter
// note). A 16th-note step at 1x scale is TicksPerBeat/4 = 24 ticks, which is
// fine enough for swing, parameter slides and external clock (24 ppqn x 4).
const TicksPerBeat = 96

// ScaleNames and scaleTicks are the pattern scale options: how long one step
// lasts, relative to a 16th note.
var (
	ScaleNames = [...]string{"1/8x", "1/4x", "1/2x", "3/4x", "1x", "3/2x", "2x"}
	scaleTicks = [...]int{192, 96, 48, 32, 24, 16, 12}
	// DefaultScale is the index of "1x".
	DefaultScale = 4
)

// StepTicks is the length of one step of a pattern at scale index s.
func StepTicks(s int) int {
	if s < 0 || s >= len(scaleTicks) {
		s = DefaultScale
	}
	return scaleTicks[s]
}

// Lock is a set of parameter locks on one step: the parameters in Mask take
// the value in Val for that trig, instead of the kit's value.
type Lock struct {
	Mask uint32
	Val  [NParams]uint8
}

// Has reports whether parameter p is locked.
func (l *Lock) Has(p int) bool { return l != nil && l.Mask&(1<<uint(p)) != 0 }

// TrackPat is one track's row in a pattern.
type TrackPat struct {
	Trig [MaxSteps]bool
	Lock [MaxSteps]*Lock
}

// Pattern is 16 tracks of up to 64 steps, plus the three shared layers the
// Machinedrum has: accent, swing and slide.
type Pattern struct {
	Name      string
	Len       int   // 2..64 steps
	Scale     int   // index into ScaleNames
	SwingAmt  int   // 50..80 (percent)
	AccentAmt uint8 // 0..127
	Kit       int   // kit this pattern loads when it starts; -1 keeps the current kit
	Accent    [MaxSteps]bool
	Swing     [MaxSteps]bool
	Slide     [MaxSteps]bool
	Tracks    [NumTracks]TrackPat
}

// NewPattern is an empty pattern: 16 steps, 1x, no swing.
func NewPattern() *Pattern {
	return &Pattern{Len: 16, Scale: DefaultScale, SwingAmt: 50, AccentAmt: 80, Kit: -1}
}

// Clone is a deep copy.
func (p *Pattern) Clone() *Pattern {
	c := *p
	for t := range p.Tracks {
		for s, l := range p.Tracks[t].Lock {
			if l != nil {
				lc := *l
				c.Tracks[t].Lock[s] = &lc
			}
		}
	}
	return &c
}

// Empty reports whether the pattern holds no trigs and differs from a new
// pattern in nothing but its trigs, so the project file can leave it out.
func (p *Pattern) Empty() bool {
	n := NewPattern()
	if p.Len != n.Len || p.Scale != n.Scale || p.SwingAmt != n.SwingAmt ||
		p.AccentAmt != n.AccentAmt || p.Kit != n.Kit || p.Name != "" {
		return false
	}
	if p.Accent != n.Accent || p.Swing != n.Swing || p.Slide != n.Slide {
		return false
	}
	for t := range p.Tracks {
		if p.Tracks[t].Trig != n.Tracks[t].Trig {
			return false
		}
		for _, l := range p.Tracks[t].Lock {
			if l != nil {
				return false
			}
		}
	}
	return true
}

// normalise puts every field in range (a project file may be hand edited).
func (p *Pattern) normalise() {
	if p.Len < 2 {
		p.Len = 2
	}
	if p.Len > MaxSteps {
		p.Len = MaxSteps
	}
	if p.Scale < 0 || p.Scale >= len(ScaleNames) {
		p.Scale = DefaultScale
	}
	if p.SwingAmt < 50 {
		p.SwingAmt = 50
	}
	if p.SwingAmt > 80 {
		p.SwingAmt = 80
	}
	if p.Kit < -1 || p.Kit >= NumKits {
		p.Kit = -1
	}
	for t := range p.Tracks {
		for s, l := range p.Tracks[t].Lock {
			if l != nil && l.Mask == 0 {
				p.Tracks[t].Lock[s] = nil
			}
		}
	}
}

// ---------------------------------------------------------------- kits

// TrackSetup is one track of a kit: which machine, and all 24 parameters.
type TrackSetup struct {
	Machine int
	P       [NParams]uint8
}

// Kit is a sound set: 16 tracks of machine and parameters, plus the global
// effects.
type Kit struct {
	Name   string
	Track  [NumTracks]TrackSetup
	Global [NGlobal]uint8
}

// SetMachine puts a machine on a track and resets its synth parameters to
// that machine's defaults. The effect and routing parameters stay.
func (k *Kit) SetMachine(track, machine int) {
	if track < 0 || track >= NumTracks || machine < 0 || machine >= len(Machines) {
		return
	}
	k.Track[track].Machine = machine
	copy(k.Track[track].P[:NSynth], Machines[machine].Defaults[:])
}

// NewKit is a kit with every track on the given machine names (in order) and
// neutral effects. Unknown or missing names fall back to TRX-BD.
func NewKit(name string, machines []string) *Kit {
	k := &Kit{Name: name, Global: DefaultGlobal}
	for t := 0; t < NumTracks; t++ {
		m := 0
		if t < len(machines) {
			if i := MachineIndex(machines[t]); i >= 0 {
				m = i
			}
		}
		k.Track[t].Machine = m
		copy(k.Track[t].P[:NSynth], Machines[m].Defaults[:])
		copy(k.Track[t].P[NSynth:], DefaultFXRoute[:])
	}
	return k
}

// Set sets a parameter, clamped to 0..127.
func (k *Kit) Set(track, param, v int) {
	if track < 0 || track >= NumTracks || param < 0 || param >= NParams {
		return
	}
	if v < 0 {
		v = 0
	}
	if v > 127 {
		v = 127
	}
	k.Track[track].P[param] = uint8(v)
}

// ---------------------------------------------------------------- songs

// SongRow plays a pattern for a number of repeats.
type SongRow struct {
	Pattern int
	Repeats int // 1..99
}

// Song is a list of rows, with an optional loop back to a row at the end.
type Song struct {
	Name   string
	Rows   []SongRow
	LoopTo int // row to jump to after the last row; -1 stops
}

// NewSong is an empty song that stops at its end.
func NewSong() *Song { return &Song{LoopTo: -1} }
