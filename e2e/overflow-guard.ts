import { test as base, expect } from './worker-server';
import type { Page, Response, ConsoleMessage } from '@playwright/test';
import { expectingHxRedirect } from './tour-helpers';

// Widest-offender detail for a horizontal-overflow failure: element,
// scrollWidth/clientWidth pair, so a red run tells you what to fix instead of
// just "something overflowed somewhere on some page".
interface OverflowOffender {
  selector: string;
  scrollWidth: number;
  clientWidth: number;
}

// Finds elements whose content is wider than the box that's supposed to
// contain it — the S23 competition-tabs bug (scrollWidth 482 > clientWidth
// 360) was exactly this, just never asserted on automatically. Runs in the
// page context: fixed/hidden elements are excluded because they don't
// contribute to the page's real scrollable width (a fixed-position drawer or
// a display:none template fragment can be wider than the viewport by design).
async function findOverflowOffenders(page: Page): Promise<OverflowOffender[]> {
  return page.evaluate(() => {
    const root = document.documentElement;
    if (root.scrollWidth <= root.clientWidth) return [];

    const offenders: { selector: string; scrollWidth: number; clientWidth: number }[] = [];
    const all = document.body.querySelectorAll<HTMLElement>('*');
    for (const el of all) {
      const style = getComputedStyle(el);
      if (style.position === 'fixed' || !el.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true })) continue;
      if (el.scrollWidth > el.clientWidth && el.scrollWidth > root.clientWidth) {
        const id = el.id ? `#${el.id}` : '';
        const cls = el.className && typeof el.className === 'string'
          ? `.${el.className.trim().split(/\s+/).slice(0, 3).join('.')}`
          : '';
        offenders.push({
          selector: `${el.tagName.toLowerCase()}${id}${cls}`,
          scrollWidth: el.scrollWidth,
          clientWidth: el.clientWidth,
        });
      }
    }
    // Widest offenders first — the one that actually drives the page's
    // scrollWidth is almost always the root cause, not a nested descendant.
    return offenders.sort((a, b) => b.scrollWidth - a.scrollWidth).slice(0, 5);
  });
}

function describeOffenders(offenders: OverflowOffender[]): string {
  return offenders
    .map(o => `${o.selector} (scrollWidth ${o.scrollWidth} > clientWidth ${o.clientWidth})`)
    .join('; ');
}

// findClippedText looks for content silently cut off — not the deliberate
// single-line ellipsis this codebase uses ~15+ places (Tailwind `.truncate`,
// text-overflow: ellipsis), but overflow:hidden/clip boxes where content
// exceeds the box in either axis with no ellipsis and no scrollbar. That
// combination means a real user sees truncated text with no visual signal
// anything is missing — the flash-alert-missing-class class of bug (census
// #20), generalized past "missing a class" to "any silent clip". Scoped to
// leaf elements (no element children — text nodes only): a layout wrapper
// with overflow-hidden and a wide descendant (a deliberate full-bleed hero,
// this codebase's login.html has one) reports the same scrollWidth mismatch
// on the wrapper without a single character of its own text being clipped;
// only a leaf's own box being too small for its own text is the real bug.
function findClippedText(page: Page): Promise<string[]> {
  return page.evaluate(() => {
    const offenders: string[] = [];
    const all = document.body.querySelectorAll<HTMLElement>('*');
    for (const el of all) {
      if (el.children.length > 0) continue;
      if (!el.textContent?.trim()) continue;
      if (el.classList.contains('sr-only')) continue;
      const style = getComputedStyle(el);
      // `clip`/`clip-path` (Tailwind's own sr-only utility, and any other
      // visually-hidden-but-announced pattern) clips on purpose for screen
      // readers — that's not the silent-truncation bug this guard hunts.
      if (style.clipPath !== 'none' || style.clip !== 'auto') continue;
      if (!el.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true })) continue;
      const hidesOverflow = style.overflow === 'hidden' || style.overflow === 'clip'
        || style.overflowX === 'hidden' || style.overflowX === 'clip'
        || style.overflowY === 'hidden' || style.overflowY === 'clip';
      if (!hidesOverflow || style.textOverflow === 'ellipsis') continue;
      const clippedX = el.scrollWidth > el.clientWidth + 1;
      const clippedY = el.scrollHeight > el.clientHeight + 1;
      if (clippedX || clippedY) {
        const id = el.id ? `#${el.id}` : '';
        const cls = el.className && typeof el.className === 'string'
          ? `.${el.className.trim().split(/\s+/).slice(0, 3).join('.')}`
          : '';
        offenders.push(`${el.tagName.toLowerCase()}${id}${cls} (${clippedX ? 'horizontal' : ''}${clippedX && clippedY ? '+' : ''}${clippedY ? 'vertical' : ''} clip, no ellipsis)`);
      }
    }
    return offenders.slice(0, 5);
  });
}

// findEmptyInteractive flags a visible <a>/<button> with no accessible name:
// no text content, no aria-label, and (for <a>) no title — a link/button a
// screen reader announces as blank and a sighted user sees as a dead
// clickable area. Icon-only buttons are legitimate in this codebase
// (component-modes.md) but MUST carry aria-label; this guard is exactly the
// mechanical enforcement of that rule.
function findEmptyInteractive(page: Page): Promise<string[]> {
  return page.evaluate(() => {
    const offenders: string[] = [];
    const els = document.body.querySelectorAll<HTMLElement>('a, button');
    for (const el of els) {
      if (!el.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true })) continue;
      if (el.getAttribute('aria-hidden') === 'true') continue;
      const hasText = !!el.textContent?.trim();
      const hasLabel = !!el.getAttribute('aria-label')?.trim() || !!el.getAttribute('title')?.trim();
      const hasImgAlt = !!el.querySelector('img[alt]:not([alt=""])');
      if (!hasText && !hasLabel && !hasImgAlt) {
        offenders.push(el.outerHTML.slice(0, 300));
      }
    }
    return offenders.slice(0, 5);
  });
}

// findBrokenImages flags a rendered (non-lazy-pending) <img> whose
// naturalWidth is 0 — the browser attempted the request and it failed (404,
// broken URL, CORS), as opposed to a lazy-loaded image simply not yet in
// the viewport (which also has naturalWidth 0 before it loads, so this
// only counts images the browser has finished attempting: complete === true).
function findBrokenImages(page: Page): Promise<string[]> {
  return page.evaluate(() => {
    const offenders: string[] = [];
    const imgs = document.body.querySelectorAll<HTMLImageElement>('img');
    for (const img of imgs) {
      if (img.complete && img.naturalWidth === 0 && img.src) {
        offenders.push(`img[src="${img.src}"]`);
      }
    }
    return offenders.slice(0, 5);
  });
}

// findDuplicateIds flags any `id` attribute value used on more than one
// element. Duplicate ids break `document.getElementById`, `label[for]`
// association, and any :target/fragment link — always a bug, never
// intentional.
function findDuplicateIds(page: Page): Promise<string[]> {
  return page.evaluate(() => {
    const counts = new Map<string, number>();
    for (const el of document.body.querySelectorAll<HTMLElement>('[id]')) {
      if (!el.id) continue;
      counts.set(el.id, (counts.get(el.id) ?? 0) + 1);
    }
    return [...counts.entries()].filter(([, n]) => n > 1).map(([id, n]) => `#${id} (${n}x)`).slice(0, 5);
  });
}

// findSmallTapTargets flags a visible tap target under 44x44 CSS px — this
// project's own stated minimum (CLAUDE.md: "min 44px"). Scope matches WCAG
// 2.5.5's own carve-out (and this codebase's real layout): buttons, form
// controls, and standalone icon links are held to 44px; a plain inline text
// link inside running prose or a footer list (this project's own legal-link
// row, breadcrumbs) is exempt — `display: inline` is the signal an <a> is
// text-flow, not a discrete tap target, since a block/inline-block/flex link
// is deliberately sized as a button-like control. Elements smaller than 4px
// in either dimension are treated as decorative (0-size icons intentionally
// hidden by a parent) rather than a violation.
function findSmallTapTargets(page: Page): Promise<string[]> {
  return page.evaluate(() => {
    const offenders: string[] = [];
    const selector = 'a, button, select, input:not([type="hidden"]):not([type="checkbox"]):not([type="radio"])';
    // A test step can start a transition while this check runs (the tour
    // opens the bell dropdown, which scales from 0.95 over 0.2 s), so the
    // wait in sampleDOM can't cover it. An element inside a running finite
    // animation is measured by its layout box, which transforms don't touch.
    const moving = document.getAnimations()
      .filter(a => a.playState === 'running' && a.effect?.getComputedTiming().endTime !== Infinity)
      .map(a => (a.effect as KeyframeEffect | null)?.target)
      .filter((t): t is Element => t instanceof Element);
    for (const el of document.body.querySelectorAll<HTMLElement>(selector)) {
      const style = getComputedStyle(el);
      if (!el.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true })) continue;
      if (el.tagName === 'A' && style.display === 'inline') continue;
      const rect = el.getBoundingClientRect();
      // checkVisibility() doesn't account for a `transform` pushing the
      // element off-canvas (DaisyUI's closed drawer-side: pointer-events:
      // none + translateX(-100%)) — that leaves it "visible" by the DOM's
      // own rules while a real user can neither see nor tap it. A rect
      // entirely outside the viewport means exactly that: skip it.
      if (rect.right <= 0 || rect.bottom <= 0 || rect.left >= window.innerWidth || rect.top >= window.innerHeight) continue;
      const animating = moving.some(t => t.contains(el));
      const width = animating ? el.offsetWidth : rect.width;
      const height = animating ? el.offsetHeight : rect.height;
      if (width <= 4 || height <= 4) continue;
      if (width < 44 || height < 44) {
        const id = el.id ? `#${el.id}` : '';
        const text = el.textContent?.trim().slice(0, 20) ?? '';
        offenders.push(`${el.tagName.toLowerCase()}${id} "${text}" (${Math.round(width)}x${Math.round(height)})`);
      }
    }
    return offenders.slice(0, 8);
  });
}

// findUnvariantedAlerts flags a `.alert` with none of DaisyUI's variant
// classes — the flashAlert-missing-base-class bug (census #20) generalized:
// a status message with no color/variant is invisible/indistinguishable
// styling regardless of which specific class went missing.
const ALERT_VARIANTS = ['alert-info', 'alert-success', 'alert-warning', 'alert-error'];
function findUnvariantedAlerts(page: Page): Promise<string[]> {
  return page.evaluate((variants) => {
    const offenders: string[] = [];
    for (const el of document.body.querySelectorAll<HTMLElement>('.alert')) {
      if (!el.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true })) continue;
      const classes = el.className.split(/\s+/);
      if (!variants.some(v => classes.includes(v))) {
        const id = el.id ? `#${el.id}` : '';
        offenders.push(`.alert${id} classes="${el.className}"`);
      }
    }
    return offenders.slice(0, 5);
  }, ALERT_VARIANTS);
}

// findEmptyBadges flags a visible `.badge` with no text content — a status
// pill rendering as an empty pill conveys nothing to the user.
function findEmptyBadges(page: Page): Promise<string[]> {
  return page.evaluate(() => {
    const offenders: string[] = [];
    for (const el of document.body.querySelectorAll<HTMLElement>('.badge')) {
      if (!el.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true })) continue;
      if (!el.textContent?.trim()) {
        offenders.push(`.badge${el.id ? `#${el.id}` : ''} (empty)`);
      }
    }
    return offenders.slice(0, 5);
  });
}

// findWrappedBadges flags a visible `.badge` whose text runs over more than
// one line. A badge is a fixed-height pill, so a wrapped label spills out of
// it (the admin card's "1 alerta" on 360px) without overflowing the page or
// being clipped, which the other checks look for.
function findWrappedBadges(page: Page): Promise<string[]> {
  return page.evaluate(() => {
    const offenders: string[] = [];
    for (const el of document.body.querySelectorAll<HTMLElement>('.badge')) {
      if (!el.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true })) continue;
      // Text rects only, and a new line only where a rect starts below the
      // previous line: a badge holding a taller child (the penalty badge's
      // "×" button) centers its text at a different height on one line.
      const rects: DOMRect[] = [];
      const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
      for (let n = walker.nextNode(); n; n = walker.nextNode()) {
        const range = document.createRange();
        range.selectNodeContents(n);
        rects.push(...Array.from(range.getClientRects()).filter(r => r.width > 0 && r.height > 0));
      }
      rects.sort((a, b) => a.top - b.top);
      let lines = 0, bottom = -Infinity;
      for (const r of rects) {
        if (r.top >= bottom - 1) lines++;
        bottom = Math.max(bottom, r.bottom);
      }
      if (lines > 1) offenders.push(`.badge "${el.textContent?.trim().replace(/\s+/g, ' ')}" (${lines} lines)`);
    }
    return offenders.slice(0, 5);
  });
}

// findOverlappingNavbarSiblings generalizes the navbar-360 wordmark bug
// (60610ec): any two visible direct-child blocks of a `.navbar` whose
// horizontal ranges overlap. A navbar is laid out left-to-right in DOM
// order; two blocks overlapping means one is visually clipping or sitting
// on top of the other, exactly the "role switch overlapped the wordmark"
// shape — checked on every page instead of the one admin page the original
// bug was found on.
function findOverlappingNavbarSiblings(page: Page): Promise<string[]> {
  return page.evaluate(() => {
    const nav = document.querySelector('.navbar');
    if (!nav) return [];
    const blocks = [...nav.querySelectorAll(':scope > div')]
      .filter(d => (d as HTMLElement).offsetParent !== null)
      .map(d => ({ el: d, rect: d.getBoundingClientRect() }))
      .filter(b => b.rect.width > 0)
      .sort((a, b) => a.rect.left - b.rect.left);
    const offenders: string[] = [];
    for (let i = 1; i < blocks.length; i++) {
      const prev = blocks[i - 1];
      const cur = blocks[i];
      if (cur.rect.left < prev.rect.right - 1) {
        const idA = (prev.el as HTMLElement).id ? `#${(prev.el as HTMLElement).id}` : `nth-child block ${i - 1}`;
        const idB = (cur.el as HTMLElement).id ? `#${(cur.el as HTMLElement).id}` : `nth-child block ${i}`;
        offenders.push(`${idA} (right=${Math.round(prev.rect.right)}) overlaps ${idB} (left=${Math.round(cur.rect.left)})`);
      }
    }
    return offenders;
  });
}

// checkAll runs every DOM-snapshot guard (everything except the
// event-driven ones: console errors, HTTP >=400, and horizontal overflow,
// which are recorded continuously — see below) and returns one combined
// violation list, prefixed per navigation so a failure names which guard
// and which page.
//
// Exported for specs that open a manually-created second `browser.newContext`
// page instead of the fixture's own `page` (e.g. to compare two viewports
// side by side in one test) — that page is never wrapped by the `pageGuards`
// auto-fixture below, since the fixture only instruments the `page` it's
// handed. Call this directly on such a page, with hasTouch true to match a
// real phone, to get the same tap-target coverage a fixture page gets for
// free (scheduling-walkover.spec.ts's playoff-bracket mobile check).
//
// The checks are separate evaluates, so a navigation can land between them:
// page.url() already names the new document while an evaluate still reads
// the old one (the home page's cards were reported as empty under
// /match/<id>). A sample only counts when one document, at the URL it is
// reported under, answered every check: the first and the last evaluate
// read a per-document id (assigned on the document's first sample) and the
// href, and both must match. Otherwise the sample is dropped (see checkDOM).
// A document still parsing (readyState 'loading') is never sampled: the
// parser has inserted an <a> but not yet its text, so a link with text in
// the template reads as empty (the layout's "Preferencias" link, sampled by
// the teardown after a test failed mid-navigation). checkAll waits for its
// domcontentloaded and samples again; a document that doesn't get there in
// 5 s is a violation of its own, never a silent skip.
export function checkAll(page: Page, isMobileProject: boolean): Promise<string[]> {
  return checkDOM(page, isMobileProject, true);
}

// checkDOM samples the page, waiting out a document still parsing. A sample
// a navigation interrupted is handled by followNavigations:
//   - false (the pageGuards listeners): dropped. The navigation that
//     interrupted it fired framenavigated (for a new document and for a
//     same-document pushState alike), so the newer document has its own
//     check queued. Chasing it here instead made each stale check report the
//     test's own quick navigations as [unsettled].
//   - true (the exported checkAll and the teardown's final sample): taken
//     again on the newer document, since no listener stands behind them. A
//     page still changing after SAMPLE_ATTEMPTS samples, with no test step
//     driving it, is reported as [unsettled].
async function checkDOM(page: Page, isMobileProject: boolean, followNavigations: boolean): Promise<string[]> {
  for (let attempt = 0; attempt < SAMPLE_ATTEMPTS; attempt++) {
    const found = await sampleDOM(page, isMobileProject);
    if (Array.isArray(found)) return found;
    if (found === null && !followNavigations) return [];
    if (found === 'parsing') {
      try {
        await page.waitForLoadState('domcontentloaded', { timeout: PARSE_TIMEOUT_MS });
      } catch (err) {
        if (err instanceof Error && err.name === 'TimeoutError') {
          return [`[never-parsed] ${page.url()}: page never finished parsing within ${PARSE_TIMEOUT_MS} ms`];
        }
        throw err;
      }
    }
  }
  return [`[unsettled] ${page.url()}: the document changed during each of ${SAMPLE_ATTEMPTS} samples`];
}

const SAMPLE_ATTEMPTS = 3;

// guardError turns a check's error into a violation, so a broken guard
// fails the test instead of silently turning itself off. Only a page that
// closed under the check yields none: there is nothing left to record.
function guardError(err: unknown): string[] {
  const msg = err instanceof Error ? err.message : String(err);
  if (msg.includes('Target page, context or browser has been closed')) return [];
  return [`[guard-error] ${msg.slice(0, 300)}`];
}

// onNavigation(value) answers an evaluate that a navigation cut off with
// value (null: the sample is dropped). Any other error is thrown, and
// guardError reports it.
function onNavigation<T>(value: T): (err: unknown) => T {
  return err => {
    if (err instanceof Error && err.message.includes('Execution context was destroyed')) return value;
    throw err;
  };
}
const PARSE_TIMEOUT_MS = 5000;
const ANIMATION_TIMEOUT_MS = 2000;

// sampleDOM runs every check once. It returns null when a navigation
// changed the document mid-sample, and 'parsing' when the document had not
// finished parsing.
async function sampleDOM(page: Page, isMobileProject: boolean): Promise<string[] | null | 'parsing'> {
  const url = page.url();
  // The id is written once per document and only read after that, so
  // concurrent samples of one document (load and framenavigated both fire
  // per navigation) agree on it instead of overwriting each other.
  const before = await page.evaluate(id => {
    const w = window as unknown as { __guardDoc?: string };
    w.__guardDoc ??= id;
    return { doc: w.__guardDoc, href: location.href, parsing: document.readyState === 'loading' };
  }, Math.random().toString(36).slice(2)).catch(onNavigation(null));
  if (before === null || before.href !== url) return null;
  if (before.parsing) return 'parsing';
  const violations: string[] = [];
  // The geometry checks read an element's box mid-transition as its size:
  // the bell dropdown opens with a 0.2 s scale from 0.95, so its 44 px
  // links read 43 px while it plays. Finite animations and transitions
  // finish first; infinite ones (a spinner) never do and don't resize.
  const settled = await page.evaluate(async timeout => {
    const deadline = performance.now() + timeout;
    for (;;) {
      const running = document.getAnimations().filter(a =>
        a.playState === 'running' && a.effect?.getComputedTiming().endTime !== Infinity);
      if (running.length === 0) return true;
      const left = deadline - performance.now();
      if (left <= 0) return false;
      await Promise.race([
        Promise.allSettled(running.map(a => a.finished)),
        new Promise(resolve => setTimeout(resolve, left)),
      ]);
    }
  }, ANIMATION_TIMEOUT_MS).catch(onNavigation(null));
  if (settled === null) return null;
  if (!settled) violations.push(`[animating] ${url}: animations still running after ${ANIMATION_TIMEOUT_MS} ms`);
  // A check can fail because the document went away under it: the
  // context was destroyed, or the next document had no <body> yet
  // ("Cannot read properties of null"). Only the closing evaluate can tell
  // that from a broken check, so errors are held until then: a changed
  // document drops the sample, an unchanged one throws the first error.
  const errors: unknown[] = [];
  const held = (err: unknown): never[] => { errors.push(err); return []; };
  const overflow = await findOverflowOffenders(page).catch(held);
  if (overflow.length > 0) violations.push(`[overflow] ${url}: ${describeOffenders(overflow)}`);
  const clipped = await findClippedText(page).catch(held);
  if (clipped.length > 0) violations.push(`[clipped-text] ${url}: ${clipped.join('; ')}`);
  const empty = await findEmptyInteractive(page).catch(held);
  if (empty.length > 0) violations.push(`[empty-interactive] ${url}: ${empty.join('; ')}`);
  const broken = await findBrokenImages(page).catch(held);
  if (broken.length > 0) violations.push(`[broken-img] ${url}: ${broken.join('; ')}`);
  const dupes = await findDuplicateIds(page).catch(held);
  if (dupes.length > 0) violations.push(`[duplicate-id] ${url}: ${dupes.join('; ')}`);
  const alerts = await findUnvariantedAlerts(page).catch(held);
  if (alerts.length > 0) violations.push(`[alert-no-variant] ${url}: ${alerts.join('; ')}`);
  const badges = await findEmptyBadges(page).catch(held);
  if (badges.length > 0) violations.push(`[empty-badge] ${url}: ${badges.join('; ')}`);
  const wrapped = await findWrappedBadges(page).catch(held);
  if (wrapped.length > 0) violations.push(`[wrapped-badge] ${url}: ${wrapped.join('; ')}`);
  const navOverlap = await findOverlappingNavbarSiblings(page).catch(held);
  if (navOverlap.length > 0) violations.push(`[navbar-overlap] ${url}: ${navOverlap.join('; ')}`);
  if (isMobileProject) {
    // The 44px floor lives in input.css's `@media (hover: none)`, so sizes
    // measured without touch emulation belong to a mouse device. Chromium
    // drops that emulation for the page's life after any screenshot that
    // captures beyond the viewport (fullPage, element): measuring on then
    // would blame every compliant control. Report the lost emulation instead.
    const touch = await page.evaluate(() => matchMedia('(hover: none)').matches).catch(held);
    if (touch === false) {
      violations.push(`[no-touch-emulation] ${url}: (hover: none) doesn't match, so the 44px floor isn't active; a screenshot beyond the viewport turns touch emulation off`);
    } else {
      const small = await findSmallTapTargets(page).catch(held);
      if (small.length > 0) violations.push(`[small-tap-target] ${url}: ${small.join('; ')}`);
    }
  }
  const after = await page.evaluate(() => ({
    doc: (window as unknown as { __guardDoc?: string }).__guardDoc,
    href: location.href,
  })).catch(onNavigation(null));
  if (after === null || after.doc !== before.doc || after.href !== url || page.url() !== url) return null;
  if (errors.length > 0) throw errors[0];
  return violations;
}

// Custom `test` that attaches `pageGuards` to every test on every project
// (`auto: true` — no per-spec opt-in). After each navigation/HTMX settle it
// snapshots the DOM for the checks above; console errors and HTTP >=400
// responses are recorded continuously via page events since they can occur
// between navigations (an HTMX request that never triggers a `load` event).
//
// Recording happens in event handlers rather than asserting inline there:
// `expect()` thrown from inside an event handler is invisible to
// Playwright's test runner (it isn't on the awaited call stack), so it
// would silently pass. Instead each event appends to `violations`, and the
// fixture's teardown — which DOES run on the test's stack — asserts the
// accumulated list is empty, plus a final DOM snapshot of the page's end
// state.
// A test that deliberately navigates to a broken/missing URL (a 404 page
// test) declares it via `testInfo.annotations.push({ type:
// 'expected-http-error', description: '404 /match/' })` — status plus a URL
// substring. Only a response matching BOTH is allowed through; anything
// else (a different status, or a 404 on an unrelated URL) still fails the
// test. This is scoped per-test (annotations are per-testInfo, never
// shared), so it can't mask a real regression elsewhere.
function parseExpectedHTTPErrors(testInfo: { annotations: { type: string; description?: string }[] }): { status: number; urlSubstring: string }[] {
  return testInfo.annotations
    .filter(a => a.type === 'expected-http-error' && a.description)
    .map(a => {
      const [status, ...rest] = a.description!.trim().split(/\s+/);
      return { status: Number(status), urlSubstring: rest.join(' ') };
    });
}

export const test = base.extend<{ pageGuards: void }>({
  pageGuards: [async ({ page }, use, testInfo) => {
    const isMobileProject = testInfo.project.name === 'mobile';
    const violations: string[] = [];
    const pending: Promise<void>[] = [];
    // Read lazily (per event, not once here): this fixture's setup runs
    // BEFORE the test body, so a test that pushes its annotation from
    // inside the test (e.g. right before the goto it excuses) would
    // otherwise be invisible — testInfo.annotations wouldn't have it yet.
    const expectedHTTPErrors = () => parseExpectedHTTPErrors(testInfo);

    const recordDOMGuards = () => {
      // `framenavigated` fires the instant navigation commits — well before
      // the new document finishes parsing, so checking immediately can catch
      // a `<button>` whose icon SVG child hasn't been inserted yet and
      // misreport it as empty (flaky, not a real bug: e2e/tests/leveled-league.spec.ts
      // intermittently failed on this before the wait was added). checkAll
      // waits for domcontentloaded itself and reports a document that never
      // gets there.
      const check = checkDOM(page, isMobileProject, false)
        .catch(guardError)
        .then(found => { violations.push(...found); });
      pending.push(check);
    };
    page.on('load', recordDOMGuards);
    page.on('framenavigated', recordDOMGuards);

    const onConsole = (msg: ConsoleMessage) => {
      if (msg.type() === 'error') {
        // A resource that 404s (an expected-http-error page) also logs a
        // browser-native "Failed to load resource: ... 404" console error for
        // that same request — same annotation, same status/URL pair, covers
        // both. Anything else on the page still fails the test.
        const text = msg.text();
        const statusMatch = text.match(/status of (\d+)/);
        if (statusMatch) {
          const status = Number(statusMatch[1]);
          const expected = expectedHTTPErrors().some(e => e.status === status && page.url().includes(e.urlSubstring));
          if (expected) return;
        }
        violations.push(`[console-error] ${page.url()}: ${text.slice(0, 200)}`);
      }
    };
    page.on('console', onConsole);

    const onResponse = (response: Response) => {
      if (response.status() >= 400) {
        const expected = expectedHTTPErrors().some(e => e.status === response.status() && response.url().includes(e.urlSubstring));
        if (expected) return;
        violations.push(`[http-error] ${response.status()} ${response.request().method()} ${response.url()}`);
      }
    };
    page.on('response', onResponse);

    // Any response carrying HX-Redirect must have been triggered through
    // clickAndWaitForHxRedirect/clickConfirmAndWaitForHxRedirect — those set
    // expectingHxRedirect(page) before act() fires and clear it once the
    // redirect has landed. A bare `.click()` on the same kind of button skips
    // that wait, so the very next helper call on the page can race the
    // still-in-flight navigation (tour-guided.spec.ts's playoff-publish
    // button did exactly this — 100% reproducible "Execution context was
    // destroyed"). Catching it here, at the response that proves the class
    // of bug exists, finds every site — not just the one a grep for a
    // specific button label happens to name.
    const onHxRedirectResponse = (response: Response) => {
      if (response.frame() !== page.mainFrame()) return;
      // In pending like the DOM checks, so the teardown's expect waits for it.
      pending.push(response.headerValue('hx-redirect').then(target => {
        if (!target) return;
        if (expectingHxRedirect.get(page)) return;
        violations.push(
          `[unhelpered-hx-redirect] ${response.request().method()} ${response.url()} → ${target}: ` +
          `wrap the click in clickAndWaitForHxRedirect`,
        );
      }).catch(err => { violations.push(...guardError(err)); }));
    };
    page.on('response', onHxRedirectResponse);

    await use();

    page.off('load', recordDOMGuards);
    page.off('framenavigated', recordDOMGuards);
    page.off('console', onConsole);
    page.off('response', onResponse);
    page.off('response', onHxRedirectResponse);
    // Drain any check still in flight from the last navigation before
    // reading `violations` below — otherwise a late-resolving promise from
    // the test's final `goto` could land after the assertion already ran.
    await Promise.all(pending);

    const finalViolations = await checkAll(page, isMobileProject).catch(guardError);
    violations.push(...finalViolations.map(v => v.replace(/^\[(\w[\w-]*)\]/, '[$1:end]')));

    expect(violations, 'page guard violations').toEqual([]);
  }, { auto: true }],
});

export { expect };
