import { test } from '@playwright/test';
import { ScenarioApi, ScenarioData, loadCtx, printChatMatch, saveCtx, seedChatMatch } from '../scenario-helpers';

test.describe('chat email for a match with a pending date and place proposal via Mailpit', () => {
  test.describe.configure({ retries: 0 });

  test('00 seed: as mail-chat, plus a date, time and venue proposed by p01 and not accepted', async () => {
    const raw = loadCtx();
    const api: ScenarioApi = { baseURL: raw.baseURL, suToken: raw.suToken, adminCookie: raw.adminCookie };
    const m = await seedChatMatch(api, 'Chat con propuesta', 'proposed');
    const ctx: ScenarioData = { ...raw, competitionId: m.competitionId, players: m.players, pairs: m.pairs, stage: 'blank' };
    saveCtx(ctx);
    printChatMatch(ctx.baseURL, m);
  });
});
