// Push notification error-handling verification.
//
// Requires system Chrome with persistent context and notification permissions.
// Cannot run under `make e2e` (bundled Chromium lacks Push API support).
//
// Run:
//   cd e2e/manual && npm install playwright && DISPLAY=:0 node push-error-handling.mjs
//
// Verifies 4 error-handling paths in static/js/push.js:
//   1. Subscribe POST returns non-ok → toggle reverts, error toast shown
//   2. Subscribe POST network error  → same
//   3. Unsubscribe POST returns non-ok → warning toast shown
//   4. Unsubscribe POST network error  → same
//
// Approach: window.fetch is mocked in the browser context before push.js
// loads. page.route() cannot intercept fetches from a SW-controlled page,
// even when the SW has no fetch handler — Chrome routes all requests through
// the SW's network context regardless.

import { chromium } from 'playwright';
import { createServer } from 'http';
import { readFileSync } from 'fs';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';
import { mkdtempSync } from 'fs';
import { tmpdir } from 'os';

const __dirname = dirname(fileURLToPath(import.meta.url));
const pushJs = readFileSync(join(__dirname, '..', '..', 'static', 'js', 'push.js'), 'utf8');
const testSw = readFileSync(join(__dirname, 'fixtures', 'test-sw.js'), 'utf8');
const testHtml = readFileSync(join(__dirname, 'fixtures', 'test-push.html'), 'utf8');

const server = createServer((req, res) => {
    if (req.url === '/' || req.url === '/index.html') {
        res.writeHead(200, { 'Content-Type': 'text/html' });
        res.end(testHtml);
    } else if (req.url === '/push.js') {
        res.writeHead(200, { 'Content-Type': 'application/javascript' });
        res.end(pushJs);
    } else if (req.url === '/test-sw.js') {
        res.writeHead(200, { 'Content-Type': 'application/javascript', 'Service-Worker-Allowed': '/' });
        res.end(testSw);
    } else {
        res.writeHead(404);
        res.end('not found');
    }
});

await new Promise(r => server.listen(0, '127.0.0.1', r));
const PORT = server.address().port;
const BASE = `http://127.0.0.1:${PORT}`;
console.log(`Server on ${BASE}`);

let passed = 0;
const total = 4;
function ok(name) { passed++; console.log(`  PASS — ${name}`); }
function fail(name, detail) { console.error(`  FAIL — ${name}: ${detail}`); }

const userDataDir = mkdtempSync(join(tmpdir(), 'pw-push-'));
const context = await chromium.launchPersistentContext(userDataDir, {
    headless: false,
    channel: 'chrome',
    permissions: ['notifications'],
    args: ['--no-sandbox']
});

async function waitForToast(page, timeout = 10000) {
    return page.waitForFunction(() => {
        var el = document.getElementById('push-error-toast');
        return el && el.classList.contains('toast') && el.textContent.length > 0;
    }, {}, { timeout });
}

try {
    // --- Path 1: Subscribe POST returns 500 ---
    console.log('\n--- Path 1: subscribe POST failure (non-ok response) ---');
    {
        const page = await context.newPage();
        await page.goto(BASE);
        await page.waitForTimeout(2000);

        await page.evaluate(() => { window._fetchMode = 'fail-status'; window._fetchLog = []; });
        await page.click('#push-toggle');
        try {
            await waitForToast(page);
            const toastText = await page.evaluate(() => document.getElementById('push-error-toast').textContent);
            const toggleChecked = await page.isChecked('#push-toggle');
            const log = await page.evaluate(() => window._fetchLog);
            if (!toggleChecked && toastText.includes('No se pudieron activar') && log.some(l => l.url.includes('/push/subscribe'))) {
                ok('subscribe-500: toggle reverted + error toast + POST made');
            } else {
                fail('subscribe-500', `toggle=${toggleChecked}, toast="${toastText}", log=${JSON.stringify(log)}`);
            }
        } catch (e) {
            const toggleChecked = await page.isChecked('#push-toggle');
            const log = await page.evaluate(() => window._fetchLog);
            fail('subscribe-500', `timeout. toggle=${toggleChecked}, log=${JSON.stringify(log)}, err=${e.message}`);
        }
        await page.close();
    }

    // --- Path 2: Subscribe POST network error ---
    console.log('\n--- Path 2: subscribe POST network error ---');
    {
        const page = await context.newPage();
        await page.goto(BASE);
        await page.waitForTimeout(2000);

        await page.evaluate(() => { window._fetchMode = 'network-error'; window._fetchLog = []; });
        await page.click('#push-toggle');
        try {
            await waitForToast(page);
            const toastText = await page.evaluate(() => document.getElementById('push-error-toast').textContent);
            const toggleChecked = await page.isChecked('#push-toggle');
            if (!toggleChecked && toastText.includes('No se pudieron activar')) {
                ok('subscribe-network: toggle reverted + error toast');
            } else {
                fail('subscribe-network', `toggle=${toggleChecked}, toast="${toastText}"`);
            }
        } catch (e) {
            fail('subscribe-network', `timeout: ${e.message}`);
        }
        await page.close();
    }

    // --- Path 3: Unsubscribe POST returns 500 ---
    console.log('\n--- Path 3: unsubscribe POST failure (non-ok response) ---');
    {
        const page = await context.newPage();
        await page.goto(BASE);
        await page.waitForTimeout(2000);

        await page.evaluate(() => { window._fetchMode = 'ok'; window._fetchLog = []; });
        await page.click('#push-toggle');
        await page.waitForTimeout(3000);
        const afterSubscribe = await page.isChecked('#push-toggle');
        if (!afterSubscribe) {
            const log = await page.evaluate(() => window._fetchLog);
            fail('unsubscribe-500', `subscribe did not stick. log=${JSON.stringify(log)}`);
        } else {
            await page.evaluate(() => { window._fetchMode = 'fail-status'; window._fetchLog = []; });
            await page.click('#push-toggle');
            try {
                await waitForToast(page);
                const toastText = await page.evaluate(() => document.getElementById('push-error-toast').textContent);
                const log = await page.evaluate(() => window._fetchLog);
                if (toastText.includes('desactivaron localmente') && log.some(l => l.url.includes('/push/unsubscribe'))) {
                    ok('unsubscribe-500: error toast with correct message + POST made');
                } else {
                    fail('unsubscribe-500', `toast="${toastText}", log=${JSON.stringify(log)}`);
                }
            } catch (e) {
                fail('unsubscribe-500', `timeout: ${e.message}`);
            }
        }
        await page.close();
    }

    // --- Path 4: Unsubscribe POST network error ---
    console.log('\n--- Path 4: unsubscribe POST network error ---');
    {
        const page = await context.newPage();
        await page.goto(BASE);
        await page.waitForTimeout(2000);

        await page.evaluate(() => { window._fetchMode = 'ok'; window._fetchLog = []; });
        await page.click('#push-toggle');
        await page.waitForTimeout(3000);
        const afterSubscribe = await page.isChecked('#push-toggle');
        if (!afterSubscribe) {
            const log = await page.evaluate(() => window._fetchLog);
            fail('unsubscribe-network', `subscribe did not stick. log=${JSON.stringify(log)}`);
        } else {
            await page.evaluate(() => { window._fetchMode = 'network-error'; window._fetchLog = []; });
            await page.click('#push-toggle');
            try {
                await waitForToast(page);
                const toastText = await page.evaluate(() => document.getElementById('push-error-toast').textContent);
                if (toastText.includes('desactivaron localmente')) {
                    ok('unsubscribe-network: error toast with correct message');
                } else {
                    fail('unsubscribe-network', `toast="${toastText}"`);
                }
            } catch (e) {
                fail('unsubscribe-network', `timeout: ${e.message}`);
            }
        }
        await page.close();
    }

    console.log(`\n=== ${passed}/${total} TESTS PASSED ===`);
} finally {
    await context.close();
    server.close();
}

if (passed < total) process.exit(1);
