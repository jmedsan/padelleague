import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  retries: 1,
  workers: 1,
  timeout: 30000,
  globalSetup: './global-setup.ts',
  globalTeardown: './global-teardown.ts',
  use: {
    baseURL: 'http://localhost:8099',
    viewport: { width: 1280, height: 720 },
  },
});
