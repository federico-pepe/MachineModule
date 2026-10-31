package livelink

import (
	"encoding/json"
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
	if p, ok := OnlyPair([]string{"1", "2", "5/6"}); !ok || p != "5/6" {
		t.Fatal("only")
	}
	if _, ok := OnlyPair([]string{"1/2", "3/4"}); ok {
		t.Fatal("two pairs")
	}
}

func TestEnsureSwitchesToOnlyPair(t *testing.T) {
	calls := 0
	call := func(ns, cmd string, args map[string]any) (json.RawMessage, error) {
		if cmd == "ensure" && args["kind"] == "audio" {
			calls++
			if calls == 1 {
				return nil, &Error{Code: "bad_routing", Data: json.RawMessage(`{"what":"channel","options":["1","2","5/6"]}`)}
			}
			if ch := args["input_channel"].([]string)[0]; ch != "5/6" {
				t.Fatalf("retry on %s", ch)
			}
		}
		return json.RawMessage(`{"track":1,"created":true}`), nil
	}
	got := -1
	res, err := EnsureTracks(call, 0, func(o int) error { got = o; return nil })
	if err != nil || got != 4 || len(res.Lines) < 3 {
		t.Fatalf("%v %d %v", err, got, res.Lines)
	}
}

func TestEnsureReportsMissingPair(t *testing.T) {
	call := func(ns, cmd string, args map[string]any) (json.RawMessage, error) {
		return nil, &Error{Code: "bad_routing", Data: json.RawMessage(`{"what":"channel","options":["1/2","3/4"]}`)}
	}
	if _, err := EnsureTracks(call, 6, func(int) error { return nil }); err == nil {
		t.Fatal("expected error")
	}
}
