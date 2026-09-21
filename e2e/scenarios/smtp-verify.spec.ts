import { test, expect } from '@playwright/test';
import { readFileSync } from 'fs';
import { join } from 'path';
import {
  ScenarioApi,
  ScenarioCtx,
  apiGet,
  startSmtpSink,
  enableSmtp,
  disableSmtp,
  playMatchFastForward,
} from '../scenario-helpers';

// Load the baseline context written by scenario-setup.ts globalSetup.
function loadCtx(): ScenarioCtx & { baseURL: string; suToken: string; adminCookie: string } {
  const raw = readFileSync(join(__dirname, '../.test-data/scenario.json'), 'utf-8');
  return JSON.parse(raw);
}

test('smtp sink receives email on match finalization', async () => {
  const saved = loadCtx();
  const api: ScenarioApi = {
    baseURL: saved.baseURL,
    suToken: saved.suToken,
    adminCookie: saved.adminCookie,
  };

  // Pick one pending match from the competition.
  const data = await apiGet(
    api,
    `/api/collections/matches/records?filter=competition='${saved.competitionId}'%20%26%26%20status='pending'&perPage=1`,
  );
  const match = data.items[0];
  if (!match) throw new Error('No pending match found in baseline — invariant violated');

  const sink = await startSmtpSink();
  try {
    await enableSmtp(api, sink.port);

    // Verify settings took effect before proceeding.
    const settings = await apiGet(api, '/api/settings');
    if (!settings.smtp?.enabled) throw new Error('SMTP not enabled after PATCH');

    // Fast-forward fires the finalization hook which triggers notifications.
    await playMatchFastForward(api, match.id, '6-3 6-3', match.pair1);

    // emailNotification runs synchronously inside the hook, so no wait needed —
    // but allow a brief window for any OS-level TCP buffering.
    await new Promise((r) => setTimeout(r, 500));

    expect(sink.messages.length).toBeGreaterThan(0);
  } finally {
    await disableSmtp(api);
    await sink.close();
  }
});
