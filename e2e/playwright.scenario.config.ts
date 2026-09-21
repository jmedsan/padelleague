import { defineConfig } from '@playwright/test';

// Fallback differs from the main suite (8099) so both can run concurrently.
const PORT = process.env.E2E_PORT ? Number(process.env.E2E_PORT) : 8098;

export default defineConfig({
  testDir: './scenarios',
  retries: 0,
  workers: 1,
  timeout: 120000,
  globalSetup: './scenario-setup.ts',
  globalTeardown: './scenario-teardown.ts',
  use: {
    baseURL: `http://localhost:${PORT}`,
  },
  projects: [
    {
      name: 'desktop',
      use: { viewport: { width: 1280, height: 720 } },
    },
  ],
});
