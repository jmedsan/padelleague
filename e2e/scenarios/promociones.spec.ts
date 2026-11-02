import { readFileSync } from 'fs';
import { join } from 'path';
import { expect, test } from '@playwright/test';
import { ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';
import {
  PLAYER_PASSWORD, ScenarioApi, ScenarioData, formPost, loadCtx, printLinks, saveCtx, seedChatMatch,
} from '../scenario-helpers';

interface Offer {
  title: string;
  description: string;
  url: string;
  image: boolean;
  expiresInDays: number | null;
  active: boolean;
}

const DAY_MS = 24 * 60 * 60 * 1000;

const OFFERS: Offer[] = [
  {
    title: '20 % en palas de pádel',
    description: 'Hasta un 20 % de descuento en palas de pádel seleccionadas.',
    url: 'https://www.decathlon.es/es/browse/c0-deportes/c1-padel/_/N-1g6yy3s',
    image: true, expiresInDays: null, active: true,
  },
  {
    title: 'Pista gratis entre semana',
    description: 'Reserva una pista de lunes a jueves y la segunda hora es gratis.',
    url: 'https://www.padelnuestro.com/',
    image: false, expiresInDays: 60, active: true,
  },
  {
    title: 'Clase de prueba con monitor',
    description: 'Una clase de prueba gratuita con un monitor titulado.',
    url: 'https://www.playtomic.com/',
    image: true, expiresInDays: null, active: false,
  },
  {
    title: 'Oferta de verano en zapatillas',
    description: 'Zapatillas de pádel con descuento de fin de temporada.',
    url: 'https://www.bullpadel.com/',
    image: false, expiresInDays: -7, active: true,
  },
];

const SHOWN = OFFERS.filter(o => o.active && (o.expiresInDays === null || o.expiresInDays > 0));
const HIDDEN = OFFERS.filter(o => !SHOWN.includes(o));

function isoDay(offsetDays: number): string {
  return new Date(Date.now() + offsetDays * DAY_MS).toISOString().slice(0, 10);
}

// Creates an offer the way the admin form does: multipart POST to /admin/promotions.
async function createOffer(api: ScenarioApi, o: Offer): Promise<void> {
  const form = new FormData();
  form.set('title', o.title);
  form.set('description', o.description);
  form.set('url', o.url);
  form.set('expires_on', o.expiresInDays === null ? '' : isoDay(o.expiresInDays));
  if (o.image) {
    const png = readFileSync(join(__dirname, '../../static/img/sample-sponsors/decathlon.png'));
    form.set('image', new Blob([png], { type: 'image/png' }), 'decathlon.png');
  }
  const resp = await fetch(`${api.baseURL}/admin/promotions`, {
    method: 'POST',
    headers: { Cookie: api.adminCookie, 'HX-Request': 'true' },
    body: form,
    redirect: 'manual',
  });
  if (resp.status >= 400) throw new Error(`create "${o.title}": ${resp.status} ${await resp.text()}`);
  const body = await resp.text();
  if (body.includes('alert-error')) throw new Error(`create "${o.title}" refused: ${body}`);
}

async function offerIds(api: ScenarioApi): Promise<Map<string, string>> {
  const resp = await fetch(`${api.baseURL}/admin/promotions`, { headers: { Cookie: api.adminCookie } });
  const html = await resp.text();
  const ids = new Map<string, string>();
  for (const o of OFFERS) {
    const at = html.indexOf(o.title);
    const before = html.lastIndexOf('data-promo-id="', at);
    if (at < 0 || before < 0) throw new Error(`offer "${o.title}" not listed on /admin/promotions`);
    ids.set(o.title, html.slice(before + 15, html.indexOf('"', before + 15)));
  }
  return ids;
}

test.describe('Promociones: four offers (active, no image, paused, expired)', () => {
  test.describe.configure({ retries: 0 });

  let ctx: ScenarioData;

  test('00 seed: competition, players and the four offers', async () => {
    const raw = loadCtx();
    const api: ScenarioApi = { baseURL: raw.baseURL, suToken: raw.suToken, adminCookie: raw.adminCookie };
    const m = await seedChatMatch(api, 'Promociones', 'none');
    ctx = { ...raw, competitionId: m.competitionId, players: m.players, pairs: m.pairs, stage: 'blank' };
    saveCtx(ctx);

    for (const o of OFFERS) await createOffer(api, o);
    const ids = await offerIds(api);
    for (const o of OFFERS.filter(x => !x.active)) await formPost(api, `/admin/promotions/${ids.get(o.title)}/toggle`);

    const player = m.players[0].email;
    console.log('\nPromociones scenario — one link per offer state:');
    printLinks(ctx.baseURL, [
      { label: 'Activa con imagen, sin caducidad (Inicio)', email: player, password: PLAYER_PASSWORD, path: '/' },
      { label: 'Activa sin imagen, caduca en 60 días (Promociones)', email: player, password: PLAYER_PASSWORD, path: '/promotions' },
      { label: 'Activa con imagen (página de la competición)', email: player, password: PLAYER_PASSWORD, path: `/competition/${ctx.competitionId}` },
      { label: 'Pausada y expirada (admin, lista con Inactiva y Expirada)', email: ADMIN_EMAIL, password: ADMIN_PASSWORD, path: '/admin/promotions' },
    ]);
  });

  test('01 players see only the live offers; the admin sees all four with their state', async ({ page }) => {
    ctx = loadCtx();
    const signIn = async (email: string, password: string, next: string) => {
      await page.goto(`/dev-login?${new URLSearchParams({ email, password, next })}`);
    };
    const shownTitles = SHOWN.map(o => o.title);
    const hiddenTitles = HIDDEN.map(o => o.title);

    await signIn(ctx.players[0].email, PLAYER_PASSWORD, '/');
    for (const path of ['/', `/competition/${ctx.competitionId}`]) {
      const seen = new Set<string>();
      for (let i = 0; i < 20; i++) {
        await page.goto(path);
        const card = page.getByTestId('promo-card');
        await expect(card).toHaveCount(1);
        const text = (await card.innerText()).trim();
        const title = shownTitles.find(t => text.includes(t));
        expect(title, `${path} showed an offer that must stay hidden: ${text}`).toBeTruthy();
        for (const h of hiddenTitles) expect(text).not.toContain(h);
        seen.add(title!);
      }
      expect([...seen].sort(), `${path} rotation over 20 loads`).toEqual([...shownTitles].sort());
    }

    await page.goto('/promotions');
    await expect(page.getByTestId('promo-card')).toHaveCount(SHOWN.length);
    for (const t of shownTitles) await expect(page.getByTestId('promo-card').filter({ hasText: t })).toHaveCount(1);
    for (const t of hiddenTitles) await expect(page.getByText(t)).toHaveCount(0);

    await signIn(ADMIN_EMAIL, ADMIN_PASSWORD, '/admin/promotions');
    const rows = page.getByTestId('promotion-row');
    await expect(rows).toHaveCount(OFFERS.length);
    for (const o of OFFERS) {
      const row = rows.filter({ hasText: o.title });
      await expect(row).toHaveCount(1);
      const paused = !o.active;
      const expired = o.expiresInDays !== null && o.expiresInDays < 0;
      await expect(row.getByText('Inactiva')).toHaveCount(paused ? 1 : 0);
      await expect(row.getByText('Expirada')).toHaveCount(expired ? 1 : 0);
      await expect(row.getByText('Activa', { exact: true })).toHaveCount(!paused && !expired ? 1 : 0);
    }
  });
});
