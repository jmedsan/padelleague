import { test, expect } from '../overflow-guard';
import { loadTestData } from '../helpers';
import {
  adminCookie, ChatSchedule, ScenarioApi, disableSmtp, enableSmtp, htmlOf, playerPost, seedChatMatch, startSmtpSink,
} from '../scenario-helpers';

// SMTP is a global setting of the shared server, so other specs' emails can
// reach the sink while it is on: assertions look only at mail to the four
// seeded players. The spec runs in the desktop project only (it is not in
// MOBILE_VIEWPORT_SPECS), so no second copy toggles SMTP at the same time.
// A chat message by p01 on a match of p01 + p02 against p03 + p04 (p04 is
// unverified), delivered to a local SMTP sink: who gets an email, and what
// the context card in it shows.
test.describe('chat email', { tag: '@notifications' }, () => {
  const text = 'Hola, ¿jugamos el sábado? '.repeat(8).trim(); // well past the 60-char in-app preview

  for (const [schedule, badge] of [['confirmed', 'Confirmada'], ['proposed', 'Propuesta'], ['none', '']] as const satisfies ReadonlyArray<readonly [ChatSchedule, string]>) {
    test(`a chat message emails exactly the verified rest of the match; card for ${schedule} date`, async ({ baseURL }) => {
      const api: ScenarioApi = { baseURL: baseURL!, suToken: loadTestData().adminToken, adminCookie: await adminCookie(baseURL!) };
      const m = await seedChatMatch(api, `Chat email ${schedule}`, schedule);
      const sink = await startSmtpSink();
      try {
        // Enabled only now, so the seeding's own notifications are not captured.
        await enableSmtp(api, sink.port);
        await playerPost(api, m.players[0].email, `/match/${m.matchID}/thread/message`, { content: text });
        const seeded = new Set(m.players.map((p) => p.email));
        const ours = () => sink.messages.filter((msg) => msg.to.some((a) => seeded.has(a)));
        await expect.poll(() => ours().length).toBeGreaterThanOrEqual(2);
        await new Promise((r) => setTimeout(r, 500)); // a stray third email would land here

        expect(ours().flatMap((msg) => msg.to).sort()).toEqual([m.players[1].email, m.players[2].email].sort());

        const html = htmlOf(ours()[0].data);
        expect(html).toContain(text); // the full message, not the 60-char in-app preview
        expect(html).toContain(m.competitionName);
        expect(html).toContain(m.pair1.name);
        expect(html).toContain(m.pair2.name);
        if (badge) {
          expect(html).toContain('Fecha y lugar');
          expect(html).toContain(badge);
          expect(html).toContain('Padel 360');
        } else {
          expect(html).not.toContain('Fecha y lugar');
        }
        if (schedule !== 'confirmed') expect(html).not.toContain('Confirmada');
        if (schedule !== 'proposed') expect(html).not.toContain('Propuesta');
      } finally {
        await disableSmtp(api);
        await sink.close();
      }
    });
  }
});
