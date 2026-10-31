package ui

import (
	"machinemodule/engine"

	"github.com/federico-pepe/ableton-push-hack/core/push3"
)

func pal(name string) byte {
	i, ok := push3.ColorByName(name)
	if !ok {
		panic("ui: unknown palette colour " + name)
	}
	return i
}

var (
	ledOff   = byte(0)
	ledWhite = pal("white")
	ledDim   = pal("dgray")
	ledGreen = pal("green")
	ledRed   = pal("red")
	ledAmber = pal("amber")
	ledBlue  = pal("blue")

	trackColors = [engine.NumTracks]byte{
		pal("red"), pal("orange"), pal("amber"), pal("yellow"),
		pal("lime"), pal("green"), pal("teal"), pal("sky"),
		pal("blue"), pal("indigo"), pal("violet"), pal("purple"),
		pal("crimson"), pal("pink"), pal("peach"), pal("mint"),
	}
	trackDim = [engine.NumTracks]byte{
		pal("dk_crimson"), pal("maroon"), pal("sienna"), pal("khaki"),
		pal("olive"), pal("forest"), pal("teal"), pal("cobalt"),
		pal("navy"), pal("dk_violet"), pal("dk_violet"), pal("dk_magenta"),
		pal("dk_crimson"), pal("dk_pink"), pal("sienna"), pal("forest"),
	}
	pageColors = [numPages]byte{
		pal("red"), pal("orange"), pal("amber"), pal("lime"),
		pal("green"), pal("sky"), pal("violet"), pal("white"),
	}
)

// LEDs says what every lit control shows. Buttons maps a CC to a palette
// index (RGB buttons) or a brightness (white buttons); pads is indexed
// row*8+col from the bottom left, as the pad notes are. Controls not in the
// map are dark.
func (c *Controller) LEDs() (buttons map[byte]byte, pads [64]byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b := map[byte]byte{}
	pos := c.eng.Position()
	pat := c.eng.PatternCopy(-1)

	for i := 0; i < numPages; i++ {
		v := ledDim
		if i == c.page {
			v = pageColors[i]
		}
		b[push3.CCScreenTopN(i)] = v
	}
	for i, l := range c.softLabels() {
		if l != "" {
			b[push3.CCScreenBotN(i)] = ledWhite
		}
	}
	// scene column: CC 36 is the bottom button, so bank A (top) is the highest CC
	for i := 0; i < 8; i++ {
		v := ledDim
		if 7-i == c.bank {
			v = ledAmber
		}
		b[byte(push3.CCScene14)+byte(i)] = v
	}
	if pos.Playing {
		b[push3.CCPlay] = 127
	} else {
		b[push3.CCPlay] = 40
	}
	if c.eng.Recording() {
		b[push3.CCRecord] = 127
	} else {
		b[push3.CCRecord] = 30
	}
	on := func(cc byte, v bool) {
		if v {
			b[cc] = 127
		} else {
			b[cc] = 30
		}
	}
	on(push3.CCMute, c.muteMode)
	on(push3.CCAccent, c.layer == layerAccent)
	on(push3.CCQuantize, c.layer == layerSwing)
	on(push3.CCRepeat, c.layer == layerSlide)
	on(push3.CCDuplicate, c.cpy)
	on(push3.CCDelete, c.clear)
	on(push3.CCShift, c.shift)
	b[push3.CCSave] = 40
	b[push3.CCTapTempo] = 40
	b[push3.CCConvert] = 40
	b[push3.CCOctaveUp], b[push3.CCOctaveDown] = 40, 40
	b[push3.CCPageLeft], b[push3.CCPageRight] = 40, 40
	b[push3.CCDPadUp], b[push3.CCDPadDown], b[push3.CCDPadLeft], b[push3.CCDPadRight] = 40, 40, 40, 40

	// pads
	set := func(col, row int, v byte) { pads[row*8+col] = v }
	for k := 0; k < 16; k++ {
		s := c.stepPage*16 + k
		col, row := k%8, 1-k/8
		if s >= pat.Len {
			set(col, row, ledOff)
			continue
		}
		v := ledDim
		switch c.layer {
		case layerAccent:
			if pat.Accent[s] {
				v = ledRed
			}
		case layerSwing:
			if pat.Swing[s] {
				v = ledBlue
			}
		case layerSlide:
			if pat.Slide[s] {
				v = ledGreen
			}
		default:
			if pat.Tracks[c.track].Trig[s] {
				v = trackColors[c.track]
			}
		}
		if pos.Playing && pos.Step == s {
			v = ledWhite
		}
		set(col, row, v)
	}
	mask := c.eng.MuteMask()
	for t := 0; t < engine.NumTracks; t++ {
		col, row := t%8, 3-t/8
		v := trackDim[t]
		if t == c.track {
			v = trackColors[t]
		}
		if mask&(1<<uint(t)) != 0 {
			v = ledOff
			if t == c.track {
				v = ledDim
			}
		}
		set(col, row, v)
	}
	for k := 0; k < 16; k++ {
		p := c.bank*16 + k
		col, row := k%8, 5-k/8
		v := ledOff
		if pp := c.eng.PatternCopy(p); !pp.Empty() {
			v = pal("cobalt")
		}
		if p == pos.Next {
			v = ledAmber
		}
		if p == pos.Pattern {
			v = ledGreen
		}
		set(col, row, v)
	}
	for k := 0; k < 16; k++ {
		col, row := k%8, 7-k/8
		v := ledDim
		if (k-4)%12 == 0 {
			v = pal("amber")
		}
		set(col, row, v)
	}
	return b, pads
}
