import { defineConfig } from '@playwright/test';

// `make e2e` picks a free port up front (e2e/find-free-port.mjs) and passes
// it as E2E_PORT so this config and global-setup.ts agree on the same
// value — Playwright reads this config synchronously before globalSetup
// runs, so the port can't be resolved asynchronously here. Running
// `npx playwright test` directly (bypassing make e2e) falls back to 8099.
const PORT = process.env.E2E_PORT ? Number(process.env.E2E_PORT) : 8099;

// Specs whose purpose is viewport/layout coverage — everything else runs
// once, on desktop only. The overflow-guard fixture (tap-target sizing,
// horizontal-overflow checks) is gated by isMobileProject and runs on every
// test regardless of file, so this list is deliberately broad: any spec
// whose *point* is layout/responsive/tour/route coverage, not just the
// files with "mobile" in the name.
const MOBILE_VIEWPORT_SPECS = [
  'navbar-360.spec.ts',
  'responsive.spec.ts',
  'presentation-guards.spec.ts',
  'screens.spec.ts',
  'tour-guided.spec.ts',
  'tour-reference.spec.ts',
  'route-census.spec.ts',
  'mobile-lifecycle.spec.ts',
  'search.spec.ts',
  'admin-edit-forms.spec.ts',
  'player-contact.spec.ts',
  'provisional-warning.spec.ts',
  'notification-dismiss-mobile.spec.ts',
  'leveled-league-phone.spec.ts',
];
const mobileViewportMatch = MOBILE_VIEWPORT_SPECS.map(f => new RegExp(f.replace('.', '\\.') + '$'));

export default defineConfig({
  testDir: './tests',
  retries: 1,
  workers: 1,
  timeout: 30000,
  expect: { timeout: 5000 },
  use: {
    baseURL: `http://localhost:${PORT}`,
    actionTimeout: 10000,
    navigationTimeout: 15000,
  },
  projects: [
    {
      name: 'desktop',
      // admin-edit-forms exists solely to verify the 44px touch-tap-target
      // floor (static/css/input.css's `@media (hover: none)` rule) on the
      // real mobile viewport — that floor deliberately never applies to a
      // mouse-capable desktop context, so running it there only reproduces
      // expected-non-compliant sizes, not a defect.
      testIgnore: /z-admin-settings|admin-edit-forms|notification-dismiss-mobile|leveled-league-phone/,
      use: {
        viewport: { width: 1280, height: 720 },
      },
    },
    {
      name: 'mobile',
      testMatch: mobileViewportMatch,
      use: {
        // Samsung Galaxy S23 (owner's real device): 360×780 CSS px, DPR 3,
        // touch. Previously a generic 375×812 iPhone-ish viewport, which
        // missed a real overflow (competition tabs, scrollWidth 482 > 360)
        // that only showed up at the S23's narrower width.
        viewport: { width: 360, height: 780 },
        deviceScaleFactor: 3,
        isMobile: true,
        hasTouch: true,
      },
    },
    {
      name: 'destructive',
      testMatch: /z-admin-settings/,
      dependencies: ['desktop', 'mobile'],
      use: {
        viewport: { width: 1280, height: 720 },
      },
    },
  ],
});
