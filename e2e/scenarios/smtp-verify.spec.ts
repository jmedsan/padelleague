import { test, expect } from '@playwright/test';
import { readFileSync } from 'fs';
import { join } from 'path';
import { loginAs } from '../helpers';
import { enterScore, clickAndWaitForHxRedirect } from '../tour-helpers';
import {
  ScenarioApi,
  ScenarioCtx,
  apiGet,
  apiPatch,
  startSmtpSink,
  enableSmtp,
  disableSmtp,
  PLAYER_PASSWORD,
} from '../scenario-helpers';

// Load the baseline context written by scenario-setup.ts globalSetup.
function loadCtx(): ScenarioCtx & { baseURL: string; suToken: string; adminCookie: string } {
  const raw = readFileSync(join(__dirname, '../.test-data/scenario.json'), 'utf-8');
  return JSON.parse(raw);
}

test('smtp sink receives email on match finalization', async ({ page }) => {
  const saved = loadCtx();
  const api: ScenarioApi = {
    baseURL: saved.baseURL,
    suToken: saved.suToken,
    adminCookie: saved.adminCookie,
  };

  // Pick a pending match and drive it through the real submit → accept HTTP
  // flow (not a raw record PATCH). A raw PATCH to status='final' only fires
  // the OnRecordUpdate hook (top-up assignment, notifying only when a pair
  // still has assignment budget left — not deterministic); the real accept
  // flow calls league.NotifResultConfirmed unconditionally, which is what
  // this spec actually needs to verify SMTP delivery against.
  const data = await apiGet(
    api,
    `/api/collections/matches/records?filter=competition='${saved.competitionId}'%20%26%26%20status='pending'&perPage=1`,
  );
  const match = data.items[0];
  if (!match) throw new Error('No pending match found in baseline — invariant violated');

  const pair1Record = await apiGet(api, `/api/collections/pairs/records/${match.pair1}`);
  const pair2Record = await apiGet(api, `/api/collections/pairs/records/${match.pair2}`);
  const player1Record = await apiGet(api, `/api/collections/users/records/${pair1Record.player1}`);
  const player2Record = await apiGet(api, `/api/collections/users/records/${pair2Record.player1}`);

  // Date and club are required before score submission.
  await apiPatch(api, `/api/collections/matches/records/${match.id}`, {
    date: '2026-10-01T10:00:00.000Z',
    club: 'Test Club',
  });

  const sink = await startSmtpSink();
  try {
    await enableSmtp(api, sink.port);

    const settings = await apiGet(api, '/api/settings');
    if (!settings.smtp?.enabled) throw new Error('SMTP not enabled after PATCH');

    // Player 1 submits the score.
    await loginAs(page, player1Record.email, PLAYER_PASSWORD);
    await page.goto(`/match/${match.id}`);
    await page.waitForLoadState('domcontentloaded');
    await enterScore(page, '6-3 6-4');
    await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Enviar resultado")').first());

    // Player 2 accepts — this fires NotifResultConfirmed to player 1
    // (league/result.go), unconditionally on finalization.
    await loginAs(page, player2Record.email, PLAYER_PASSWORD);
    await page.goto(`/match/${match.id}`);
    await page.waitForLoadState('domcontentloaded');
    await page.locator('#thread-details').waitFor({ timeout: 15000 });
    const acceptBtn = page.locator('#thread-details button:has-text("Aceptar resultado")').first();
    await acceptBtn.waitFor({ timeout: 10000 });
    await clickAndWaitForHxRedirect(page, acceptBtn);

    // emailNotification runs synchronously inside the handler, so no wait
    // needed — but allow a brief window for any OS-level TCP buffering.
    await new Promise((r) => setTimeout(r, 500));

    expect(sink.messages.length).toBeGreaterThan(0);
  } finally {
    await disableSmtp(api);
    await sink.close();
  }
});
