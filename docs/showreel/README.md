# Showreel

The 48-second motion piece at the top of the main README. It tells the product story in six scenes:
the hook, track, understand, gate, check again later, and the logo.

Every frame is a pure function of time, so the picture and the sound are rebuilt from the same files
and always match.

| File | What it does |
| --- | --- |
| `make_timeline.py` | Writes `timeline.js`: the one list of event times for picture and sound |
| `showreel.html` | All six scenes, drawn with HTML, CSS and SVG from `render(t)` |
| `render.mjs` | Renders `render(t)` to PNG frames with headless Chrome (needs `puppeteer-core`) |
| `audio.py` | Synthesizes the soundtrack from `timeline.js` (needs `numpy`) |
| `donewhen-showreel.mp4` | The finished video, 1080p, with sound |

Rebuild it:

```bash
python3 make_timeline.py
node render.mjs video out/frames 4          # 1440 frames, 1920x1080, 30 fps
python3 audio.py out/audio.wav
ffmpeg -framerate 30 -i out/frames/f%04d.png -i out/audio.wav \
  -c:v libx264 -preset slow -crf 17 -pix_fmt yuv420p -c:a aac -b:a 192k -shortest \
  -movflags +faststart donewhen-showreel.mp4
```

`node render.mjs stills out/stills 8.2 26.5` renders single frames for review. Set
`PUPPETEER_FROM=/path/to/` if `puppeteer-core` is not installed next to this folder.
Fonts (Newsreader, IBM Plex) load from Google Fonts, so the first render needs network access.
