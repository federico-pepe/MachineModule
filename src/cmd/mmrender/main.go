// mmrender renders MachineModule's engine to a WAV file on any computer, no
// Push needed. It is a demo, a smoke test and a way to hear a change.
//
//	go run ./cmd/mmrender -o demo.wav           a short beat that uses most features
//	go run ./cmd/mmrender -tour -o tour.wav     every machine, one hit each
//	go run ./cmd/mmrender -stats                level and length of every machine
package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"os"

	"machinemodule/engine"
)

const sr = 44100

func main() {
	out := flag.String("o", "demo.wav", "output WAV file")
	bars := flag.Int("bars", 4, "bars of the demo beat")
	bpm := flag.Float64("bpm", 124, "tempo")
	tour := flag.Bool("tour", false, "play every machine once instead of the beat")
	stats := flag.Bool("stats", false, "print level and length per machine and exit")
	flag.Parse()

	switch {
	case *stats:
		printStats()
	case *tour:
		write(*out, renderTour())
	default:
		write(*out, renderBeat(*bars, *bpm))
	}
}

func write(path string, lr [][2]float32) {
	inter := make([]float32, 0, len(lr)*2)
	for _, s := range lr {
		inter = append(inter, s[0], s[1])
	}
	if err := os.WriteFile(path, engine.EncodeWAV16(inter, sr, 2), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %s: %.1f s\n", path, float64(len(lr))/sr)
}

func block(e *engine.Engine, n int) [][2]float32 {
	out := make([][2]float32, 0, n)
	l, r := make([]float32, 128), make([]float32, 128)
	for len(out) < n {
		e.Render(l, r)
		for i := range l {
			out = append(out, [2]float32{l[i], r[i]})
		}
	}
	return out[:n]
}

func renderTour() [][2]float32 {
	var all [][2]float32
	for m := range engine.Machines {
		e := engine.New(sr)
		tone := make([]float32, 11025)
		for i := range tone {
			tone[i] = float32(math.Sin(float64(i)*0.07)) * float32(math.Exp(-float64(i)/3000))
		}
		_ = e.SetWave(0, "TONE", sr, tone)
		e.SetMachine(0, m)
		e.Hit(0, 1)
		all = append(all, block(e, sr)...)
	}
	return all
}

type row struct {
	name                  string
	peak, rms, secs, cent float64
}

func printStats() {
	fmt.Printf("%-9s %7s %7s %7s %8s\n", "machine", "peak", "rms", "len(s)", "centroid")
	for m := range engine.Machines {
		e := engine.New(sr)
		tone := make([]float32, 11025)
		for i := range tone {
			tone[i] = float32(math.Sin(float64(i)*0.07)) * float32(math.Exp(-float64(i)/3000))
		}
		_ = e.SetWave(0, "TONE", sr, tone)
		e.SetMachine(0, m)
		e.Hit(0, 1)
		x := block(e, sr*4)
		var pk, sum float64
		last := 0
		var zc int
		for i, s := range x {
			v := float64(s[0])
			if a := math.Abs(v); a > pk {
				pk = a
			}
			if math.Abs(v) > 0.001 {
				last = i
			}
			sum += v * v
			if i > 0 && (float64(x[i-1][0]) < 0) != (v < 0) {
				zc++
			}
		}
		rms := math.Sqrt(sum / float64(len(x)))
		// a crude brightness: zero crossings per second / 2 over the audible part
		cent := 0.0
		if last > 0 {
			cent = float64(zc) / 2 / (float64(last) / sr)
		}
		fmt.Printf("%-9s %7.3f %7.4f %7.2f %8.0f\n", engine.Machines[m].Name, pk, rms, float64(last)/sr, cent)
	}
}

// renderBeat plays a pattern with accents, swing, a slide, parameter locks
// and a second pattern, all from the sequencer.
func renderBeat(bars int, bpm float64) [][2]float32 {
	e := engine.New(sr)
	e.SetTempo(bpm)
	const (
		BD, SD, HT, MT, LT, CP, RS, CB, CH, OH, RC, CC, M1, M2, M3, M4 = 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15
	)
	e.SetMachine(M1, engine.MachineIndex("GND-SIN"))
	e.SetPatternName(0, "DEMO")
	e.SetPatternLen(0, 16)
	e.SetSwingAmount(0, 58)
	for s := 0; s < 16; s += 4 {
		e.SetTrig(0, BD, s, true)
	}
	e.SetTrig(0, BD, 10, true)
	e.SetLock(0, BD, 10, 1, 40)
	for _, s := range []int{4, 12} {
		e.SetTrig(0, SD, s, true)
	}
	e.SetTrig(0, CP, 12, true)
	for s := 0; s < 16; s++ {
		e.SetTrig(0, CH, s, true)
		if s%2 == 1 {
			e.ToggleLayer(0, engine.LayerSwing, s)
		}
		if s%4 == 2 {
			e.SetLock(0, CH, s, 1, 60) // longer hats on the offbeats
		}
	}
	e.SetTrig(0, OH, 14, true)
	e.SetTrig(0, RS, 6, true)
	e.SetTrig(0, CB, 7, true)
	e.SetLock(0, CB, 7, 0, 30)
	e.ToggleLayer(0, engine.LayerAccent, 0)
	e.ToggleLayer(0, engine.LayerAccent, 8)
	// a sine bass line with a slide
	for i, s := range []int{0, 3, 6, 8, 11, 14} {
		e.SetTrig(0, M1, s, true)
		e.SetLock(0, M1, s, 0, []int{14, 14, 20, 14, 24, 18}[i])
	}
	e.ToggleLayer(0, engine.LayerSlide, 14)
	e.SetParam(M1, 1, 70)
	// the fill pattern
	e.CopyPattern(0, 1)
	for s := 8; s < 16; s++ {
		e.SetTrig(1, HT+(s%3), s, true)
	}
	e.SetGlobal(engine.GDLEV, 70)
	e.SetPatternLen(1, 16)
	e.SelectPattern(0, true)
	e.Play()
	var out [][2]float32
	bar := int(60 / bpm * 4 * sr)
	for b := 0; b < bars; b++ {
		if b == bars-1 {
			e.SelectPattern(1, false)
		}
		out = append(out, block(e, bar)...)
	}
	out = append(out, block(e, sr*2)...)
	return out
}
