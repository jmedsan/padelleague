import { defineConfig } from '@playwright/test';
import { SCENARIOS } from './scenario-registry';

const PORT = process.env.E2E_PORT ? Number(process.env.E2E_PORT) : 8098;
const scenarioName = process.env.SCENARIO ?? '';
const scenario = SCENARIOS[scenarioName];

if (!scenario) {
  const names = Object.entries(SCENARIOS)
    .map(([k, v]) => `  ${k.padEnd(22)} ${v.description}`)
    .join('\n');
  throw new Error(
    `SCENARIO env var is missing or unknown.\n\nAvailable scenarios:\n${names}\n\nUsage: make e2e-scenario SCENARIO=<name>`,
  );
}

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
  projects: scenario.specs.map((file, i) => ({
    name: file.replace('.spec.ts', ''),
    testMatch: new RegExp(file.replace(/\./g, '\\.') + '$'),
    dependencies: i > 0
      ? [scenario.specs[i - 1].replace('.spec.ts', '')]
      : [],
    use: { viewport: { width: 1280, height: 720 } },
  })),
});
