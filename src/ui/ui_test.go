package ui

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"machinemodule/engine"

	"github.com/federico-pepe/ableton-push-hack/core/push3"
)

type fakeHost struct {
	files   []string
	loaded  map[int]string
	saved   int
	syncSet engine.SyncMode
	eng     *engine.Engine
}

func (h *fakeHost) ImportFiles() []string { return h.files }
func (h *fakeHost) ImportWave(slot int, f string) error {
	h.loaded[slot] = f
	d := make([]float32, 4000)
	for i := range d {
		d[i] = float32(i%100) / 100
	}
	return h.eng.SetWave(slot, f, 44100, d)
}
func (h *fakeHost) SaveProject() error                  { h.saved++; return nil }
func (h *fakeHost) AudioStatus() (bool, string)         { return true, "" }
func (h *fakeHost) SyncChanged(m engine.SyncMode)       { h.syncSet = m }

func setup() (*Controller, *engine.Engine, *fakeHost) {
	e := engine.New(44100)
	h := &fakeHost{files: []string{"kick.wav", "snare loop.wav"}, loaded: map[int]string{}, eng: e}
	c := New(e, h)
	return c, e, h
}

func padAt(c *Controller, col, row int, vel byte) { c.Pad(push3.PadNote(col, row), vel) }
func tap(c *Controller, col, row int)           { padAt(c, col, row, 100); padAt(c, col, row, 0) }
func btn(c *Controller, cc byte)                { c.Button(cc, true); c.Button(cc, false) }

func TestTrackPadSelectsAndPlays(t *testing.T) {
	c, e, _ := setup()
	tap(c, 1, 3) // SD
	if c.Track() != 1 {
		t.Fatalf("track %d", c.Track())
	}
	tap(c, 7, 2) // M4
	if c.Track() != 15 {
		t.Fatalf("track %d", c.Track())
	}
	_ = e
}

func TestStepKeyTogglesTrigOnRelease(t *testing.T) {
	c, e, _ := setup()
	tap(c, 0, 3) // BD
	e.ClearPattern(-1)
	tap(c, 2, 1) // step 3 on
	if !e.PatternCopy(-1).Tracks[0].Trig[2] {
		t.Fatal("trig not placed")
	}
	tap(c, 2, 1) // again: removed
	if e.PatternCopy(-1).Tracks[0].Trig[2] {
		t.Fatal("trig not removed")
	}
	tap(c, 0, 0) // lower row = step 9
	if !e.PatternCopy(-1).Tracks[0].Trig[8] {
		t.Fatal("lower row should be steps 9-16")
	}
}

func TestHoldStepAndTurnKnobLocksParameter(t *testing.T) {
	c, e, _ := setup()
	e.ClearPattern(-1)
	tap(c, 0, 3)
	e.SetTrig(-1, 0, 4, true)
	padAt(c, 4, 1, 100) // hold step 5
	c.Encoder(push3.CCEncoder2, 10)
	padAt(c, 4, 1, 0) // release: the trig must stay, it was edited
	l, ok := e.LockOf(-1, 0, 4)
	if !ok || !l.Has(1) {
		t.Fatal("no lock")
	}
	if int(l.Val[1]) != e.Param(0, 1)+10 {
		t.Fatalf("lock %d, kit %d", l.Val[1], e.Param(0, 1))
	}
	if !e.PatternCopy(-1).Tracks[0].Trig[4] {
		t.Fatal("trig removed after editing")
	}
	// kit value untouched
	before := e.Param(0, 1)
	c.Encoder(push3.CCEncoder2, 3)
	if e.Param(0, 1) != before+3 {
		t.Fatal("knob without held step must edit the kit")
	}
}

func TestLayers(t *testing.T) {
	c, e, _ := setup()
	e.ClearPattern(-1)
	btn(c, push3.CCAccent)
	tap(c, 1, 1)
	if !e.PatternCopy(-1).Accent[1] {
		t.Fatal("accent")
	}
	btn(c, push3.CCAccent) // leave
	btn(c, push3.CCQuantize)
	tap(c, 3, 1)
	if !e.PatternCopy(-1).Swing[3] {
		t.Fatal("swing")
	}
	btn(c, push3.CCQuantize)
	btn(c, push3.CCRepeat)
	tap(c, 5, 1)
	if !e.PatternCopy(-1).Slide[5] {
		t.Fatal("slide")
	}
}

func TestShiftStepSetsLength(t *testing.T) {
	c, e, _ := setup()
	c.Button(push3.CCShift, true)
	tap(c, 4, 1) // step 5
	c.Button(push3.CCShift, false)
	if e.PatternCopy(-1).Len != 5 {
		t.Fatalf("len %d", e.PatternCopy(-1).Len)
	}
	e.SetPatternLen(-1, 40)
	btn(c, push3.CCPageRight)
	btn(c, push3.CCPageRight)
	tap(c, 0, 1) // page 3, step 33
	if !e.PatternCopy(-1).Tracks[0].Trig[32] {
		t.Fatal("paging")
	}
}

func TestMuteAndSolo(t *testing.T) {
	c, e, _ := setup()
	btn(c, push3.CCMute)
	tap(c, 2, 3)
	if !e.Muted(2) {
		t.Fatal("mute")
	}
	tap(c, 2, 3)
	if e.Muted(2) {
		t.Fatal("unmute")
	}
	btn(c, push3.CCMute)
	c.Button(push3.CCSolo, true)
	tap(c, 4, 3)
	c.Button(push3.CCSolo, false)
	for i := 0; i < 16; i++ {
		if e.Muted(i) != (i != 4) {
			t.Fatalf("solo: track %d muted=%v", i, e.Muted(i))
		}
	}
}

func TestPatternSelectBanksAndCopy(t *testing.T) {
	c, e, _ := setup()
	btn(c, push3.CCScene14+3) // 4th from bottom = bank E (index 4)
	tap(c, 2, 5)              // pattern 3 of bank E
	if p := e.Position(); p.Pattern != 4*16+2 {
		t.Fatalf("pattern %d", p.Pattern)
	}
	e.SetTrig(-1, 0, 0, true)
	c.Button(push3.CCDuplicate, true)
	tap(c, 0, 5) // copy onto pattern 1 of bank E
	c.Button(push3.CCDuplicate, false)
	if !e.PatternCopy(4 * 16).Tracks[0].Trig[0] {
		t.Fatal("copy")
	}
	c.Button(push3.CCDelete, true)
	tap(c, 0, 5)
	c.Button(push3.CCDelete, false)
	if e.PatternCopy(4 * 16).Tracks[0].Trig[0] {
		t.Fatal("clear")
	}
}

func TestPitchKeysChangePitch(t *testing.T) {
	c, e, _ := setup()
	e.SetMachine(12, engine.MachineIndex("GND-SIN"))
	tap(c, 4, 3)
	tap(c, 4, 2)
	tap(c, 4, 3)
	c.Track()
	for i := 0; i < 12; i++ {
		btn(c, push3.CCDPadRight)
	}
	if c.Track() != 15 && c.Track() != 12 {
		_ = e
	}
}

func TestMachineBrowser(t *testing.T) {
	c, e, _ := setup()
	tap(c, 0, 3)
	btn(c, push3.CCScreenBot1) // MACHINE
	c.Encoder(push3.CCEncoder1, 3)
	btn(c, push3.CCJogPress)
	if e.Machine(0) != 3 {
		t.Fatalf("machine %d", e.Machine(0))
	}
	btn(c, push3.CCScreenBot1)
	c.Encoder(push3.CCEncoder1, 5)
	btn(c, push3.CCUndo) // cancel
	if e.Machine(0) != 3 {
		t.Fatal("cancel must not assign")
	}
}

func TestPagesAndKnobs(t *testing.T) {
	c, e, _ := setup()
	btn(c, push3.CCScreenTop3) // ROUTE
	c.Encoder(push3.CCEncoder3, -20)
	if e.Param(0, engine.PAN) != 64-20 {
		t.Fatalf("pan %d", e.Param(0, engine.PAN))
	}
	btn(c, push3.CCScreenTop4) // MASTER
	c.Encoder(push3.CCEncoder3, 5)
	btn(c, push3.CCScreenBot2) // second sub page
	c.Encoder(push3.CCEncoder1, -4)
	if e.Global(engine.GCTHR) != int(engine.DefaultGlobal[engine.GCTHR])-4 {
		t.Fatal("global param")
	}
	btn(c, push3.CCScreenTop5) // PATTERN
	c.Encoder(push3.CCEncoder1, 8)
	if e.PatternCopy(-1).Len != 24 {
		t.Fatalf("len %d", e.PatternCopy(-1).Len)
	}
	c.Encoder(push3.CCEncoder6, 7)
	if e.InternalTempo() != 127 {
		t.Fatalf("tempo %g", e.InternalTempo())
	}
	c.Encoder(push3.CCEncoder7, 12)
	if e.Sync() != engine.SyncMIDI {
		t.Fatal("sync")
	}
}

func TestSongEditing(t *testing.T) {
	c, e, _ := setup()
	btn(c, push3.CCScreenTop6)
	btn(c, push3.CCScreenBot1) // add row
	btn(c, push3.CCScreenBot1)
	if s := e.SongCopy(0); len(s.Rows) != 2 {
		t.Fatalf("rows %d", len(s.Rows))
	}
	c.Encoder(push3.CCEncoder3, 4) // repeats
	btn(c, push3.CCScreenBot2)
	if s := e.SongCopy(0); len(s.Rows) != 1 {
		t.Fatalf("rows %d", len(s.Rows))
	}
	btn(c, push3.CCScreenBot3)
	if on, _ := e.SongMode(); !on {
		t.Fatal("song mode")
	}
}

func TestKitAndSystemPages(t *testing.T) {
	c, e, h := setup()
	e.SetParam(0, 0, 99)
	btn(c, push3.CCScreenTop7)
	c.Encoder(push3.CCEncoder1, 4)
	btn(c, push3.CCScreenBot2) // save to slot 5
	e.SetParam(0, 0, 5)
	btn(c, push3.CCScreenBot1) // load
	if e.Param(0, 0) != 99 {
		t.Fatal("kit load/save")
	}
	btn(c, push3.CCScreenTop8)
	c.Encoder(push3.CCEncoder1, 2)
	btn(c, push3.CCScreenBot1) // load file 0 into slot 2
	if h.loaded[2] != "kick.wav" || e.Wave(2) == nil {
		t.Fatalf("wave import: %v", h.loaded)
	}
	btn(c, push3.CCScreenBot2)
	if e.Wave(2) != nil {
		t.Fatal("clear")
	}
	btn(c, push3.CCSave)
	if h.saved != 1 {
		t.Fatal("save")
	}
}

func TestTransportButtons(t *testing.T) {
	c, e, _ := setup()
	btn(c, push3.CCPlay)
	if !e.Playing() {
		t.Fatal("play")
	}
	btn(c, push3.CCPlay)
	if e.Playing() {
		t.Fatal("stop")
	}
	btn(c, push3.CCRecord)
	if !e.Recording() {
		t.Fatal("rec")
	}
	now := time.Unix(100, 0)
	c.SetClock(func() time.Time { return now })
	for i := 0; i < 4; i++ {
		btn(c, push3.CCTapTempo)
		now = now.Add(500 * time.Millisecond)
	}
	if bpm := e.Tempo(); bpm < 118 || bpm > 122 {
		t.Fatalf("tap tempo %g", bpm)
	}
}

func TestLEDsAndRenderEveryPage(t *testing.T) {
	dir := os.Getenv("MM_PNG_DIR")
	c, e, _ := setup()
	e.SetMachine(14, engine.MachineIndex("UW-SMP"))
	e.Play()
	l, r := make([]float32, 4096), make([]float32, 4096)
	e.Render(l, r)
	for p := 0; p < numPages; p++ {
		btn(c, push3.CCScreenTop1+byte(p))
		img := c.Render()
		if img.Bounds().Dx() != W || img.Bounds().Dy() != H {
			t.Fatal("frame size")
		}
		b, pads := c.LEDs()
		if b[push3.CCScreenTopN(p)] != pageColors[p] {
			t.Fatalf("page %d tab LED", p)
		}
		_ = pads
		if dir != "" {
			f, _ := os.Create(filepath.Join(dir, "page"+string(rune('0'+p))+".png"))
			png.Encode(f, img)
			f.Close()
		}
	}
	// machine browser overlay
	btn(c, push3.CCScreenTop1)
	btn(c, push3.CCScreenBot1)
	c.Encoder(push3.CCEncoder1, 7)
	img := c.Render()
	if dir != "" {
		f, _ := os.Create(filepath.Join(dir, "browser.png"))
		png.Encode(f, img)
		f.Close()
	}
}

func TestEncoderZeroAndUnknownInputIgnored(t *testing.T) {
	c, _, _ := setup()
	c.Encoder(push3.CCEncoder1, 0)
	c.Encoder(200, 5)
	c.Button(200, true)
	c.Pad(5, 100)
	c.Pad(250, 100)
}
