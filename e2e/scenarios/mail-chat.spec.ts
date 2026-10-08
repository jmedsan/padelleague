import { test } from '@playwright/test';
import { ScenarioApi, ScenarioData, loadCtx, printChatMatch, saveCtx, seedChatMatch } from '../scenario-helpers';

test.describe('chat email to the match players via Mailpit', () => {
  test.describe.configure({ retries: 0 });

  test('00 seed: one competition, one match, four players, SMTP -> Mailpit', async () => {
    const raw = loadCtx();
    const api: ScenarioApi = { baseURL: raw.baseURL, suToken: raw.suToken, adminCookie: raw.adminCookie };
    const m = await seedChatMatch(api, 'Chat por email', 'none');
    const ctx: ScenarioData = { ...raw, competitionId: m.competitionId, players: m.players, pairs: m.pairs, stage: 'blank' };
    saveCtx(ctx);
    printChatMatch(ctx.baseURL, m);
  });
});
