// Renders showreel.html frame by frame with headless Chrome.
//   node render.mjs stills out/stills 0.4 1.2 3.0      (single frames, for review)
//   node render.mjs video  out/frames [workers]        (all frames, 1920x1080 @ 30fps)
//   VERTICAL=1 node render.mjs ...                     (9:16, 1080x1920)
import { createRequire } from 'node:module';
import { mkdirSync, readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';
import path from 'node:path';

const require = createRequire(process.env.PUPPETEER_FROM || '/tmp/shots/');
const puppeteer = require('puppeteer-core');
const CHROME = process.env.CHROME || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
const here = path.dirname(new URL(import.meta.url).pathname);
const VERT = process.env.VERTICAL === '1';
const url = pathToFileURL(path.join(here, 'showreel.html')).href + (VERT ? '?v=1' : '');
const VW = VERT ? 1080 : 1920, VH = VERT ? 1920 : 1080;
const T = JSON.parse(readFileSync(path.join(here, 'timeline.js'), 'utf8').replace(/^window\.T = /, '').replace(/;\s*$/, ''));

const [mode, outDir, ...rest] = process.argv.slice(2);
mkdirSync(outDir, { recursive: true });

async function open(browser) {
  const page = await browser.newPage();
  await page.setViewport({ width: VW, height: VH, deviceScaleFactor: 1 });
  await page.goto(url, { waitUntil: 'networkidle0' });
  await page.evaluate(() => window.ready);
  return page;
}
const browser = await puppeteer.launch({ executablePath: CHROME, headless: 'new', args: ['--hide-scrollbars', '--force-color-profile=srgb'] });

if (mode === 'stills') {
  const page = await open(browser);
  for (const s of rest) {
    await page.evaluate(t => window.render(t), parseFloat(s));
    await page.screenshot({ path: path.join(outDir, `t${parseFloat(s).toFixed(2).padStart(5, '0')}.png`) });
  }
} else {
  const workers = parseInt(rest[0] || '4', 10);
  const total = Math.round(T.dur * T.fps);
  let next = 0, done = 0; const t0 = Date.now();
  await Promise.all(Array.from({ length: workers }, async () => {
    const page = await open(browser);
    for (;;) {
      const i = next++; if (i >= total) break;
      await page.evaluate(t => window.render(t), i / T.fps);
      await page.screenshot({ path: path.join(outDir, `f${String(i).padStart(4, '0')}.png`) });
      if (++done % 120 === 0) console.log(`${done}/${total}  ${((Date.now() - t0) / 1000).toFixed(0)}s`);
    }
  }));
}
await browser.close();
