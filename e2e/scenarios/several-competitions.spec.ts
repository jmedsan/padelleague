import { test, expect } from '@playwright/test';
import { ScenarioApi, apiGet, formPostData, loadCtx, uniqueSuffix } from '../scenario-helpers';

// Run on a binary WITHOUT the 24h migration, then `make scenario-migrate`:
// the two 48h competitions become 24h, the 12h one stays.
test.describe('several competitions', () => {
  test.describe.configure({ retries: 0 });

  test('00 three competitions: 12h, 48h, 48h', async () => {
    const ctx = loadCtx();
    const api: ScenarioApi = { baseURL: ctx.baseURL, suToken: ctx.suToken, adminCookie: ctx.adminCookie };
    const suffix = uniqueSuffix();
    for (const [name, hours] of [[`Quorum12-${suffix}`, '12'], [`Quorum48a-${suffix}`, '48'], [`Quorum48b-${suffix}`, '48']]) {
      await formPostData(api, '/admin/competitions', { name, type: 'league', active: 'on', quorum_timeout_hours: hours });
    }
    const list = await apiGet(api, `/api/collections/competitions/records?filter=${encodeURIComponent(
      `name ~ '${suffix}'`)}&sort=name&perPage=10`);
    expect(list.items.map((c: any) => [c.name, c.quorum_timeout_hours])).toEqual([
      [`Quorum12-${suffix}`, 12], [`Quorum48a-${suffix}`, 48], [`Quorum48b-${suffix}`, 48],
    ]);
    console.log(`\nAdmin competitions: ${ctx.baseURL}/admin/competitions`);
    console.log(`Check after migrating: ${ctx.baseURL}/api/collections/competitions/records?filter=${encodeURIComponent(`name ~ '${suffix}'`)}`);
  });
});
