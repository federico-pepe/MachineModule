# MachineModule

A [Push Hack](https://github.com/federico-pepe/ableton-push-hack) module for Ableton Push 3: a drum machine in the style of the Elektron Machinedrum SPS-1UW MKII. It runs on the Push, draws on Push's screen, plays from its pads and buttons, and sends its audio into Live through the Push Hack virtual audio card.

Not affiliated with or endorsed by Elektron. The sound engine is original; only the model (names, track layout, parameter pages, sequencer features) follows the Machinedrum.

## What it does

- **16 tracks**, each with a "machine" chosen from 33 synth models: TRX (kick, snare, clap, rim, cowbell, hats, cymbal, maracas, claves), EFM (FM drums), PI (physical-model toms and percussion), GND (sine, noise, square) and UW (a UserWave sample player).
- **UserWave samples**: 64 slots, loaded from WAV files (Push SYSTEM page or the web page). Start, end, loop, reverse, pitch and decay per hit.
- **Sound shaping**: 8 synth parameters per machine, an effects page (amplitude modulation, EQ, filter, sample-rate reduction), a routing page (delay and reverb sends, pan, volume) and one LFO per track that can modulate any parameter.
- **Global effects** in every kit: delay, reverb, compressor, master EQ, master volume.
- **Sequencer**: 128 patterns (banks A-H, 16 each), 2-64 steps, pattern scale (1/8x to 2x), swing, accent, slide, **parameter locks** per step, real-time recording, copy/paste/clear, tap tempo, internal clock or MIDI clock.
- **Kits** (64) and **songs** (16 songs of pattern rows with repeats and a loop point).
- **Project saving**: patterns, kits, songs, tempo and samples are saved automatically.

See [MANUAL.md](MANUAL.md) for the controls.

## Requirements

- Push 3 with Push Hack, including **push-manager** and **push-display** (for the Push screen).
- **push-audio-loopback** (the virtual audio card). The catalog installs it for you.
- The **PushHack Remote Script** in Live, only for the "Create tracks in Live" button.

## Install and first run

1. Install **MachineModule** from the Push Hack catalog (or `make`, then `scripts/deploy.sh`).
2. In Live: Push audio preferences, Devices, enable an input pair of the *Push Hack Virtual Audio* card.
3. Open MachineModule's web page from the Push Hack menu, press **Create tracks in Live** (this makes an audio track that listens to the card; the sequencer is inside MachineModule, so no MIDI track is needed).
   Without the Remote Script: make an audio track with that input and Monitor *In*.
   The web page's Setup section has dropdowns for the audio output device and the channel pair.
4. On Push, open **Push Hack Home** (Shift + Help) and choose MachineModule.

## Build

```
make          # builds ./machinemodule (needs Docker: cgo + libasound for linux/amd64)
make test     # go vet + go test -race inside the same container
```

The engine, sequencer, front panel, storage and MIDI mapping are plain Go and are covered by tests that run anywhere:

```
cd src && go test -race ./...
```

`src/cmd/mmrender` renders a pattern to a WAV file with no hardware, which is handy for sound design.

## Status

Alpha. The audio engine, sequencer and front panel are tested on a development machine; the Push, Live and loopback integration (`src/*.go` in the main package) follows the patterns of the other Push Hack audio modules but has **not yet been run on real hardware**. Expect rough edges and please open issues.

## Not (yet) implemented

RAM-R / RAM-P live sampling, the CTR, INP and MID machine families, the ROM bank, per-track multi-output routing, per-track pattern length and scale ("multi" mode), sysex dump/load of the original hardware.
