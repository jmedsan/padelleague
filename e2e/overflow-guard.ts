import { test as base, expect } from '@playwright/test';
import type { Page } from '@playwright/test';

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
      if (style.position === 'fixed' || style.display === 'none' || style.visibility === 'hidden') continue;
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

// Custom `test` that, on the mobile project only, records horizontal overflow
// after every navigation and asserts none was ever seen, once, at the end of
// the test. `auto: true` means every test gets the guard for free — no
// per-spec opt-in, so a new page added anywhere is covered without anyone
// remembering to wire it.
//
// Recording happens in `page.on('load'/'framenavigated')` handlers rather
// than asserting inline there: `expect()` thrown from inside an event
// handler is invisible to Playwright's test runner (it isn't on the awaited
// call stack), so it would silently pass. Instead each navigation appends
// what it found to `violations`, and the fixture's teardown — which DOES run
// on the test's stack — asserts the accumulated list is empty, plus a final
// check of the page's end state.
export const test = base.extend<{ overflowGuard: void }>({
  overflowGuard: [async ({ page }, use, testInfo) => {
    if (testInfo.project.name !== 'mobile') {
      await use();
      return;
    }
    const violations: string[] = [];
    const pending: Promise<void>[] = [];
    const recordOverflow = () => {
      const url = page.url();
      const check = findOverflowOffenders(page)
        .then(offenders => {
          if (offenders.length > 0) {
            violations.push(`after navigation to ${url}: ${describeOffenders(offenders)}`);
          }
        })
        .catch(() => {}); // page mid-navigation/closed — nothing to record
      pending.push(check);
    };
    page.on('load', recordOverflow);
    page.on('framenavigated', recordOverflow);

    await use();

    page.off('load', recordOverflow);
    page.off('framenavigated', recordOverflow);
    // Drain any check still in flight from the last navigation before
    // reading `violations` below — otherwise a late-resolving promise from
    // the test's final `goto` could land after the assertion already ran.
    await Promise.all(pending);

    const finalOffenders = await findOverflowOffenders(page).catch(() => []);
    if (finalOffenders.length > 0) {
      violations.push(`at test end: ${describeOffenders(finalOffenders)}`);
    }
    expect(violations, 'horizontal overflow detected').toEqual([]);
  }, { auto: true }],
});

export { expect };
