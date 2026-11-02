package audioout

import (
	"testing"

	"github.com/federico-pepe/ableton-push-hack/core/alsapcm"
)

func TestDevices(t *testing.T) {
	list := []alsapcm.PlaybackDevice{{CardIndex: 1, CardID: "PHVAudio", Device: 1, Name: "Loopback PCM"}}
	got := Devices(list, "hw:PHVAudio,1,0")
	if len(got) != 1 || !got[0].Current {
		t.Fatalf("%+v", got)
	}
	got = Devices(list, "hw:Gone,0,0")
	if len(got) != 2 || !got[1].Current || got[0].Current {
		t.Fatalf("unlisted current device: %+v", got)
	}
	if HasDevice(list, "hw:Gone,0,0") || !HasDevice(list, got[0].Value) {
		t.Fatal("HasDevice")
	}
}

func TestPairs(t *testing.T) {
	p := Pairs(6, 2)
	if len(p) != 3 || p[1].Label != "Ch 3-4" || !p[1].Current || p[0].Current {
		t.Fatalf("%+v", p)
	}
	if n := len(Pairs(0, 0)); n != LoopbackChannels/2 {
		t.Fatalf("default has %d pairs", n)
	}
	if !ValidPair(6, 4) || ValidPair(6, 6) || ValidPair(6, 3) || ValidPair(6, -2) {
		t.Fatal("ValidPair")
	}
}
