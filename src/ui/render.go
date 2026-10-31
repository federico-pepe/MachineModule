package ui

import (
	"fmt"
	"image"
	"strings"
	"time"

	"machinemodule/engine"
)

const (
	cellW      = W / 8
	tabsY      = 0
	headY      = 18
	cellsY     = 38
	ribbonY    = 134
	softY      = 144
	lfoShapes  = "SIN TRI SAW S&H"
	delaySyncs = "FREE 1/16 1/8 3/16 1/4 3/8 1/2 3/4"
)

var (
	lfoShapeNames = strings.Fields(lfoShapes)
	delaySyncName = strings.Fields(delaySyncs)
)

// Render draws the whole screen.
func (c *Controller) Render() *image.NRGBA {
	c.mu.Lock()
	defer c.mu.Unlock()
	cv := newCanvas()
	c.drawTabs(cv)
	c.drawHeader(cv)
	switch {
	case c.ov == ovMachine:
		c.drawMachineList(cv)
	default:
		switch c.page {
		case PgSynth, PgFX, PgRoute:
			c.drawParamCells(cv)
		case PgMaster:
			c.drawMaster(cv)
		case PgSeq:
			c.drawSeq(cv)
		case PgSong:
			c.drawSong(cv)
		case PgKit:
			c.drawKit(cv)
		case PgSystem:
			c.drawSystem(cv)
		}
	}
	c.drawRibbon(cv)
	c.drawSoftKeys(cv)
	return cv.NRGBA
}

func (c *Controller) drawTabs(cv canvas) {
	for i := 0; i < numPages; i++ {
		x := i * cellW
		if i == c.page {
			cv.fill(x+2, tabsY, cellW-4, 16, colFG)
			cv.textC(x+cellW/2, tabsY+1, 2, pageNames[i], colBG)
		} else {
			cv.textC(x+cellW/2, tabsY+1, 2, pageNames[i], colFG)
		}
	}
}

func (c *Controller) drawHeader(cv canvas) {
	pos := c.eng.Position()
	m := c.eng.Machine(c.track)
	pat := c.eng.PatternCopy(-1)
	sync := "INT"
	if c.eng.Sync() == engine.SyncMIDI {
		sync = "MIDI"
	}
	cv.fill(0, headY-2, W, 1, colDim)
	cv.label(6, headY, 2, fmt.Sprintf("T%02d %s", c.track+1, engine.TrackNames[c.track]))
	cv.text(112, headY, 2, engine.Machines[m].Name, colFG)
	cv.text(230, headY, 2, fmt.Sprintf("KIT%02d", c.eng.KitNumber()+1), colFG)
	cv.text(318, headY, 2, fmt.Sprintf("%s %02d/%02d", engine.PatternName(pos.Pattern), pos.Step+1, pat.Len), colFG)
	if pos.Next >= 0 {
		cv.text(318+12*9, headY, 2, ">"+engine.PatternName(pos.Next), colWarn)
	}
	cv.text(490, headY, 2, fmt.Sprintf("%5.1f", c.eng.Tempo()), colFG)
	cv.text(560, headY, 2, sync, colFG)
	flags := ""
	if pos.Playing {
		flags += "PLAY "
	}
	if c.eng.Recording() {
		flags += "REC "
	}
	if on, _ := c.eng.SongMode(); on {
		flags += "SONG "
	}
	cv.text(620, headY, 2, flags, colFG)
	if ok, msg := c.host.AudioStatus(); !ok {
		cv.text(620, headY, 2, "NO AUDIO", colWarn)
		_ = msg
	}
	if c.now().Before(c.toastUntil) {
		w := textW(c.toast, 2)
		cv.fill(W-w-14, headY-3, w+12, 20, colFG)
		cv.text(W-w-8, headY, 2, c.toast, colWarn)
	}
	if c.layer != layerNone || c.muteMode {
		name := map[layerKind]string{layerAccent: "ACCENT", layerSwing: "SWING", layerSlide: "SLIDE"}[c.layer]
		if c.muteMode {
			name = "MUTE"
		}
		cv.text(800, headY, 2, name, colWarn)
	}
}

// cell draws one encoder column.
func (c *Controller) cell(cv canvas, i int, label, big, small string, frac float64, centre bool, locked bool) {
	x := i * cellW
	if i > 0 {
		cv.fill(x, cellsY+2, 1, 90, colDim)
	}
	touched := c.lastTouch == i && c.now().Sub(c.lastTouchAt) < 600*time.Millisecond
	if touched {
		cv.label(x+cellW/2-textW(label, 2)/2, cellsY+4, 2, label)
	} else {
		cv.textC(x+cellW/2, cellsY+4, 2, label, colFG)
	}
	cv.textC(x+cellW/2, cellsY+24, 3, big, colFG)
	if frac >= 0 {
		if centre {
			cv.centreBar(x+10, cellsY+52, cellW-20, 12, frac, colFG)
		} else {
			cv.bar(x+10, cellsY+52, cellW-20, 12, frac, colFG)
		}
	}
	if small != "" {
		cv.textC(x+cellW/2, cellsY+72, 2, small, colFG)
	}
	if locked {
		cv.label(x+cellW-40, cellsY+84, 1, "LOCK")
	}
}

func (c *Controller) drawParamCells(cv canvas) {
	m := c.eng.Machine(c.track)
	// a held step shows its own lock values
	var lock engine.Lock
	haveLock := false
	for s := range c.held {
		if l, ok := c.eng.LockOf(-1, c.track, s); ok {
			lock, haveLock = l, true
		}
	}
	for i := 0; i < 8; i++ {
		p := paramIndex(c.page, i)
		v := c.eng.Param(c.track, p)
		locked := false
		if haveLock && lock.Has(p) {
			v, locked = int(lock.Val[p]), true
		}
		label := engine.ParamName(m, p)
		small := c.paramNote(m, p, v)
		centre := p == engine.PAN || p == engine.EQG
		c.cell(cv, i, label, num3(v), small, float64(v)/127, centre, locked)
	}
}

// paramNote is the small line under a parameter: what the number means.
func (c *Controller) paramNote(m, p, v int) string {
	switch p {
	case engine.PAN:
		switch {
		case v >= 62 && v <= 66:
			return "CENTER"
		case v < 64:
			return fmt.Sprintf("L%d", (64-v)*100/64)
		default:
			return fmt.Sprintf("R%d", (v-64)*100/63)
		}
	case engine.EQG:
		return fmt.Sprintf("%+.1fDB", float64(v-64)*36/127)
	case engine.LFOP:
		d := int(float64(v) / 127 * float64(engine.NParams) * 0.9999)
		if d >= engine.LFOP {
			return "OFF"
		}
		return engine.ParamName(m, d)
	case engine.LFOM:
		return lfoShapeNames[clampi(v*4/128, 0, 3)]
	case 0:
		if engine.Machines[m].Group == "UW" {
			slot := clampi(v/2, 0, engine.NumWaves-1)
			if w := c.eng.Wave(slot); w != nil {
				return fmt.Sprintf("%02d %s", slot+1, strings.ToUpper(w.Name))
			}
			return fmt.Sprintf("%02d EMPTY", slot+1)
		}
	}
	return ""
}

func (c *Controller) drawMaster(cv canvas) {
	for i := 0; i < 8; i++ {
		g := c.sub*8 + i
		v := c.eng.Global(g)
		small := ""
		switch g {
		case engine.GDSYN:
			small = delaySyncName[clampi(v/16, 0, 7)]
		case engine.GDTIM:
			small = fmt.Sprintf("%dMS", int(float64(5)*pow(300, float64(v)/127)))
		case engine.GELG, engine.GEHG:
			small = fmt.Sprintf("%+.1fDB", (float64(v)/127-0.5)*24)
		}
		c.cell(cv, i, engine.GlobalNames[g], num3(v), small, float64(v)/127, g == engine.GELG || g == engine.GEHG, false)
	}
}

func pow(b, e float64) float64 {
	r := 1.0
	// small, allocation-free exp by repeated square root (enough for a label)
	x := b
	for k := 0; k < 12; k++ {
		if int(e*4096)&(1<<(11-uint(k))) != 0 {
			r *= x
		}
		x = sqrt(x)
	}
	return r
}

func sqrt(x float64) float64 {
	z := x
	for i := 0; i < 20; i++ {
		z = (z + x/z) / 2
	}
	return z
}

func (c *Controller) drawSeq(cv canvas) {
	p := c.eng.PatternCopy(-1)
	kit := "--"
	if p.Kit >= 0 {
		kit = fmt.Sprintf("%02d", p.Kit+1)
	}
	sync, mode := "INT", "PAT"
	if c.eng.Sync() == engine.SyncMIDI {
		sync = "MIDI"
	}
	if on, _ := c.eng.SongMode(); on {
		mode = "SONG"
	}
	c.cell(cv, 0, "LENGTH", num3(p.Len), "STEPS", float64(p.Len)/64, false, false)
	c.cell(cv, 1, "SCALE", engine.ScaleNames[p.Scale], "", float64(p.Scale)/6, false, false)
	c.cell(cv, 2, "SWING", fmt.Sprintf("%d%%", p.SwingAmt), "", float64(p.SwingAmt-50)/30, false, false)
	c.cell(cv, 3, "ACCENT", num3(int(p.AccentAmt)), "", float64(p.AccentAmt)/127, false, false)
	c.cell(cv, 4, "PAT KIT", kit, "AT START", -1, false, false)
	c.cell(cv, 5, "TEMPO", fmt.Sprintf("%.1f", c.eng.Tempo()), "BPM", (c.eng.Tempo()-30)/270, false, false)
	c.cell(cv, 6, "SYNC", sync, "CLOCK", -1, false, false)
	c.cell(cv, 7, "PLAY", mode, "", -1, false, false)
}

func (c *Controller) drawSong(cv canvas) {
	s := c.eng.SongCopy(c.songNum)
	on, _ := c.eng.SongMode()
	row := "NEW"
	pat, rep := "---", "-"
	if c.songRow < len(s.Rows) {
		row = fmt.Sprintf("%02d", c.songRow+1)
		pat = engine.PatternName(s.Rows[c.songRow].Pattern)
		rep = fmt.Sprintf("x%d", s.Rows[c.songRow].Repeats)
	}
	loop := "STOP"
	if s.LoopTo >= 0 {
		loop = fmt.Sprintf("ROW %02d", s.LoopTo+1)
	}
	c.cell(cv, 0, "ROW", row, "", -1, false, false)
	c.cell(cv, 1, "PATTERN", pat, "", -1, false, false)
	c.cell(cv, 2, "REPEAT", rep, "", -1, false, false)
	c.cell(cv, 3, "AT END", loop, "", -1, false, false)
	c.cell(cv, 4, "SONG", fmt.Sprintf("%02d", c.songNum+1), onOffWord(on), -1, false, false)
	pos := c.eng.Position()
	for k := -1; k <= 1; k++ {
		r := c.songRow + k
		y := cellsY + 70 + (k+1)*0
		_ = y
		_ = r
	}
	// list of rows, 3 lines
	for k := 0; k < 3; k++ {
		r := c.songRow - 1 + k
		y := cellsY + 72 + k*16
		if r < 0 || r > len(s.Rows) {
			continue
		}
		txt := "-- END --"
		if r < len(s.Rows) {
			txt = fmt.Sprintf("%02d  %s  x%d", r+1, engine.PatternName(s.Rows[r].Pattern), s.Rows[r].Repeats)
		}
		if r == c.songRow {
			cv.label(560, y, 2, pad(txt, 14))
		} else {
			cv.text(560, y, 2, txt, colFG)
		}
		if on && pos.Row == r {
			cv.text(540, y, 2, ">", colFG)
		}
	}
}

func onOffWord(b bool) string {
	if b {
		return "ON"
	}
	return "OFF"
}

func (c *Controller) drawKit(cv canvas) {
	cur := c.eng.KitNumber()
	c.cell(cv, 0, "KIT", fmt.Sprintf("%02d", c.kitCursor+1), "", -1, false, false)
	cv.textC(cellW*2, cellsY+8, 2, "LOADED: "+fmt.Sprintf("%02d %s", cur+1, strings.ToUpper(c.eng.KitName())), colFG)
	cv.textC(cellW*2, cellsY+32, 2, "MACHINES", colFG)
	for t := 0; t < engine.NumTracks; t++ {
		x := 250 + (t%8)*88
		y := cellsY + 56 + (t/8)*20
		cv.text(x, y, 2, engine.TrackNames[t], colDim)
		_ = x
	}
	cv.text(6, cellsY+60, 2, "LOAD COPIES THE SLOT INTO", colFG)
	cv.text(6, cellsY+76, 2, "THE WORKING KIT. SAVE", colFG)
	cv.text(6, cellsY+92, 2, "WRITES IT BACK.", colFG)
}

func (c *Controller) drawSystem(cv canvas) {
	files := c.host.ImportFiles()
	w := c.eng.Wave(c.waveSlot)
	name, size := "EMPTY", ""
	if w != nil {
		name = strings.ToUpper(w.Name)
		size = fmt.Sprintf("%.2fS", float64(len(w.Data))/float64(w.Rate))
	}
	c.cell(cv, 0, "UW SLOT", fmt.Sprintf("%02d", c.waveSlot+1), name, -1, false, false)
	f := "NONE"
	if len(files) > 0 {
		f = strings.ToUpper(files[clampi(c.fileCur, 0, len(files)-1)])
		if len(f) > 11 {
			f = f[:11]
		}
	}
	c.cell(cv, 1, "FILE", fmt.Sprintf("%d/%d", clampi(c.fileCur+1, 0, len(files)), len(files)), f, -1, false, false)
	sync := "INT"
	if c.eng.Sync() == engine.SyncMIDI {
		sync = "MIDI"
	}
	c.cell(cv, 6, "SYNC", sync, "CLOCK", -1, false, false)
	used, total := c.eng.WaveMemory()
	cv.text(cellW*2+10, cellsY+8, 2, fmt.Sprintf("USERWAVE MEMORY %d%%", used*100/total), colFG)
	cv.bar(cellW*2+10, cellsY+28, 4*cellW, 12, float64(used)/float64(total), colFG)
	cv.text(cellW*2+10, cellsY+50, 2, size, colFG)
	ok, msg := c.host.AudioStatus()
	st := "AUDIO OK"
	if !ok {
		st = "AUDIO: " + upperASCII(strings.SplitN(msg, "\n", 2)[0], 30)
	}
	cv.text(cellW*2+10, cellsY+72, 2, st, colFG)
}

func (c *Controller) drawMachineList(cv canvas) {
	n := len(engine.Machines)
	for k := -2; k <= 2; k++ {
		i := c.ovCursor + k
		if i < 0 || i >= n {
			continue
		}
		y := cellsY + 8 + (k+2)*18
		m := engine.Machines[i]
		txt := fmt.Sprintf("%02d %-8s %s", i+1, m.Name, m.Group)
		if k == 0 {
			cv.label(20, y, 2, pad(txt, 20))
		} else {
			cv.text(20, y, 2, txt, colFG)
		}
	}
	m := engine.Machines[c.ovCursor]
	for i := 0; i < 8; i++ {
		x := 340 + (i%4)*150
		y := cellsY + 12 + (i/4)*24
		cv.text(x, y, 2, fmt.Sprintf("%d %s", i+1, m.Params[i]), colFG)
	}
	cv.text(340, cellsY+62, 2, "TURN A: BROWSE", colFG)
	cv.text(340, cellsY+78, 2, "ENTER: ASSIGN  EXIT: CANCEL", colFG)
}

// drawRibbon draws the 16 steps of the selected track at the bottom of the
// screen: the trigs, the playhead and the accents.
func (c *Controller) drawRibbon(cv canvas) {
	pat := c.eng.PatternCopy(-1)
	pos := c.eng.Position()
	for k := 0; k < 16; k++ {
		s := c.stepPage*16 + k
		x := k*60 + 2
		if s >= pat.Len {
			cv.fill(x, ribbonY+3, 56, 2, colDim)
			continue
		}
		on := pat.Tracks[c.track].Trig[s]
		switch {
		case c.layer == layerAccent:
			on = pat.Accent[s]
		case c.layer == layerSwing:
			on = pat.Swing[s]
		case c.layer == layerSlide:
			on = pat.Slide[s]
		}
		if on {
			cv.fill(x, ribbonY, 56, 8, colFG)
		} else {
			cv.frame(x, ribbonY, 56, 8, colDim)
		}
		if _, ok := c.eng.LockOf(-1, c.track, s); ok && c.layer == layerNone {
			cv.fill(x+24, ribbonY+2, 8, 4, colLight)
		}
		if pos.Playing && pos.Step == s {
			cv.frame(x-1, ribbonY-1, 58, 10, colWarn)
		}
		if s%4 == 0 {
			cv.fill(x, ribbonY-2, 2, 2, colFG)
		}
	}
}

// softLabels are the bottom button names of the current page.
func (c *Controller) softLabels() [8]string {
	var l [8]string
	switch c.page {
	case PgSynth:
		l[0], l[1], l[2] = "MACHINE", "<TRACK", "TRACK>"
		if c.ov == ovMachine {
			l[0] = "ASSIGN"
		}
	case PgFX, PgRoute:
		l[1], l[2] = "<TRACK", "TRACK>"
	case PgMaster:
		l[0], l[1] = "DLY/REV", "CMP/EQ"
	case PgSeq:
		l[0], l[1], l[2], l[3], l[4], l[5] = "CLR PAT", "COPY", "PASTE", "REC", "<SHIFT", "SHIFT>"
	case PgSong:
		l[0], l[1], l[2] = "ADD ROW", "DEL ROW", "SONG ON"
		if on, _ := c.eng.SongMode(); on {
			l[2] = "SONG OFF"
		}
	case PgKit:
		l[0], l[1], l[2] = "LOAD", "SAVE", "INIT"
	case PgSystem:
		l[0], l[1], l[2] = "LOAD UW", "CLEAR UW", "SAVE"
	}
	return l
}

func (c *Controller) drawSoftKeys(cv canvas) {
	cv.fill(0, softY-1, W, 1, colDim)
	for i, s := range c.softLabels() {
		if s == "" {
			continue
		}
		cv.textC(i*cellW+cellW/2, softY+1, 2, s, colFG)
	}
}
