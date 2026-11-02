#!/usr/bin/env python3
"""Merge Machinedrum kit dumps (.syx, message 0x52) into a MachineModule project.

Usage: mdkits2project.py kits.syx project.json out.json

project.json is what GET /api/project returns. Kits are written to the same
slot numbers they have in the dump; patterns, songs and tempo are untouched.
Only kits are read. Tracks whose machine does not exist in MachineModule get
the closest one (see MODELS), so the sound is similar, not identical.
"""
import json
import sys

# Machinedrum model id -> MachineModule machine name. None = silent track.
MODELS = {
    0x00: None,
    0x01: "GND-SIN", 0x02: "GND-NS", 0x03: "GND-SQ",
    # TRX
    0x10: "TRX-BD", 0x11: "TRX-SD", 0x12: "TRX-XT", 0x13: "TRX-CP",
    0x14: "TRX-RS", 0x15: "TRX-CB", 0x16: "TRX-CH", 0x17: "TRX-OH",
    0x18: "TRX-CY", 0x19: "TRX-MA", 0x1A: "TRX-CL", 0x1B: "TRX-CL",
    0x1C: "TRX-B2",
    # EFM
    0x20: "EFM-BD", 0x21: "EFM-SD", 0x22: "EFM-XT", 0x23: "EFM-CP",
    0x24: "EFM-RS", 0x25: "EFM-CB", 0x26: "EFM-CH", 0x27: "EFM-CY",
    # E12 has no counterpart: nearest TRX drum of the same role
    0x30: "TRX-BD", 0x31: "TRX-SD", 0x32: "TRX-XT", 0x33: "TRX-XT",
    0x34: "TRX-CP", 0x35: "TRX-RS", 0x36: "TRX-CB", 0x37: "TRX-CH",
    0x38: "TRX-OH", 0x39: "TRX-CY", 0x3A: "TRX-CY", 0x3B: "TRX-MA",
    0x3C: "TRX-MA", 0x3D: "TRX-CL", 0x3E: "TRX-CL", 0x3F: "TRX-XT",
    # PI
    0x40: "PI-BD", 0x41: "PI-SD", 0x42: "PI-MT", 0x43: "PI-LT",
    0x44: "TRX-MA", 0x45: "PI-RS", 0x46: "PI-RC", 0x47: "PI-CL",
    0x48: "TRX-CH",
}


def messages(data):
    i = 0
    while i < len(data):
        j = data.index(0xF7, i)
        yield data[i:j + 1]
        i = j + 1


def unpack(b):
    """Machinedrum 7-bit packing: one MSB byte, then up to 7 data bytes."""
    out = bytearray()
    for g in range(0, len(b), 8):
        grp = b[g:g + 8]
        for k, x in enumerate(grp[1:]):
            out.append(x | (0x80 if grp[0] & (0x40 >> k) else 0))
    return bytes(out)


def convert_kit(body, names, defaults):
    """body: kit payload after version, revision and slot bytes."""
    raw = bytes(body[:16]).split(b"\0")[0]
    if raw[:2] == b"\x7f\x7f":  # empty slot
        return None
    # some dumps have a 0x7f in place of the first letter, or a stray byte at the end
    raw = (b"_" if raw[:1] == b"\x7f" else raw[:1]) + raw[1:]
    for i, c in enumerate(raw):
        if not 32 <= c < 127:
            raw = raw[:i]
            break
    name = raw.decode("ascii").strip()
    if len(name) < 3:  # leftover bytes of an empty slot
        return None
    models = unpack(body[416:])
    levels = body[400:416]
    kit = {"Name": name[:16], "Track": [], "Global": defaults["Global"]}
    for t in range(16):
        p = list(body[16 + t * 24:16 + (t + 1) * 24])
        mid = int.from_bytes(models[t * 4:t * 4 + 4], "big")
        mname = MODELS.get(mid)
        silent = mid not in MODELS or mname is None
        machine = names.index(mname or "GND-SIN")
        # The dump orders the routing row DEL, VOL, PAN, REV, LFO...; the
        # module orders it DEL, REV, PAN, VOL. The LFO destination is a
        # Machinedrum parameter number, so the LFO is left off.
        delay, vol, pan, rev = p[16], p[17], p[18], p[19]
        vol = vol * levels[t] // 127
        route = [delay, rev, pan, 0 if silent else vol, 0, 64, 0, 0]
        kit["Track"].append({"Machine": machine, "P": p[:16] + route})
    return kit


def main(syx, proj_in, proj_out):
    data = open(syx, "rb").read()
    proj = json.load(open(proj_in))
    base = proj["kits"]["0"]
    # machine names, in menu order, from the factory kit's neighbours are not
    # available in the project file, so they come from the module source order
    names = MACHINES
    n = 0
    for m in messages(data):
        if len(m) < 12 or m[1:6] != b"\x00\x20\x3c\x02\x00" or m[6] != 0x52:
            continue
        slot = m[9]
        if slot >= 64:
            continue
        kit = convert_kit(m[10:-5], names, base)
        if kit is None:
            continue
        proj["kits"][str(slot)] = kit
        n += 1
    json.dump(proj, open(proj_out, "w"), indent=1)
    print(f"{n} kits merged into {proj_out}")


# Order of engine.Machines (index = machine id in the project file).
MACHINES = [
    "TRX-BD", "TRX-B2", "TRX-XT", "TRX-SD", "TRX-CP", "TRX-RS", "TRX-CB",
    "TRX-CH", "TRX-OH", "TRX-CY", "TRX-MA", "TRX-CL",
    "EFM-BD", "EFM-SD", "EFM-XT", "EFM-CP", "EFM-RS", "EFM-CB", "EFM-CH",
    "EFM-CY",
    "PI-BD", "PI-SD", "PI-LT", "PI-MT", "PI-HT", "PI-RS", "PI-RC", "PI-CL",
    "PI-BL",
    "GND-SIN", "GND-NS", "GND-SQ", "UW-SMP",
]

if __name__ == "__main__":
    if len(sys.argv) != 4:
        sys.exit(__doc__)
    main(*sys.argv[1:])
