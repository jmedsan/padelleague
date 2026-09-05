import { defineConfig } from '@playwright/test';

// `make e2e` picks a free port up front (e2e/find-free-port.mjs) and passes
// it as E2E_PORT so this config and global-setup.ts agree on the same
// value — Playwright reads this config synchronously before globalSetup
// runs, so the port can't be resolved asynchronously here. Running
// `npx playwright test` directly (bypassing make e2e) falls back to 8099.
const PORT = process.env.E2E_PORT ? Number(process.env.E2E_PORT) : 8099;

export default defineConfig({
  testDir: './tests',
  retries: 1,
  workers: 1,
  timeout: 60000,
  globalSetup: './global-setup.ts',
  globalTeardown: './global-teardown.ts',
  use: {
    baseURL: `http://localhost:${PORT}`,
  },
  projects: [
    {
      name: 'desktop',
      testIgnore: /z-admin-settings/,
      use: {
        viewport: { width: 1280, height: 720 },
      },
    },
    {
      name: 'mobile',
      testIgnore: /z-admin-settings/,
      use: {
        viewport: { width: 375, height: 812 },
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
