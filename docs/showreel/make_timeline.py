"""Writes timeline.js: the one list of times that both the picture (showreel.html)
and the sound (audio.py) read, so they cannot drift apart."""
import json

T = {
    "dur": 48, "fps": 30,
    "cuts": [0, 7, 16, 24, 34, 40, 48],                  # six scenes
    # 1  hook
    "titleWords": [0.4, 1.2, 2.0], "shrink": 3.3,
    "bubbles": [round(3.6 + 1.7 * (i / 11) ** 1.2, 3) for i in range(12)],
    "bubbleStart": 3.6, "askQ": 5.3,
    # 2  track
    "h2": 7.7, "cardIn": 8.5, "note": 9.4, "rows": [10.1, 10.7, 11.3], "termIn": 12.0, "pillProg": 12.5,
    "term": [12.6, 13.6, 14.6, 15.4], "tick": [13.8, 15.6],
    # 3  understand
    "h3": 16.7, "docIn": 17.5, "diagram": [18.3, 19.1, 19.9, 20.7, 21.5],
    "activity": [18.1, 19.0, 19.9, 20.8, 21.7], "chips": [18.9, 20.1, 21.3],
    # 4  gate
    "h4": 24.5, "cardIn4": 24.7, "chipAppr": 25.4, "hit": 26.5, "notYet": 25.9,
    "tick3": 28.8, "open": 29.3, "pillReview": 29.7, "split": 30.1, "inboxIn": 30.8,
    "key": 32.5, "approve": 32.65, "pillDone": 33.0, "count": 33.1,
    # 5  check again
    "h5": 34.7, "scrubIn": 35.0, "scrubGo": 35.5, "scrubEnd": 37.4, "untick": 37.7, "badge": 38.1,
    # 6  logo
    "logo": 40.3, "trail": [40.7, 41.1, 41.5], "dDraw": 41.7, "pass": 42.8, "word": 44.3, "tag": [45.3, 46.0, 46.7],
}
open("timeline.js", "w").write("window.T = " + json.dumps(T, indent=1) + ";\n")
print("ok")
