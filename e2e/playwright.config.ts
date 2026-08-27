import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  retries: 1,
  workers: 1,
  timeout: 60000,
  globalSetup: './global-setup.ts',
  globalTeardown: './global-teardown.ts',
  use: {
    baseURL: 'http://localhost:8099',
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
