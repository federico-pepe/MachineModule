# Changelog

## Unreleased

- Plays through the audio mixer of push-audio-loopback (audio bus) when it runs, so other audio hacks can play at the same time. Without the mixer it writes to the card directly, as before. Prototype: needs a local `core/` with `audiobus` to build.

- Web page: dropdowns for the audio output device and channel pair.
- Create tracks in Live no longer makes a MIDI track.
- The audio session reopens when the device disappears (it spun at full CPU before).
- The web page stops building state when no browser is connected.

## 0.1.0-alpha

First version. 33 machines, 64 UserWave slots, 16-track sequencer with parameter locks, kits, songs, global effects, Push 3 front panel, web panel, MIDI input, project autosave.
