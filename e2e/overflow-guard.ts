import { test as base, expect } from '@playwright/test';
import type { Page, Response, ConsoleMessage } from '@playwright/test';

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
      const style = getComputedStyle(el);
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
        const id = el.id ? `#${el.id}` : '';
        const href = el.tagName === 'A' ? ` href="${el.getAttribute('href') ?? ''}"` : '';
        offenders.push(`${el.tagName.toLowerCase()}${id}${href}`);
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
// project's own stated minimum ("min 44px"). Scope matches WCAG
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
    for (const el of document.body.querySelectorAll<HTMLElement>(selector)) {
      const style = getComputedStyle(el);
      if (!el.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true })) continue;
      if (el.tagName === 'A' && style.display === 'inline') continue;
      const rect = el.getBoundingClientRect();
      if (rect.width <= 4 || rect.height <= 4) continue;
      if (rect.width < 44 || rect.height < 44) {
        const id = el.id ? `#${el.id}` : '';
        const text = el.textContent?.trim().slice(0, 20) ?? '';
        offenders.push(`${el.tagName.toLowerCase()}${id} "${text}" (${Math.round(rect.width)}x${Math.round(rect.height)})`);
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
async function checkAll(page: Page, isMobileProject: boolean): Promise<string[]> {
  const url = page.url();
  const violations: string[] = [];
  const overflow = await findOverflowOffenders(page).catch(() => []);
  if (overflow.length > 0) violations.push(`[overflow] ${url}: ${describeOffenders(overflow)}`);
  const clipped = await findClippedText(page).catch(() => []);
  if (clipped.length > 0) violations.push(`[clipped-text] ${url}: ${clipped.join('; ')}`);
  const empty = await findEmptyInteractive(page).catch(() => []);
  if (empty.length > 0) violations.push(`[empty-interactive] ${url}: ${empty.join('; ')}`);
  const broken = await findBrokenImages(page).catch(() => []);
  if (broken.length > 0) violations.push(`[broken-img] ${url}: ${broken.join('; ')}`);
  const dupes = await findDuplicateIds(page).catch(() => []);
  if (dupes.length > 0) violations.push(`[duplicate-id] ${url}: ${dupes.join('; ')}`);
  const alerts = await findUnvariantedAlerts(page).catch(() => []);
  if (alerts.length > 0) violations.push(`[alert-no-variant] ${url}: ${alerts.join('; ')}`);
  const badges = await findEmptyBadges(page).catch(() => []);
  if (badges.length > 0) violations.push(`[empty-badge] ${url}: ${badges.join('; ')}`);
  const navOverlap = await findOverlappingNavbarSiblings(page).catch(() => []);
  if (navOverlap.length > 0) violations.push(`[navbar-overlap] ${url}: ${navOverlap.join('; ')}`);
  if (isMobileProject) {
    const small = await findSmallTapTargets(page).catch(() => []);
    if (small.length > 0) violations.push(`[small-tap-target] ${url}: ${small.join('; ')}`);
  }
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
export const test = base.extend<{ pageGuards: void }>({
  pageGuards: [async ({ page }, use, testInfo) => {
    const isMobileProject = testInfo.project.name === 'mobile';
    const violations: string[] = [];
    const pending: Promise<void>[] = [];

    const recordDOMGuards = () => {
      const check = checkAll(page, isMobileProject)
        .then(found => { violations.push(...found); })
        .catch(() => {}); // page mid-navigation/closed — nothing to record
      pending.push(check);
    };
    page.on('load', recordDOMGuards);
    page.on('framenavigated', recordDOMGuards);

    const onConsole = (msg: ConsoleMessage) => {
      if (msg.type() === 'error') {
        violations.push(`[console-error] ${page.url()}: ${msg.text().slice(0, 200)}`);
      }
    };
    page.on('console', onConsole);

    const onResponse = (response: Response) => {
      if (response.status() >= 400) {
        violations.push(`[http-error] ${response.status()} ${response.request().method()} ${response.url()}`);
      }
    };
    page.on('response', onResponse);

    await use();

    page.off('load', recordDOMGuards);
    page.off('framenavigated', recordDOMGuards);
    page.off('console', onConsole);
    page.off('response', onResponse);
    // Drain any check still in flight from the last navigation before
    // reading `violations` below — otherwise a late-resolving promise from
    // the test's final `goto` could land after the assertion already ran.
    await Promise.all(pending);

    const finalViolations = await checkAll(page, isMobileProject).catch(() => []);
    violations.push(...finalViolations.map(v => v.replace(/^\[(\w[\w-]*)\]/, '[$1:end]')));

    expect(violations, 'page guard violations').toEqual([]);
  }, { auto: true }],
});

export { expect };
