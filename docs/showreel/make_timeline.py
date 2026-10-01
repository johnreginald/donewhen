"""Writes timeline.js: the one list of times that both the picture (showreel.html)
and the sound (audio.py) read, so they cannot drift apart."""
import json

T = {
    "dur": 39,
    "fps": 30,
    "cuts": [
        0,
        5.0,
        12.0,
        19.0,
        27.0,
        32.0,
        39.0
    ],
    "titleStart": 0.2,
    "titleGap": 0.045,
    "shrink": 1.9,
    "bubbleStart": 2.0,
    "bubbles": [round(2.0 + 1.5 * (i / 13) ** 1.2, 3) for i in range(14)],
    "askQ": 3.4,
    "h2": 5.4,
    "cardIn": 5.8,
    "parts": [
        6.1,
        6.3,
        6.5,
        6.7
    ],
    "rows": [
        7.0,
        7.3,
        7.6
    ],
    "orbit": 6.9,
    "termIn": 8.2,
    "pillProg": 8.6,
    "term": [
        8.7,
        9.4,
        10.1,
        10.7
    ],
    "tick": [
        9.6,
        11.0
    ],
    "h3": 12.4,
    "docIn": 12.8,
    "diagram": [
        13.4,
        13.9,
        14.4,
        14.9,
        15.4
    ],
    "activity": [
        13.3,
        13.9,
        14.5,
        15.1,
        15.7
    ],
    "tags": [
        13.8,
        15.2,
        16.6
    ],
    "h4": 19.4,
    "cardIn4": 19.6,
    "gateIn": 20.1,
    "chipAppr": 20.3,
    "hit": 21.4,
    "notYet": 20.7,
    "tick3": 22.9,
    "open": 23.3,
    "passChip": 23.5,
    "split": 24.1,
    "pillReview": 24.2,
    "inboxIn": 24.5,
    "key": 25.9,
    "approve": 26.05,
    "pillDone": 26.4,
    "count": 26.5,
    "h5": 27.4,
    "scrubIn": 27.6,
    "scrubGo": 28.0,
    "scrubEnd": 29.9,
    "untick": 30.2,
    "badge": 30.6,
    "logo": 32.2,
    "trail": [
        32.6,
        32.9,
        33.2
    ],
    "dDraw": 33.4,
    "pass": 34.3,
    "bloom": 35.2,
    "word": 35.5,
    "tag": [
        36.2,
        36.7,
        37.2
    ],
    "url": 37.5
}
open("timeline.js", "w").write("window.T = " + json.dumps(T, indent=1) + ";\n")
print("ok")
