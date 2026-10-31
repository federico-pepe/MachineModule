package midimap

import "testing"

func TestNoteTrack(t *testing.T) {
	if tr, ok := NoteTrack(36); !ok || tr != 0 {
		t.Fatal("36")
	}
	if tr, ok := NoteTrack(51); !ok || tr != 15 {
		t.Fatal("51")
	}
	if _, ok := NoteTrack(35); ok {
		t.Fatal("35")
	}
	if _, ok := NoteTrack(52); ok {
		t.Fatal("52")
	}
}

func TestTrackParam(t *testing.T) {
	cases := []struct{ ch, cc, base, tr, p int; ok bool }{
		{0, 16, 0, 0, 0, true},
		{0, 39, 0, 0, 23, true},
		{0, 40, 0, 1, 0, true},
		{0, 111, 0, 3, 23, true},
		{0, 112, 0, 0, 0, false},
		{3, 16, 0, 12, 0, true},
		{4, 16, 0, 0, 0, false},
		{2, 40, 2, 1, 0, true},
		{1, 16, 2, 0, 0, false},
	}
	for _, c := range cases {
		tr, p, ok := TrackParam(c.ch, c.cc, c.base)
		if ok != c.ok || (ok && (tr != c.tr || p != c.p)) {
			t.Errorf("%+v -> %d %d %v", c, tr, p, ok)
		}
	}
}
