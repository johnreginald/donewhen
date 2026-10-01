"""Synthesizes the showreel soundtrack (numpy only) from timeline.js, so the sound
lands on the same times as the picture.   python3 audio.py out/audio.wav"""
import json, re, sys, wave
import numpy as np

SR = 44100
T = json.loads(re.sub(r"^window\.T = |;\s*$", "", open("timeline.js").read().strip()))
DUR = T["dur"]; N = int(DUR * SR); C = T["cuts"]
rng = np.random.default_rng(11)
dry = np.zeros((N, 2)); wet = np.zeros((N, 2))

def mtof(m): return 440.0 * 2 ** ((m - 69) / 12)
def put(bus, t0, x, pan=0.0, gain=1.0):
    i = int(t0 * SR)
    if i >= N or i + len(x) <= 0: return
    j = min(N, i + len(x)); x = x[: j - i] * gain
    l, r = np.cos((pan + 1) * np.pi / 4), np.sin((pan + 1) * np.pi / 4)
    bus[i:j, 0] += x * l * 1.4142; bus[i:j, 1] += x * r * 1.4142
def tt(d): return np.arange(int(d * SR)) / SR
def adsr(n, a, d, s, r):
    e = np.ones(n); a_, d_, r_ = int(a * SR), int(d * SR), int(r * SR)
    e[:a_] = np.linspace(0, 1, max(a_, 1)); e[a_:a_ + d_] = np.linspace(1, s, max(d_, 1)); e[a_ + d_: n - r_] = s
    e[n - r_:] = np.linspace(s, 0, max(r_, 1)); return e[:n]
def lowpass(x, fc):
    a = 1 - np.exp(-2 * np.pi * fc / SR); y = np.zeros_like(x); acc = 0.0
    for k in range(len(x)): acc += a * (x[k] - acc); y[k] = acc
    return y
def noise_sweep(d, f0, f1):
    n = rng.standard_normal(int(d * SR)); out = np.zeros_like(n); lo = hi = 0.0; fc = np.geomspace(f0, f1, len(n))
    for k in range(len(n)):
        a = 1 - np.exp(-2 * np.pi * fc[k] / SR); lo += a * (n[k] - lo); hi += a * (lo - hi); out[k] = lo - hi
    return out / (np.abs(out).max() + 1e-9)
def bell(f, d=1.6, bright=1.8):
    t = tt(d); e = np.exp(-t * 2.8)
    return (np.sin(2 * np.pi * f * t + bright * e * np.sin(2 * np.pi * f * 3.5 * t)) * 0.7 + np.sin(2 * np.pi * f * 2 * t) * 0.15 * np.exp(-t * 6)) * e
def pluck(f, d=0.8):
    t = tt(d); e = np.exp(-t * 6)
    return (np.sin(2 * np.pi * f * t + 1.3 * e * np.sin(2 * np.pi * f * t)) + 0.3 * np.sin(2 * np.pi * f * 2 * t) * np.exp(-t * 12)) * e
def thud(f0=95, d=0.6, grit=0.1):
    t = tt(d); f = f0 * np.exp(-t * 9) + 32
    return np.sin(2 * np.pi * np.cumsum(f) / SR) * np.exp(-t * 6) + grit * rng.standard_normal(len(t)) * np.exp(-t * 60)
def click(f=2600, d=0.03):
    t = tt(d); return np.sin(2 * np.pi * f * t) * np.exp(-t * 180) * 0.8 + rng.standard_normal(len(t)) * np.exp(-t * 220) * 0.4
def blip(f, d=0.2):
    t = tt(d); fr = f * (1 + 0.5 * np.exp(-t * 40)); return np.sin(2 * np.pi * np.cumsum(fr) / SR) * np.exp(-t * 18)
def whoosh(d, up=True, f=(300, 7000)):
    x = noise_sweep(d, *(f if up else f[::-1])); t = np.linspace(0, 1, len(x)); return x * (np.sin(np.pi * t) ** 1.5)
def buzz(f=110, d=0.16):
    t = tt(d); x = np.sign(np.sin(2 * np.pi * f * t)) * 0.5 + np.sin(2 * np.pi * f * 1.5 * t) * 0.3
    return lowpass(x, 1800) * adsr(len(t), .005, .02, .8, .05)
def pad(notes, d, gain=0.05):
    t = tt(d); out = np.zeros(len(t))
    for m in notes:
        for det in (-0.07, 0.0, 0.07):
            f = mtof(m) * 2 ** (det / 12)
            out += np.sin(2 * np.pi * f * t + rng.uniform(0, 6.28)) + 0.3 * np.sin(2 * np.pi * f * 2 * t) + 0.1 * np.sin(2 * np.pi * f * 3 * t)
    return out * (1 + 0.12 * np.sin(2 * np.pi * 0.17 * t)) * gain / len(notes)

# ---- bed: a chord per scene, slow swells over each wipe ----
CH = [[45, 52, 57, 60], [48, 55, 60, 64, 67], [41, 48, 55, 59, 64], [43, 50, 55, 59, 62], [45, 52, 57, 64], [48, 55, 60, 64, 67, 71]]
for i, ch in enumerate(CH):
    a = max(0.0, C[i] - 0.5); b = min(DUR, C[i + 1] + 0.7); x = pad(ch, b - a, gain=0.08 if i < 5 else 0.11)
    x *= adsr(len(x), 0.6 if i else 1.2, 0.1, 1.0, 0.8)
    if i == 0: x = lowpass(x, 1800)
    put(dry, a, x, 0.0)
# soft pulse in the hook (every 0.75 s), a gentle arpeggio while the AI works and during the gate
for k in range(int(6.5 / 0.75)): put(dry, k * 0.75, thud(60, 0.4, 0.0), 0, 0.2 * min(1, k / 3))
arp = [57, 64, 69, 72, 69, 64, 60, 67]
for k in range(int((C[2] - T["termIn"]) / 0.5)): put(wet, T["termIn"] + k * 0.5, pluck(mtof(arp[k % 8] + 12), 0.5), (k % 2) * 0.6 - 0.3, 0.06)
for k in range(int((C[4] - T["inboxIn"]) / 0.5)): put(wet, T["inboxIn"] + k * 0.5, pluck(mtof([53, 60, 65, 69][k % 4] + 12), 0.5), (k % 2) * 0.6 - 0.3, 0.05)

# ---- 1 hook ----
for i, w in enumerate(T["titleWords"]): put(dry, w, thud(105 - i * 10, 0.55), 0, 0.5); put(wet, w, click(1700 + i * 400), 0, 0.45)
pent = [69, 72, 74, 76, 79, 81]
for i, b in enumerate(T["bubbles"]): put(wet, b, blip(mtof(pent[(i * 5) % 6] + 12 * (i % 2)), 0.18), -0.5 + (i % 5) * 0.25, 0.2)
put(wet, T["askQ"] - 0.8, whoosh(0.8, True, (200, 5000)), 0, 0.45)
put(dry, T["askQ"], thud(70, 1.1, 0.25), 0, 0.8); put(wet, T["askQ"] + 0.35, bell(mtof(57), 2.2, 1.1), 0, 0.4)
# ---- wipes ----
for i in range(1, 6): put(wet, C[i] - 0.42, whoosh(0.8, True, (250, 6500)), 0.0, 0.45)

# ---- 2 track ----
for i in range(3): put(wet, T["h2"] + i * 0.18, pluck(mtof([60, 64, 67][i] + 12), 0.8), 0, 0.26)
put(wet, T["cardIn"] - 0.2, whoosh(0.7, True, (200, 3500)), 0, 0.3)
for i, r in enumerate(T["rows"]): put(wet, r, pluck(mtof([72, 76, 79][i]), 0.6), -0.2 + i * 0.2, 0.3)
put(wet, T["note"], bell(mtof(84), 1.0), 0.4, 0.18)
for tm in T["term"]:
    for k in range(5): put(wet, tm + 0.06 * k, click(2200 + 150 * k, 0.02), 0.5, 0.14)
for i, tk in enumerate(T["tick"]): put(wet, tk, bell(mtof(79 + 4 * i), 1.5), 0.1, 0.5); put(dry, tk, click(1500, 0.04), 0, 0.3)

# ---- 3 understand ----
for i in range(5): put(wet, T["h3"] + i * 0.14, pluck(mtof([60, 64, 67, 72, 76][i]), 0.7), 0, 0.22)
put(wet, T["docIn"] - 0.2, whoosh(0.7, True, (200, 3500)), 0, 0.3)
for i, d in enumerate(T["diagram"]): put(wet, d, blip(mtof(74 + i * 2), 0.16), -0.4 + i * 0.2, 0.2)
for i, a in enumerate(T["activity"]): put(wet, a, click(2400 + i * 200, 0.025), 0.5, 0.18)
for i, c in enumerate(T["chips"]): put(wet, c, bell(mtof([81, 84, 88][i]), 1.4, 1.2), -0.3 + i * 0.3, 0.3)

# ---- 4 gate ----
for i in range(5): put(wet, T["h4"] + i * 0.14, pluck(mtof([57, 60, 64, 69, 72][i]), 0.7), 0, 0.2)
put(wet, T["hit"] - 0.7, whoosh(0.7, True, (400, 4000)), 0.3, 0.4)
put(dry, T["hit"], buzz(110, 0.16), 0, 0.9); put(dry, T["hit"] + 0.2, buzz(104, 0.16), 0, 0.8); put(dry, T["hit"], thud(80, 0.55), 0, 0.7)
put(wet, T["tick3"], bell(mtof(83), 1.5), 0, 0.55); put(dry, T["tick3"], click(1500, 0.04), 0, 0.3)
for i, m in enumerate([72, 76, 79, 84]): put(wet, T["open"] + i * 0.08, bell(mtof(m), 2.4, 1.3), -0.3 + i * 0.2, 0.42)
put(wet, T["split"] - 0.1, whoosh(0.8, False, (6000, 300)), 0, 0.3)
put(wet, T["inboxIn"] - 0.2, whoosh(0.7, True, (200, 3000)), 0, 0.28)
put(dry, T["key"], thud(140, 0.14, 0.0), 0, 0.45); put(wet, T["key"], click(1400, 0.035), 0, 0.7)
for i, m in enumerate([65, 69, 72, 77, 81]): put(wet, T["approve"] + i * 0.06, bell(mtof(m), 2.6, 1.2), -0.4 + i * 0.2, 0.4)
put(dry, T["approve"], thud(100, 0.6, 0.0), 0, 0.6); put(wet, T["count"], blip(mtof(86), 0.25), 0.3, 0.4)

# ---- 5 check again ----
for i in range(4): put(wet, T["h5"] + i * 0.16, pluck(mtof([62, 65, 69, 74][i]), 0.8), 0, 0.22)
for k in range(int((T["scrubEnd"] - T["scrubGo"]) / 0.25)): put(wet, T["scrubGo"] + k * 0.25, click(2800 - k * 40, 0.02), 0.4, 0.12)
put(wet, T["scrubEnd"], bell(mtof(69), 1.6, 1.0), 0, 0.3)
put(dry, T["untick"], click(900, 0.05), 0, 0.5); put(wet, T["untick"], blip(mtof(60), 0.3), 0, 0.4)
put(dry, T["badge"], buzz(150, 0.12), 0, 0.5); put(wet, T["badge"], bell(mtof(64), 1.2, 0.8), 0, 0.25)

# ---- 6 logo ----
put(wet, T["logo"] - 0.6, whoosh(0.9, True, (150, 7000)), 0, 0.5)
put(dry, T["logo"], thud(62, 1.3, 0.15), 0, 0.9); put(wet, T["logo"], bell(mtof(60), 3.0, 1.0), 0, 0.4)
for i, tr in enumerate(T["trail"]): put(wet, tr, blip(mtof([72, 76, 79][i]), 0.25), -0.3 + i * 0.3, 0.3)
put(wet, T["dDraw"], whoosh(1.0, True, (500, 4000)), 0, 0.28)
put(wet, T["pass"], whoosh(0.9, True, (400, 6000)), 0, 0.3)
for i, m in enumerate([72, 76, 79, 83, 86]): put(wet, T["pass"] + 1.1 + i * 0.07, bell(mtof(m), 3.4, 1.3), -0.5 + i * 0.25, 0.42)
put(dry, T["pass"] + 1.1, thud(70, 1.0, 0.1), 0, 0.5)
for i in range(8): put(wet, T["word"] + i * 0.06, click(2600 + i * 120, 0.02), 0, 0.13)
for i, tg in enumerate(T["tag"]): put(wet, tg, pluck(mtof([76, 79, 84][i]), 1.3), -0.2 + i * 0.2, 0.28)

# ---- reverb on the wet bus ----
n_ir = int(2.4 * SR); t_ir = np.arange(n_ir) / SR
ir = rng.standard_normal((n_ir, 2)) * np.exp(-t_ir * 2.3)[:, None]
for c in range(2): ir[:, c] = np.convolve(ir[:, c], np.ones(4) / 4, "same")
ir /= np.abs(ir).sum(axis=0).max()
F = 1 << (N + n_ir - 1).bit_length()
rev = np.stack([np.fft.irfft(np.fft.rfft(wet[:, c], F) * np.fft.rfft(ir[:, c], F), F)[:N] for c in range(2)], 1)
mix = dry + wet * 0.8 + rev * 3.2
mix = np.tanh(mix * 0.9) / np.tanh(0.9); mix *= 0.86 / max(np.abs(mix).max(), 1e-9)
mix[: int(0.15 * SR)] *= np.linspace(0, 1, int(0.15 * SR))[:, None]
mix[-int(1.2 * SR):] *= np.linspace(1, 0, int(1.2 * SR))[:, None]
out = sys.argv[1] if len(sys.argv) > 1 else "out/audio.wav"
with wave.open(out, "wb") as w:
    w.setnchannels(2); w.setsampwidth(2); w.setframerate(SR); w.writeframes((mix * 32767).astype("<i2").tobytes())
print("wrote", out, f"{DUR}s")
