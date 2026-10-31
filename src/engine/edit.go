package engine

// Pattern editing. Every method takes a pattern index; -1 means the pattern
// that is selected now. Edits replace Lock values instead of changing them in
// place, so a copy of a Pattern taken with PatternCopy is a safe snapshot.

func (e *Engine) patLocked(pi int) *Pattern {
	if pi < 0 {
		pi = e.curPat
	}
	return e.proj.Pattern(pi)
}

func okTS(track, step int) bool {
	return track >= 0 && track < NumTracks && step >= 0 && step < MaxSteps
}

// PatternCopy is a snapshot of a pattern for drawing. Do not edit it.
func (e *Engine) PatternCopy(pi int) Pattern {
	e.mu.Lock()
	defer e.mu.Unlock()
	return *e.patLocked(pi)
}

// SetTrig turns a trig on or off. Turning it off removes its locks.
func (e *Engine) SetTrig(pi, track, step int, on bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !okTS(track, step) {
		return
	}
	tp := &e.patLocked(pi).Tracks[track]
	tp.Trig[step] = on
	if !on {
		tp.Lock[step] = nil
	}
	e.dirty = true
}

// ToggleTrig flips a trig and returns the new state.
func (e *Engine) ToggleTrig(pi, track, step int) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !okTS(track, step) {
		return false
	}
	tp := &e.patLocked(pi).Tracks[track]
	tp.Trig[step] = !tp.Trig[step]
	if !tp.Trig[step] {
		tp.Lock[step] = nil
	}
	e.dirty = true
	return tp.Trig[step]
}

// SetLock locks parameter p of a trig to v. The step must have a trig.
func (e *Engine) SetLock(pi, track, step, p, v int) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !okTS(track, step) || p < 0 || p >= NParams {
		return false
	}
	tp := &e.patLocked(pi).Tracks[track]
	if !tp.Trig[step] {
		return false
	}
	var l Lock
	if old := tp.Lock[step]; old != nil {
		l = *old
	}
	l.Mask |= 1 << uint(p)
	l.Val[p] = uint8(clampi(v, 0, 127))
	tp.Lock[step] = &l
	e.dirty = true
	return true
}

// ClearLock removes the lock on one parameter; p < 0 removes all locks of
// the step.
func (e *Engine) ClearLock(pi, track, step, p int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !okTS(track, step) {
		return
	}
	tp := &e.patLocked(pi).Tracks[track]
	old := tp.Lock[step]
	if old == nil {
		return
	}
	if p < 0 || p >= NParams {
		tp.Lock[step] = nil
	} else {
		l := *old
		l.Mask &^= 1 << uint(p)
		if l.Mask == 0 {
			tp.Lock[step] = nil
		} else {
			tp.Lock[step] = &l
		}
	}
	e.dirty = true
}

// LockOf returns a copy of the lock of a step, and whether it has one.
func (e *Engine) LockOf(pi, track, step int) (Lock, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !okTS(track, step) {
		return Lock{}, false
	}
	if l := e.patLocked(pi).Tracks[track].Lock[step]; l != nil {
		return *l, true
	}
	return Lock{}, false
}

// Layer is one of the pattern-wide trig layers.
type Layer int

const (
	LayerAccent Layer = iota
	LayerSwing
	LayerSlide
)

// ToggleLayer flips a step of the accent, swing or slide layer.
func (e *Engine) ToggleLayer(pi int, l Layer, step int) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if step < 0 || step >= MaxSteps {
		return false
	}
	p := e.patLocked(pi)
	var b *bool
	switch l {
	case LayerAccent:
		b = &p.Accent[step]
	case LayerSwing:
		b = &p.Swing[step]
	default:
		b = &p.Slide[step]
	}
	*b = !*b
	e.dirty = true
	return *b
}

// SetPatternLen sets the pattern length, 2..64 steps.
func (e *Engine) SetPatternLen(pi, n int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.patLocked(pi).Len = clampi(n, 2, MaxSteps)
	e.dirty = true
}

// SetPatternScale sets the pattern scale (index into ScaleNames).
func (e *Engine) SetPatternScale(pi, s int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.patLocked(pi).Scale = clampi(s, 0, len(ScaleNames)-1)
	e.dirty = true
}

// SetSwingAmount sets the swing amount in percent, 50..80.
func (e *Engine) SetSwingAmount(pi, pct int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.patLocked(pi).SwingAmt = clampi(pct, 50, 80)
	e.dirty = true
}

// SetAccentAmount sets how strong accents are, 0..127.
func (e *Engine) SetAccentAmount(pi, v int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.patLocked(pi).AccentAmt = uint8(clampi(v, 0, 127))
	e.dirty = true
}

// SetPatternKit sets the kit a pattern loads when it starts (-1: none).
func (e *Engine) SetPatternKit(pi, kit int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.patLocked(pi).Kit = clampi(kit, -1, NumKits-1)
	e.dirty = true
}

// SetPatternName names a pattern.
func (e *Engine) SetPatternName(pi int, name string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(name) > 12 {
		name = name[:12]
	}
	e.patLocked(pi).Name = name
	e.dirty = true
}

// ClearTrack removes every trig and lock of one track.
func (e *Engine) ClearTrack(pi, track int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if track < 0 || track >= NumTracks {
		return
	}
	e.patLocked(pi).Tracks[track] = TrackPat{}
	e.dirty = true
}

// ClearPattern empties a pattern (length, scale and the rest reset too).
func (e *Engine) ClearPattern(pi int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if pi < 0 {
		pi = e.curPat
	}
	e.proj.Patterns[clampi(pi, 0, NumPatterns-1)] = NewPattern()
	e.dirty = true
}

// CopyPattern copies pattern src over pattern dst.
func (e *Engine) CopyPattern(src, dst int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if src < 0 {
		src = e.curPat
	}
	if dst < 0 {
		dst = e.curPat
	}
	if src == dst || src >= NumPatterns || dst >= NumPatterns {
		return
	}
	e.proj.Patterns[dst] = e.proj.Pattern(src).Clone()
	e.dirty = true
}

// CopyTrack copies one track's row to another track or pattern.
func (e *Engine) CopyTrack(srcPat, srcTrack, dstPat, dstTrack int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if srcTrack < 0 || srcTrack >= NumTracks || dstTrack < 0 || dstTrack >= NumTracks {
		return
	}
	row := e.patLocked(srcPat).Tracks[srcTrack] // locks are never edited in place
	e.patLocked(dstPat).Tracks[dstTrack] = row
	e.dirty = true
}

// ShiftTrack rotates a track's trigs by n steps within the pattern length.
func (e *Engine) ShiftTrack(pi, track, n int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if track < 0 || track >= NumTracks {
		return
	}
	p := e.patLocked(pi)
	old := p.Tracks[track]
	var nw TrackPat
	for s := 0; s < p.Len; s++ {
		d := ((s+n)%p.Len + p.Len) % p.Len
		nw.Trig[d], nw.Lock[d] = old.Trig[s], old.Lock[s]
	}
	p.Tracks[track] = nw
	e.dirty = true
}

// Song editing -------------------------------------------------------

// SongCopy is a snapshot of a song for drawing.
func (e *Engine) SongCopy(n int) Song {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := *e.proj.Song(n)
	s.Rows = append([]SongRow(nil), s.Rows...)
	return s
}

// SongSetRow sets a row, appending when row == len(rows).
func (e *Engine) SongSetRow(n, row, pattern, repeats int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.proj.Song(n)
	r := SongRow{Pattern: clampi(pattern, 0, NumPatterns-1), Repeats: clampi(repeats, 1, 99)}
	switch {
	case row >= 0 && row < len(s.Rows):
		s.Rows[row] = r
	case row == len(s.Rows) && row < MaxSongRows:
		s.Rows = append(s.Rows, r)
	default:
		return
	}
	e.dirty = true
}

// SongDeleteRow removes a row.
func (e *Engine) SongDeleteRow(n, row int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.proj.Song(n)
	if row < 0 || row >= len(s.Rows) {
		return
	}
	s.Rows = append(s.Rows[:row], s.Rows[row+1:]...)
	if s.LoopTo >= len(s.Rows) {
		s.LoopTo = -1
	}
	e.dirty = true
}

// SongSetLoop sets the row the song jumps back to at its end (-1 stops).
func (e *Engine) SongSetLoop(n, row int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.proj.Song(n)
	if row < -1 || row >= len(s.Rows) {
		row = -1
	}
	s.LoopTo = row
	e.dirty = true
}
