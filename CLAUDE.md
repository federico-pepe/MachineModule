# CLAUDE.md

Guidance for Claude Code in this repository.

## What this is

MachineModule: a [push-hack](https://github.com/federico-pepe/ableton-push-hack) module for Ableton Push 3 that recreates a Machinedrum-style drum machine. A Go host renders audio (pure-Go engine) into push-audio-loopback's virtual card, draws a front panel on Push's screen, and serves a web page. Not affiliated with Elektron.

## Layout (`src/`)

| Package | Role | Needs hardware? |
|---|---|---|
| `engine/` | DSP and sequencer. Machines, per-track FX, global FX, patterns, kits, songs, player (96 PPQN), project JSON. One mutex guards `Engine`; `Render` holds it for one block. No allocations in `Render`. | no |
| `ui/` | Front panel: pads/buttons/encoders in, 960x160 frame and LED map out. Talks to the host through the `Host` interface. | no |
| `store/` | config, project.json, waves as WAV files, imports folder, autosave | no |
| `midimap/` | what notes/CCs mean | no |
| `hwparams/` | parses Live's negotiated hw_params | no |
| `livelink/` | PushHack Remote Script client (`track.ensure`) | no (Caller is injectable) |
| `cmd/mmrender/` | renders a pattern to WAV | no |
| main package | `main.go` (supervisor re-exec), `audio.go`/`pcm.go`/`bridge.c` (cgo ALSA playback only), `midi.go` (port "MachineModule MIDI In"), `surface.go` (RunSurface, frames, LEDs), `web.go` + `web/index.html`, `app.go` | yes (cgo + libasound) |

## Rules (from the Push Hack framework)

- Never write the display shm directly: frames go through `pmclient.Push` while the app owns the surface; LEDs go through `POST /api/midi/led`.
- On-screen text is ASCII only. Never hardcode ports (`hack.json` "port" is only a default; the catalog allocates).
- Push controls count only while focused (`app.focus`), and only from Push 3 client 16 port 0, channel 0 for CCs.
- The render goroutine is pinned (`LockOSThread`, SCHED_FIFO); keep it allocation-free.
- Open `hw:PHVAudio,1,0` with the params Live negotiated (`hwparams`), S16_LE interleaved, channel pair at `channel_offset`.

## Build and test

- `cd src && go test -race ./...` runs everything except the main package's hardware paths. `go vet ./...` too.
- `make` builds the cgo binary in Docker (golang:1.25-bookworm + libasound2-dev). `go mod tidy` runs there and keeps go.sum current (committed).
- `make test` runs vet and tests in the same container.
- `scripts/deploy.sh` copies the binary to a Push for a manual test (service `push-hack-machinemodule`, log `/data/push-hack/logs/machinemodule.log`).

## Release

Bump `hack.json` "version", tag `v<version>`, push the tag. The workflow checks they match, tests, builds, packages `machinemodule.tar.gz` (top directory `machinemodule/`) and commits `release.json`. The catalog entry is in `catalog-entry.json` (add it to the framework's `catalog/catalog.json`).

## Not verified on hardware

The Push/Live/loopback integration was written from the patterns of push-hack-braids and has only been compile-checked and smoke-tested off device. First things to check on hardware: the audio session opens and sounds; Home focus/blur; LED updates (one HTTP request per change); pad and encoder mapping; Remote Script track creation.
