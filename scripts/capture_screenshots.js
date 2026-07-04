#!/usr/bin/env node
/** Capture evidence screenshots of the Òfin web UI using headless Chrome. */

const puppeteer = require('puppeteer-core');
const path = require('path');
const fs = require('fs');

const OUT = path.join(__dirname, '..', 'docs', 'screenshots');
const BASE = 'http://127.0.0.1:8090';

async function sleep(ms) { return new Promise(r => setTimeout(r, ms)); }

async function askQuestion(page, question, timeoutMs = 120000) {
  // Clear and type
  const ta = await page.$('#q');
  await ta.click({ clickCount: 3 }); // select all
  await ta.type(question);
  // Click send
  await page.click('#send');
  // Wait for answer or computation to appear
  try {
    await page.waitForFunction(() => {
      return document.querySelectorAll('.answer.show').length > 0 ||
             document.querySelectorAll('.comp.show').length > 0;
    }, { timeout: timeoutMs });
  } catch (e) {
    console.log(`    (timeout, continuing...)`);
  }
  // Extra settle time for pipeline animation
  await sleep(4000);
}

async function main() {
  fs.mkdirSync(OUT, { recursive: true });

  const browser = await puppeteer.launch({
    executablePath: '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
    headless: 'new',
    args: ['--no-sandbox', '--disable-setuid-sandbox'],
  });

  const page = await browser.newPage();
  await page.setViewport({ width: 1280, height: 900 });

  // ── 1. Fresh UI ───────────────────────────────────────────────
  console.log('1. Fresh UI...');
  await page.goto(BASE, { waitUntil: 'networkidle0' });
  await sleep(1500);
  await page.screenshot({ path: path.join(OUT, '01-fresh-ui.png') });
  console.log('   ✓ 01-fresh-ui.png');

  // ── 2. English lookup — notice period ──────────────────────────
  console.log('2. English lookup (3 years → two weeks notice)...');
  await askQuestion(page, 'How much notice should my employer give me after 3 years of service?');
  // Expand a source chip to show statutory text
  try {
    await page.click('.chip');
    await sleep(500);
  } catch(e) {}
  await page.screenshot({ path: path.join(OUT, '02-english-lookup.png') });
  console.log('   ✓ 02-english-lookup.png');

  // New chat
  await page.click('#new-btn');
  await sleep(800);

  // ── 3. Computation — PAYE ──────────────────────────────────────
  console.log('3. Computation (PAYE on 450k)...');
  await askQuestion(page, 'I earn 450,000 naira monthly. How much PAYE tax should be deducted?');
  await page.screenshot({ path: path.join(OUT, '03-computation.png') });
  console.log('   ✓ 03-computation.png');

  // ── 4. Pidgin answer ───────────────────────────────────────────
  console.log('4. Pidgin answer...');
  // Toggle Pidgin on
  await page.click('#pidgin-btn');
  await sleep(400);
  await page.click('#new-btn');
  await sleep(600);
  await askQuestion(page, 'My oga sack me without notice, I don work there for 4 years. Wetin I fit do?');
  await page.screenshot({ path: path.join(OUT, '04-pidgin-answer.png') });
  console.log('   ✓ 04-pidgin-answer.png');

  // ── 5. Refusal ─────────────────────────────────────────────────
  console.log('5. Refusal...');
  await page.click('#new-btn');
  await sleep(600);
  await askQuestion(page, 'How do I divorce my husband under Nigerian law?');
  await page.screenshot({ path: path.join(OUT, '05-refusal.png') });
  console.log('   ✓ 05-refusal.png');

  // Toggle Pidgin off for dark mode shot
  await page.click('#pidgin-btn');
  await sleep(300);

  // ── 6. Dark mode ───────────────────────────────────────────────
  console.log('6. Dark mode...');
  await page.click('#theme-btn');
  await sleep(800);
  await page.screenshot({ path: path.join(OUT, '06-dark-mode.png') });
  console.log('   ✓ 06-dark-mode.png');

  await browser.close();
  console.log('\nDone! Screenshots saved to docs/screenshots/');
  console.log('Files:');
  fs.readdirSync(OUT).forEach(f => console.log(`  ${f}`));
}

main().catch(e => { console.error(e); process.exit(1); });
