package livelink

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPair(t *testing.T) {
	if PairName(2) != "3/4" {
		t.Fatal(PairName(2))
	}
	if o, ok := ParsePair("3/4"); !ok || o != 2 {
		t.Fatal("3/4")
	}
	for _, bad := range []string{"3", "3/5", "0/1", "a/b"} {
		if _, ok := ParsePair(bad); ok {
			t.Fatal(bad)
		}
	}
}

func TestEnsureKeepsChosenPair(t *testing.T) {
	var asked []string
	call := func(ns, cmd string, args map[string]any) (json.RawMessage, error) {
		asked = append(asked, args["input_channel"].([]string)[0])
		return nil, &Error{Code: "bad_routing", Data: json.RawMessage(`{"what":"channel","options":["1","2","5/6"]}`)}
	}
	_, err := EnsureTracks(call, 6)
	if err == nil || !strings.Contains(err.Error(), "5/6") {
		t.Fatalf("want an error naming the offered pair, got %v", err)
	}
	if len(asked) != 1 || asked[0] != "7/8" {
		t.Fatalf("asked %v; must not retry on another pair", asked)
	}
}

func TestEnsureMakesOnlyAudioTrack(t *testing.T) {
	var kinds []any
	call := func(ns, cmd string, args map[string]any) (json.RawMessage, error) {
		kinds = append(kinds, args["kind"])
		return json.RawMessage(`{"track":1,"created":true}`), nil
	}
	res, err := EnsureTracks(call, 2)
	if err != nil || len(kinds) != 1 || kinds[0] != "audio" || len(res.Lines) != 1 {
		t.Fatalf("%v %v %v", err, kinds, res.Lines)
	}
}

func TestEnsureReportsMissingPair(t *testing.T) {
	call := func(ns, cmd string, args map[string]any) (json.RawMessage, error) {
		return nil, &Error{Code: "bad_routing", Data: json.RawMessage(`{"what":"channel","options":["1/2","3/4"]}`)}
	}
	if _, err := EnsureTracks(call, 6); err == nil {
		t.Fatal("expected error")
	}
}
