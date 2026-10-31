package engine

import (
	"encoding/json"
	"errors"
)

// TrackNames are the Machinedrum's 16 track names, in order.
var TrackNames = [NumTracks]string{
	"BD", "SD", "HT", "MT", "LT", "CP", "RS", "CB",
	"CH", "OH", "RC", "CC", "M1", "M2", "M3", "M4",
}

// BankNames are the pattern banks A..H (16 patterns each).
const BankLetters = "ABCDEFGH"

// PatternName is the display name of pattern n: "A01".."H16".
func PatternName(n int) string {
	if n < 0 || n >= NumPatterns {
		return "---"
	}
	return string([]byte{BankLetters[n/16], byte('0' + (n%16+1)/10), byte('0' + (n%16+1)%10)})
}

// Project is everything a user saves: the kits, patterns and songs, the
// tempo and the UserWave bank.
type Project struct {
	Name     string
	Tempo    float64
	Kits     [NumKits]*Kit
	Patterns [NumPatterns]*Pattern
	Songs    [NumSongs]*Song
	// Waves are saved as WAV files by the host, not in the project file.
	Waves [NumWaves]*UserWave
	// Working is the kit as it was being edited (not yet saved to a slot) and
	// WorkingN the slot it came from. Nil in a fresh project.
	Working  *Kit
	WorkingN int
}

// defaultMachines is the factory kit: a classic drum kit on the first twelve
// tracks and four open slots (a sine, an FM kick, a sampler and shakers).
var defaultMachines = []string{
	"TRX-BD", "TRX-SD", "TRX-XT", "TRX-XT", "TRX-XT", "TRX-CP", "TRX-RS", "TRX-CB",
	"TRX-CH", "TRX-OH", "TRX-CY", "TRX-CY", "GND-SIN", "EFM-BD", "UW-SMP", "TRX-MA",
}

// DefaultKit makes the factory kit.
func DefaultKit(name string) *Kit {
	k := NewKit(name, defaultMachines)
	// tune the toms, and spread the cymbals
	k.Set(2, 0, 74)
	k.Set(3, 0, 58)
	k.Set(4, 0, 42)
	k.Set(11, 0, 50)
	k.Set(11, 1, 100)
	k.Set(10, 1, 70)
	// pan the toms and hats a little, send the clap and snare to the reverb
	for t, pan := range map[int]int{2: 36, 3: 56, 4: 80, 8: 80, 9: 84, 10: 46, 11: 84, 15: 90} {
		k.Set(t, PAN, pan)
	}
	k.Set(1, REV, 24)
	k.Set(5, REV, 40)
	k.Set(5, DEL, 12)
	k.Set(9, DEL, 16)
	k.Set(7, DEL, 30)
	return k
}

// NewProject is a fresh project: the factory kit in every kit slot's first
// place, a starter beat in pattern A01 and empty everything else.
func NewProject() *Project {
	p := &Project{Name: "NEW", Tempo: 120}
	p.Kits[0] = DefaultKit("FACTORY")
	pat := NewPattern()
	pat.Name = "STARTER"
	for _, s := range []int{0, 4, 8, 12} {
		pat.Tracks[0].Trig[s] = true // BD four on the floor
	}
	pat.Tracks[1].Trig[4], pat.Tracks[1].Trig[12] = true, true
	for s := 0; s < 16; s += 2 {
		pat.Tracks[8].Trig[s] = true
	}
	pat.Tracks[9].Trig[14] = true
	pat.Tracks[5].Trig[12] = true
	pat.Accent[0], pat.Accent[8] = true, true
	pat.Patch()
	p.Patterns[0] = pat
	return p
}

// Patch is a no-op hook kept so factory patterns can be tweaked in one place.
func (p *Pattern) Patch() {}

// Kit returns kit n, making a default one if the slot is empty.
func (p *Project) Kit(n int) *Kit {
	if n < 0 || n >= NumKits {
		n = 0
	}
	if p.Kits[n] == nil {
		p.Kits[n] = DefaultKit("KIT " + twoDigits(n+1))
	}
	return p.Kits[n]
}

// Pattern returns pattern n, making an empty one if the slot is empty.
func (p *Project) Pattern(n int) *Pattern {
	if n < 0 || n >= NumPatterns {
		n = 0
	}
	if p.Patterns[n] == nil {
		p.Patterns[n] = NewPattern()
	}
	return p.Patterns[n]
}

// Song returns song n, making an empty one if the slot is empty.
func (p *Project) Song(n int) *Song {
	if n < 0 || n >= NumSongs {
		n = 0
	}
	if p.Songs[n] == nil {
		p.Songs[n] = NewSong()
	}
	return p.Songs[n]
}

func twoDigits(n int) string { return string([]byte{byte('0' + n/10%10), byte('0' + n%10)}) }

// ------------------------------------------------------------ JSON file

type lockJSON struct {
	Step int     `json:"s"`
	Mask uint32  `json:"m"`
	Val  []uint8 `json:"v"` // the locked values, lowest parameter first
}

type trackJSON struct {
	Trig  uint64     `json:"t,omitempty"`
	Locks []lockJSON `json:"l,omitempty"`
}

type patternJSON struct {
	Name      string      `json:"name,omitempty"`
	Len       int         `json:"len"`
	Scale     int         `json:"scale"`
	Swing     int         `json:"swing"`
	AccentAmt uint8       `json:"accamt"`
	Kit       int         `json:"kit"`
	Accent    uint64      `json:"accent,omitempty"`
	SwingRow  uint64      `json:"swingrow,omitempty"`
	Slide     uint64      `json:"slide,omitempty"`
	Tracks    []trackJSON `json:"tracks"`
}

type projectJSON struct {
	Version  int                 `json:"version"`
	Name     string              `json:"name"`
	Tempo    float64             `json:"tempo"`
	Kits     map[int]*Kit        `json:"kits"`
	Patterns map[int]patternJSON `json:"patterns"`
	Songs    map[int]*Song       `json:"songs"`
	Working  *Kit                `json:"working,omitempty"`
	WorkingN int                 `json:"workingkit,omitempty"`
}

func bits(a [MaxSteps]bool) uint64 {
	var m uint64
	for i, b := range a {
		if b {
			m |= 1 << uint(i)
		}
	}
	return m
}

func unbits(m uint64, a *[MaxSteps]bool) {
	for i := range a {
		a[i] = m&(1<<uint(i)) != 0
	}
}

func (p *Pattern) toJSON() patternJSON {
	j := patternJSON{Name: p.Name, Len: p.Len, Scale: p.Scale, Swing: p.SwingAmt,
		AccentAmt: p.AccentAmt, Kit: p.Kit, Accent: bits(p.Accent),
		SwingRow: bits(p.Swing), Slide: bits(p.Slide), Tracks: make([]trackJSON, NumTracks)}
	for t := range p.Tracks {
		tj := trackJSON{Trig: bits(p.Tracks[t].Trig)}
		for s, l := range p.Tracks[t].Lock {
			if l == nil || l.Mask == 0 {
				continue
			}
			lj := lockJSON{Step: s, Mask: l.Mask}
			for i := 0; i < NParams; i++ {
				if l.Has(i) {
					lj.Val = append(lj.Val, l.Val[i])
				}
			}
			tj.Locks = append(tj.Locks, lj)
		}
		j.Tracks[t] = tj
	}
	return j
}

func (j patternJSON) pattern() *Pattern {
	p := NewPattern()
	p.Name, p.Len, p.Scale, p.SwingAmt, p.AccentAmt, p.Kit = j.Name, j.Len, j.Scale, j.Swing, j.AccentAmt, j.Kit
	unbits(j.Accent, &p.Accent)
	unbits(j.SwingRow, &p.Swing)
	unbits(j.Slide, &p.Slide)
	for t := 0; t < NumTracks && t < len(j.Tracks); t++ {
		unbits(j.Tracks[t].Trig, &p.Tracks[t].Trig)
		for _, lj := range j.Tracks[t].Locks {
			if lj.Step < 0 || lj.Step >= MaxSteps {
				continue
			}
			l := &Lock{}
			k := 0
			for i := 0; i < NParams; i++ {
				if lj.Mask&(1<<uint(i)) != 0 && k < len(lj.Val) {
					l.Mask |= 1 << uint(i)
					l.Val[i] = lj.Val[k]
					k++
				}
			}
			p.Tracks[t].Lock[lj.Step] = l
		}
	}
	p.normalise()
	return p
}

// Marshal writes the project (without the UserWave audio) as JSON.
func (p *Project) Marshal() ([]byte, error) {
	j := projectJSON{Version: 1, Name: p.Name, Tempo: p.Tempo,
		Kits: map[int]*Kit{}, Patterns: map[int]patternJSON{}, Songs: map[int]*Song{},
		Working: p.Working, WorkingN: p.WorkingN}
	for i, k := range p.Kits {
		if k != nil {
			j.Kits[i] = k
		}
	}
	for i, pt := range p.Patterns {
		if pt != nil && !pt.Empty() {
			j.Patterns[i] = pt.toJSON()
		}
	}
	for i, s := range p.Songs {
		if s != nil && len(s.Rows) > 0 {
			j.Songs[i] = s
		}
	}
	return json.MarshalIndent(j, "", " ")
}

// UnmarshalProject reads a project file. Out-of-range values are clamped, so
// a hand-edited file cannot crash the engine.
func UnmarshalProject(b []byte) (*Project, error) {
	var j projectJSON
	if err := json.Unmarshal(b, &j); err != nil {
		return nil, err
	}
	if j.Version != 1 {
		return nil, errors.New("unknown project file version")
	}
	p := &Project{Name: j.Name, Tempo: j.Tempo}
	if p.Tempo < 30 || p.Tempo > 300 {
		p.Tempo = 120
	}
	for i, k := range j.Kits {
		if i < 0 || i >= NumKits || k == nil {
			continue
		}
		for t := range k.Track {
			if k.Track[t].Machine < 0 || k.Track[t].Machine >= len(Machines) {
				k.Track[t].Machine = 0
			}
			for q := range k.Track[t].P {
				if k.Track[t].P[q] > 127 {
					k.Track[t].P[q] = 127
				}
			}
		}
		for g := range k.Global {
			if k.Global[g] > 127 {
				k.Global[g] = 127
			}
		}
		p.Kits[i] = k
	}
	for i, pj := range j.Patterns {
		if i >= 0 && i < NumPatterns {
			p.Patterns[i] = pj.pattern()
		}
	}
	for i, s := range j.Songs {
		if i < 0 || i >= NumSongs || s == nil {
			continue
		}
		if len(s.Rows) > MaxSongRows {
			s.Rows = s.Rows[:MaxSongRows]
		}
		for r := range s.Rows {
			s.Rows[r].Pattern = clampi(s.Rows[r].Pattern, 0, NumPatterns-1)
			s.Rows[r].Repeats = clampi(s.Rows[r].Repeats, 1, 99)
		}
		if s.LoopTo >= len(s.Rows) {
			s.LoopTo = -1
		}
		p.Songs[i] = s
	}
	if p.Kits[0] == nil {
		p.Kits[0] = DefaultKit("FACTORY")
	}
	if k := j.Working; k != nil {
		for t := range k.Track {
			if k.Track[t].Machine < 0 || k.Track[t].Machine >= len(Machines) {
				k.Track[t].Machine = 0
			}
			for q := range k.Track[t].P {
				if k.Track[t].P[q] > 127 {
					k.Track[t].P[q] = 127
				}
			}
		}
		for g := range k.Global {
			if k.Global[g] > 127 {
				k.Global[g] = 127
			}
		}
		p.Working, p.WorkingN = k, clampi(j.WorkingN, 0, NumKits-1)
	}
	return p, nil
}

// ResetKit loads the factory kit into the working kit, as "init kit" does.
// (The kit slot it came from does not change until it is saved.)
func (e *Engine) ResetKit() {
	e.mu.Lock()
	defer e.mu.Unlock()
	c := *DefaultKit(e.kit.Name)
	e.kit = &c
	for t := 0; t < NumTracks; t++ {
		e.tracks[t].setMachine(c.Track[t].Machine, e.sr, e)
		e.tracks[t].lockMask = 0
		e.tracks[t].hasCur = false
	}
	e.dirty = true
}

// TempoNow is only used by tests of the clock; it reads the internal tempo.
func (e *Engine) InternalTempo() float64 { e.mu.Lock(); defer e.mu.Unlock(); return e.bpm }

// MarshalProject is Marshal made safe while the engine runs: it takes the
// lock and includes the tempo and the working kit. It also returns the waves
// (immutable) so the host can store them.
func (e *Engine) MarshalProject() ([]byte, [NumWaves]*UserWave, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.proj.Tempo = e.bpm
	k := *e.kit
	e.proj.Working, e.proj.WorkingN = &k, e.kitN
	b, err := e.proj.Marshal()
	return b, e.waves, err
}
