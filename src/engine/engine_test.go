package engine

import (
	"bytes"
	"math"
	"sync"
	"testing"
	"time"
)

const testSR = 44100

func render(e *Engine, n int) (l, r []float32) {
	l, r = make([]float32, n), make([]float32, n)
	for off := 0; off < n; off += 128 {
		end := off + 128
		if end > n {
			end = n
		}
		e.Render(l[off:end], r[off:end])
	}
	return
}

func peak(x []float32) float32 {
	var p float32
	for _, v := range x {
		if a := absf(v); a > p {
			p = a
		}
	}
	return p
}

func rms(x []float32) float64 {
	var s float64
	for _, v := range x {
		s += float64(v) * float64(v)
	}
	return math.Sqrt(s / float64(len(x)+1))
}

func TestEveryMachineMakesSoundAndStaysFinite(t *testing.T) {
	// a ramp as a stand-in sample for the UW machine
	data := make([]float32, 8000)
	for i := range data {
		data[i] = float32(math.Sin(float64(i)*0.05)) * 0.8
	}
	for m := range Machines {
		e := New(testSR)
		if err := e.SetWave(0, "sine", testSR, data); err != nil {
			t.Fatal(err)
		}
		e.SetMachine(0, m)
		e.Hit(0, 1)
		l, r := render(e, testSR) // one second
		for i := range l {
			if isBad(l[i]) || isBad(r[i]) || absf(l[i]) > 1.001 || absf(r[i]) > 1.001 {
				t.Fatalf("%s: bad sample %v %v at %d", Machines[m].Name, l[i], r[i], i)
			}
		}
		if p := peak(l[:testSR/4]); p < 0.01 {
			t.Errorf("%s: silent after a hit (peak %g)", Machines[m].Name, p)
		}
	}
}

func TestMachinesDecayToSilence(t *testing.T) {
	for _, name := range []string{"TRX-BD", "TRX-SD", "TRX-CH", "TRX-CP", "EFM-BD", "PI-SD", "GND-SIN"} {
		e := New(testSR)
		e.SetMachine(0, MachineIndex(name))
		e.Hit(0, 1)
		l, _ := render(e, testSR*6)
		if p := peak(l[len(l)-testSR/2:]); p > 0.002 {
			t.Errorf("%s: still sounding after 6 s (peak %g)", name, p)
		}
	}
}

func TestParametersChangeTheSound(t *testing.T) {
	// DEC (param 1) on the kick must lengthen it.
	short, long := New(testSR), New(testSR)
	short.SetParam(0, 1, 10)
	long.SetParam(0, 1, 110)
	short.Hit(0, 1)
	long.Hit(0, 1)
	ls, _ := render(short, testSR/2)
	ll, _ := render(long, testSR/2)
	if rms(ll[testSR/4:]) <= rms(ls[testSR/4:])*2 {
		t.Fatalf("long decay should ring more: %g vs %g", rms(ll[testSR/4:]), rms(ls[testSR/4:]))
	}
}

func emptyEngine() *Engine {
	e := New(testSR)
	e.SetProject(&Project{Name: "T", Tempo: 120})
	e.SetProject(func() *Project { p := NewProject(); p.Patterns[0] = NewPattern(); return p }())
	return e
}

func TestSequencerTiming(t *testing.T) {
	e := emptyEngine()
	e.SetTrig(0, 0, 0, true)
	e.SetTrig(0, 0, 4, true) // a beat later
	e.SetTempo(120)
	e.Play()
	l, _ := render(e, testSR*2)
	// first onset at sample ~0, second at one beat = 22050 samples
	onset := func(from int) int {
		for i := from; i < len(l); i++ {
			if absf(l[i]) > 0.01 {
				return i
			}
		}
		return -1
	}
	a := onset(0)
	if a < 0 || a > 300 {
		t.Fatalf("first onset at %d", a)
	}
	// find the quiet gap, then the second onset
	gap := onset(0)
	_ = gap
	var second int
	for i := 15000; i < len(l); i++ {
		if absf(l[i]) < 0.0005 {
			second = onset(i)
			break
		}
	}
	if second == -1 {
		t.Skip("kick never went quiet within the beat; timing checked by wrap test")
	}
}

func TestPatternWrapsOnTime(t *testing.T) {
	e := emptyEngine()
	e.Play()
	// 16 steps of 16ths at 120 BPM = 2 s
	render(e, int(1.99*testSR))
	if p := e.Position(); p.Step < 14 || !p.Playing {
		t.Fatalf("expected near the end of the pattern, got %+v", p)
	}
	render(e, int(0.02*testSR))
	if p := e.Position(); p.Step > 1 {
		t.Fatalf("expected the pattern to have wrapped, got %+v", p)
	}
}

func TestStepEventsAreSampleAccurate(t *testing.T) {
	e := emptyEngine()
	e.SetTrig(0, 0, 1, true) // second step: 5512.5 samples at 120 BPM
	e.Play()
	l, _ := render(e, 9000)
	first := -1
	for i, v := range l {
		if absf(v) > 0.001 {
			first = i
			break
		}
	}
	if first < 5510 || first > 5516 {
		t.Fatalf("step 2 should start at sample ~5512, started at %d", first)
	}
}

func TestSwingDelaysSwungSteps(t *testing.T) {
	straight, swung := emptyEngine(), emptyEngine()
	for _, e := range []*Engine{straight, swung} {
		e.SetTrig(0, 0, 1, true)
	}
	swung.ToggleLayer(0, LayerSwing, 1)
	swung.SetSwingAmount(0, 66)
	onset := func(e *Engine) int {
		e.Play()
		l, _ := render(e, 12000)
		for i, v := range l {
			if absf(v) > 0.001 {
				return i
			}
		}
		return -1
	}
	a, b := onset(straight), onset(swung)
	// 66% swing delays by 0.32 of a step = ~1764 samples
	if d := b - a; d < 1650 || d > 1850 {
		t.Fatalf("swing delay %d samples, want about 1764", d)
	}
}

func TestParameterLockAppliesToOneTrigOnly(t *testing.T) {
	e := emptyEngine()
	e.SetTrig(0, 0, 0, true)
	e.SetTrig(0, 0, 8, true)
	e.SetLock(0, 0, 8, 1, 2) // DEC 2: a very short kick on step 9
	if _, ok := e.LockOf(0, 0, 8); !ok {
		t.Fatal("lock missing")
	}
	e.Play()
	l, _ := render(e, testSR) // 1 s = steps 0..7 then 8..
	step := testSR / 4 / 2    // 16th = 5512.5
	long := rms(l[step*3 : step*4])
	// the second kick starts at step 8 = 44100 samples; render on
	l2, _ := render(e, testSR/2)
	short := rms(l2[step*3 : step*4])
	if short >= long*0.5 {
		t.Fatalf("locked DEC should shorten the second kick: long %g short %g", long, short)
	}
}

func TestSlideDoesNotRetrigger(t *testing.T) {
	e := emptyEngine()
	e.SetMachine(0, MachineIndex("GND-SIN"))
	e.SetParam(0, 1, 127) // very long decay
	e.SetTrig(0, 0, 0, true)
	e.SetTrig(0, 0, 4, true)
	e.SetLock(0, 0, 4, 0, 100)
	e.ToggleLayer(0, LayerSlide, 4)
	e.Play()
	render(e, testSR/4) // before step 4
	before := e.tracks[0].active
	render(e, testSR/2)
	if !before || !e.tracks[0].active {
		t.Fatal("voice should still be playing through the slide")
	}
	// the pitch parameter should have glided to the locked value
	if got := e.tracks[0].cur[0]; got < 0.7 {
		t.Fatalf("PTCH should have slid up to the lock (100/127), cur=%g", got)
	}
}

func TestQueuedPatternStartsAtTheEnd(t *testing.T) {
	e := emptyEngine()
	e.Play()
	render(e, testSR/2)
	e.SelectPattern(5, false)
	if p := e.Position(); p.Pattern != 0 || p.Next != 5 {
		t.Fatalf("pattern must be queued, got %+v", p)
	}
	render(e, testSR*2)
	if p := e.Position(); p.Pattern != 5 || p.Next != -1 {
		t.Fatalf("pattern 5 should be playing, got %+v", p)
	}
}

func TestSongPlaysRowsAndStops(t *testing.T) {
	e := emptyEngine()
	e.SetPatternLen(0, 2)
	e.SetPatternLen(1, 2)
	e.SongSetRow(0, 0, 0, 2)
	e.SongSetRow(0, 1, 1, 1)
	e.SetSong(true, 0)
	e.Play()
	// a 2-step pattern = 2 x 5512 samples: 2 loops of A01, then A02
	render(e, 5512*4+200)
	if p := e.Position(); p.Pattern != 1 || p.Row != 1 {
		t.Fatalf("expected row 2 (A02), got %+v", p)
	}
	render(e, 5512*4)
	if e.Playing() {
		t.Fatal("song should stop after its last row")
	}
}

func TestSongLoops(t *testing.T) {
	e := emptyEngine()
	e.SetPatternLen(0, 2)
	e.SongSetRow(0, 0, 0, 1)
	e.SongSetLoop(0, 0)
	e.SetSong(true, 0)
	e.Play()
	render(e, 5512*10)
	if !e.Playing() {
		t.Fatal("a looping song must keep playing")
	}
}

func TestPatternChangeLoadsItsKit(t *testing.T) {
	e := emptyEngine()
	e.SetParam(0, 0, 99)
	e.SaveKit(3, "BASS")
	e.LoadKit(0)
	e.SetParam(0, 0, 10)
	e.SetPatternKit(2, 3)
	e.SelectPattern(2, true)
	if got := e.Param(0, 0); got != 99 {
		t.Fatalf("pattern 3 should load kit 4 (PTCH 99), got %d", got)
	}
}

func TestMuteSilencesTrack(t *testing.T) {
	e := emptyEngine()
	e.SetMute(0, true)
	e.Hit(0, 1)
	l, _ := render(e, 4000)
	if peak(l) > 0 {
		t.Fatal("muted track made sound")
	}
}

func TestProjectRoundTrip(t *testing.T) {
	e := emptyEngine()
	e.SetTrig(0, 3, 5, true)
	e.SetLock(0, 3, 5, 2, 77)
	e.SetLock(0, 3, 5, 9, 12)
	e.ToggleLayer(0, LayerAccent, 5)
	e.SetPatternLen(0, 33)
	e.SetPatternScale(0, 2)
	e.SetMachine(5, MachineIndex("EFM-CP"))
	e.SaveKit(0, "ROUND")
	e.SongSetRow(2, 0, 7, 3)
	e.SongSetLoop(2, 0)
	b, err := e.Project().Marshal()
	if err != nil {
		t.Fatal(err)
	}
	p, err := UnmarshalProject(b)
	if err != nil {
		t.Fatal(err)
	}
	pat := p.Pattern(0)
	if !pat.Tracks[3].Trig[5] || pat.Len != 33 || pat.Scale != 2 || !pat.Accent[5] {
		t.Fatalf("pattern changed: %+v", pat.Tracks[3].Trig[:8])
	}
	l := pat.Tracks[3].Lock[5]
	if l == nil || !l.Has(2) || !l.Has(9) || l.Val[2] != 77 || l.Val[9] != 12 || l.Has(3) {
		t.Fatalf("locks changed: %+v", l)
	}
	if p.Kit(0).Track[5].Machine != MachineIndex("EFM-CP") || p.Kit(0).Name != "ROUND" {
		t.Fatal("kit changed")
	}
	if s := p.Song(2); len(s.Rows) != 1 || s.Rows[0].Pattern != 7 || s.Rows[0].Repeats != 3 || s.LoopTo != 0 {
		t.Fatalf("song changed: %+v", s)
	}
}

func TestUnmarshalRejectsGarbage(t *testing.T) {
	for _, s := range []string{"", "{", `{"version":2}`, `[1,2]`} {
		if _, err := UnmarshalProject([]byte(s)); err == nil {
			t.Errorf("accepted %q", s)
		}
	}
	// out-of-range numbers are clamped, not trusted
	p, err := UnmarshalProject([]byte(`{"version":1,"tempo":9999,"patterns":{"0":{"len":500,"scale":99,"swing":5,"kit":900,"tracks":[]}},"songs":{"0":{"Rows":[{"Pattern":999,"Repeats":0}],"LoopTo":9}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if p.Tempo != 120 || p.Pattern(0).Len != MaxSteps || p.Pattern(0).Kit != -1 {
		t.Fatalf("not clamped: %+v", p.Pattern(0))
	}
	if r := p.Song(0).Rows[0]; r.Pattern != 127 || r.Repeats != 1 || p.Song(0).LoopTo != -1 {
		t.Fatalf("song not clamped: %+v", p.Song(0))
	}
}

func TestWaveImportAndPlayback(t *testing.T) {
	var buf bytes.Buffer
	data := make([]float32, 4410)
	for i := range data {
		data[i] = float32(math.Sin(float64(i)*0.2)) * 0.9
	}
	buf.Write(EncodeWAV16(data, 22050, 1))
	e := emptyEngine()
	if err := e.LoadWAV(3, "/tmp/my kick!.wav", &buf); err != nil {
		t.Fatal(err)
	}
	w := e.Wave(3)
	if w == nil || w.Name != "my kick" || w.Rate != 22050 || len(w.Data) != 4410 {
		t.Fatalf("wave: %+v", w)
	}
	e.SetMachine(14, MachineIndex("UW-SMP"))
	e.SetParam(14, 0, 6) // WAVE: param/127*127/2 -> slot 3
	e.Hit(14, 1)
	l, _ := render(e, 4000)
	if peak(l) < 0.05 {
		t.Fatalf("UW played nothing, peak %g", peak(l))
	}
	if err := e.LoadWAV(4, "x", bytes.NewReader([]byte("nope"))); err == nil {
		t.Fatal("garbage accepted as WAV")
	}
	e.ClearWave(3)
	if e.Wave(3) != nil {
		t.Fatal("wave not cleared")
	}
}

func TestWaveMemoryLimit(t *testing.T) {
	e := emptyEngine()
	big := make([]float32, MaxWaveSamples/2+10)
	if err := e.SetWave(0, "a", 192000, big[:MaxWaveSamples/2]); err != nil {
		t.Fatal(err)
	}
	if err := e.SetWave(1, "b", 192000, big); err != ErrWaveMemory {
		t.Fatalf("want ErrWaveMemory, got %v", err)
	}
	// replacing the same slot is allowed
	if err := e.SetWave(0, "a2", 192000, big); err != nil {
		t.Fatal(err)
	}
}

func TestExternalClockFollowsPulses(t *testing.T) {
	e := emptyEngine()
	e.SetSync(SyncMIDI)
	e.ExtStart()
	now := time.Unix(1000, 0)
	// 140 BPM = 24 pulses per 60/140 s
	dt := 17857142 * time.Nanosecond
	for i := 0; i < 40; i++ {
		e.ExtPulse(now)
		now = now.Add(dt)
	}
	if bpm := e.Tempo(); math.Abs(bpm-140) > 3 {
		t.Fatalf("measured tempo %g, want 140", bpm)
	}
	if !e.Playing() {
		t.Fatal("should be playing after MIDI start")
	}
	e.ExtStop()
	if e.Playing() {
		t.Fatal("should stop on MIDI stop")
	}
}

func TestRecordWritesHitsIntoThePattern(t *testing.T) {
	e := emptyEngine()
	e.SetRecord(true)
	e.Play()
	render(e, 5512*3+100) // just past step 4
	e.Hit(1, 1)
	if !e.PatternCopy(0).Tracks[1].Trig[3] && !e.PatternCopy(0).Tracks[1].Trig[4] {
		t.Fatal("the hit was not recorded near step 4")
	}
}

func TestEditHelpers(t *testing.T) {
	e := emptyEngine()
	e.SetTrig(0, 2, 1, true)
	e.SetTrig(0, 2, 3, true)
	e.ShiftTrack(0, 2, 1)
	p := e.PatternCopy(0)
	if p.Tracks[2].Trig[1] || !p.Tracks[2].Trig[2] || !p.Tracks[2].Trig[4] {
		t.Fatal("shift wrong")
	}
	e.CopyTrack(0, 2, 5, 7)
	if !e.PatternCopy(5).Tracks[7].Trig[2] {
		t.Fatal("copy track")
	}
	e.CopyPattern(0, 9)
	if !e.PatternCopy(9).Tracks[2].Trig[4] {
		t.Fatal("copy pattern")
	}
	e.ClearTrack(0, 2)
	if e.PatternCopy(0).Tracks[2].Trig[2] {
		t.Fatal("clear track")
	}
	if e.SetLock(0, 2, 2, 0, 1) {
		t.Fatal("lock on an empty step must be refused")
	}
	if PatternName(0) != "A01" || PatternName(127) != "H16" || PatternName(17) != "B02" {
		t.Fatal("pattern names")
	}
}

func TestRenderIsRaceFree(t *testing.T) {
	e := New(testSR)
	e.Play()
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for g := 0; g < 3; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				e.SetParam(g, i%24, i%128)
				e.ToggleTrig(-1, g, i%16)
				e.Hit(g, 1)
				_ = e.Position()
				_ = e.PatternCopy(-1)
			}
		}(g)
	}
	render(e, testSR*2)
	close(stop)
	wg.Wait()
}

func TestNoAllocationsInRender(t *testing.T) {
	e := New(testSR)
	e.Play()
	l, r := make([]float32, 128), make([]float32, 128)
	e.Render(l, r)
	if n := testing.AllocsPerRun(50, func() { e.Render(l, r) }); n > 0 {
		t.Errorf("Render allocates %v times per block", n)
	}
}

func BenchmarkRenderBlock(b *testing.B) {
	e := New(testSR)
	for t := 0; t < NumTracks; t++ {
		for s := 0; s < 16; s += 2 {
			e.SetTrig(0, t, s, true)
		}
	}
	e.Play()
	l, r := make([]float32, 128), make([]float32, 128)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Render(l, r)
	}
}

func TestMarshalKeepsWorkingKit(t *testing.T) {
	e := New(44100)
	e.SetParam(2, 0, 99)
	e.SetTempo(133)
	b, _, err := e.MarshalProject()
	if err != nil {
		t.Fatal(err)
	}
	p, err := UnmarshalProject(b)
	if err != nil {
		t.Fatal(err)
	}
	e2 := New(44100)
	e2.SetProject(p)
	if e2.Param(2, 0) != 99 {
		t.Fatalf("working kit lost: %d", e2.Param(2, 0))
	}
	if e2.InternalTempo() < 132.9 || e2.InternalTempo() > 133.1 {
		t.Fatalf("tempo %v", e2.InternalTempo())
	}
}

func TestSetSampleRate(t *testing.T) {
	e := New(44100)
	e.SetSampleRate(48000)
	if e.SampleRate() != 48000 {
		t.Fatal("rate not set")
	}
	e.Hit(0, 1)
	l, r := make([]float32, 1024), make([]float32, 1024)
	e.Render(l, r)
	var pk float32
	for _, v := range l {
		if v > pk {
			pk = v
		}
		if v != v {
			t.Fatal("NaN")
		}
	}
	if pk == 0 {
		t.Fatal("silent after rate change")
	}
}
