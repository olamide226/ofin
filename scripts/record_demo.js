#!/usr/bin/env node
/**
 * Demo video script — launches Chrome (visible) and walks through the key
 * Òfin interactions. Run screencapture in parallel to record the window.
 *
 * Usage: node scripts/record_demo.js
 * The recording is handled separately — see scripts/demo_video.sh
 */

const puppeteer = require('puppeteer-core');

const BASE = 'http://127.0.0.1:8090';

async function sleep(ms) { return new Promise(r => setTimeout(r, ms)); }

async function typeQuestion(page, question) {
  const ta = await page.$('#q');
  await ta.click({ clickCount: 3 });
  await ta.type(question, { delay: 30 }); // human-like typing
  await sleep(300);
  await page.click('#send');
}

async function waitForDone(page, timeoutMs = 120000) {
  try {
    await page.waitForFunction(() => {
      return document.querySelectorAll('.answer.show').length > 0 ||
             document.querySelectorAll('.comp.show').length > 0;
    }, { timeout: timeoutMs });
  } catch (e) { console.log('  (timeout)'); }
  await sleep(2000);
}

async function main() {
  const browser = await puppeteer.launch({
    executablePath: '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
    headless: false, // VISIBLE — for the recording
    args: [
      '--window-size=1280,800',
      '--window-position=200,100',
      '--no-sandbox',
    ],
  });

  const page = await browser.newPage();
  await page.setViewport({ width: 1280, height: 800 });

  // 1. INTRO — fresh UI with sidebar
  console.log('Scene 1: Fresh UI');
  await page.goto(BASE, { waitUntil: 'networkidle0' });
  await sleep(2000);

  // 2. ENGLISH LOOKUP — 3 years notice
  console.log('Scene 2: English lookup');
  await typeQuestion(page, 'How much notice should my employer give me after 3 years of service?');
  await waitForDone(page);
  // Expand a source chip
  try { await page.click('.chip'); await sleep(800); } catch(e) {}
  await sleep(2000);

  // 3. COMPUTATION — PAYE
  console.log('Scene 3: PAYE computation');
  await page.click('#new-btn');
  await sleep(800);
  await typeQuestion(page, 'I earn 450,000 naira monthly. How much PAYE tax should I pay?');
  await waitForDone(page);
  await sleep(3000);

  // 4. PIDGIN — toggle on, ask
  console.log('Scene 4: Pidgin toggle + question');
  await page.click('#pidgin-btn');
  await sleep(500);
  await page.click('#new-btn');
  await sleep(800);
  await typeQuestion(page, 'My oga sack me without notice, I don work there for 4 years. Wetin I fit do?');
  await waitForDone(page);
  await sleep(3000);

  // 5. REFUSAL — out of scope
  console.log('Scene 5: Refusal');
  await page.click('#new-btn');
  await sleep(800);
  await typeQuestion(page, 'How do I divorce my husband under Nigerian law?');
  await waitForDone(page);
  await sleep(2000);

  // 6. DARK MODE
  console.log('Scene 6: Dark mode');
  await page.click('#theme-btn');
  await sleep(1500);

  // Hold for a moment so the recording captures dark mode
  await sleep(2000);

  console.log('Demo complete — stopping Chrome');
  await browser.close();
}

main().catch(e => { console.error(e); process.exit(1); });
