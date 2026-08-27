import { Page, expect, APIRequestContext } from '@playwright/test';
import { ExpectedRow, PairId } from './season-helpers';

// ---------------------------------------------------------------------------
// referenceFallback — tracks non-affordance navigations for the guided spec
// ---------------------------------------------------------------------------

interface Fallback {
  reason: string;
  url: string;
}

const fallbacks: Fallback[] = [];

export async function referenceFallback(page: Page, reason: string, url: string): Promise<void> {
  fallbacks.push({ reason, url });
  await page.goto(url);
}

export function collectFallbacks(): Fallback[] {
  return [...fallbacks];
}

export function resetFallbacks(): void {
  fallbacks.length = 0;
}

export function assertFallbacksMatch(actual: Fallback[], expected: string[]): void {
  const reasons = actual.map(f => f.reason);
  expect(reasons.sort()).toEqual([...expected].sort());
}

// ---------------------------------------------------------------------------
// HTMX redirect helper (shared by action helpers)
// ---------------------------------------------------------------------------

export async function clickAndWaitForHxRedirect(page: Page, locator: ReturnType<Page['locator']>): Promise<void> {
  const navPromise = page.waitForEvent('framenavigated', { timeout: 15000 });
  await locator.click();
  await navPromise;
  await page.waitForLoadState('domcontentloaded');
}

// ---------------------------------------------------------------------------
// Action helpers — assume the page is already at the right place
// ---------------------------------------------------------------------------

export async function createPlayer(page: Page, email: string, displayName: string): Promise<void> {
  await page.locator('label[for="precreate-modal"]').first().click();
  const modal = page.locator('.modal[role="dialog"]').filter({ hasText: 'Pre-crear usuario' });
  await modal.locator('input[name="email"]').fill(email);
  await modal.locator('input[name="display_name"]').fill(displayName);
  const responsePromise = page.waitForResponse(
    resp => resp.url().includes('/admin/players/pre-create'),
  );
  await modal.locator('button[type="submit"]').click();
  const createResp = await responsePromise;
  if (createResp.status() >= 300) {
    throw new Error(`Pre-create failed: ${createResp.status()} for ${email}`);
  }
  await expect(page.getByText('Usuario creado').first()).toBeVisible({ timeout: 5000 });
}

export async function createCompetition(
  page: Page,
  name: string,
  type: 'league' | 'playoff',
  options?: { playTwice?: boolean; suToken?: string },
): Promise<string> {
  await page.getByRole('button', { name: /crear competición/i }).first().click();
  const dialog = page.locator('dialog#modal-create');
  await dialog.locator('input[name="name"]').fill(name);
  await dialog.locator('select[name="type"]').selectOption(type);
  if (options?.playTwice) {
    await dialog.locator('input[name="play_twice"]').check();
  }
  await dialog.locator('input[name="active"]').check();
  await clickAndWaitForHxRedirect(page, dialog.locator('button[type="submit"]'));
  const match = page.url().match(/\/admin\/competitions\/([^/]+)/);
  if (match) return match[1];
  const headers: Record<string, string> = {};
  if (options?.suToken) headers['Authorization'] = options.suToken;
  const resp = await page.request.get(
    `/api/collections/competitions/records?filter=name='${name}'&perPage=1`,
    { headers },
  );
  const data = await resp.json();
  const id = data.items?.[0]?.id;
  if (!id) throw new Error(`Competition not found after create: ${name}`);
  return id;
}

export async function createPair(
  page: Page,
  name: string,
  player1Id: string,
  player2Id: string,
  suToken: string,
): Promise<string> {
  await page.evaluate(() => {
    (document.getElementById('modal-create') as HTMLDialogElement)?.showModal();
  });
  const dialog = page.locator('dialog#modal-create');
  await dialog.locator('input[name="name"]').fill(name);
  await dialog.locator('select[name="player1"]').selectOption(player1Id);
  await dialog.locator('select[name="player2"]').selectOption(player2Id);
  await clickAndWaitForHxRedirect(page, dialog.locator('button[type="submit"]'));
  const resp = await page.request.get(`/api/collections/pairs/records?filter=name='${name}'`, {
    headers: { Authorization: suToken },
  });
  const data = await resp.json();
  const id = data.items?.[0]?.id;
  if (!id) throw new Error(`Failed to find created pair: ${name}`);
  return id;
}

export async function addPairToCompetition(page: Page, pairId: string, seed?: number): Promise<void> {
  await page.selectOption('select[name="pair"]', pairId);
  if (seed !== undefined) {
    await page.fill('input[name="seed"]', String(seed));
  }
  await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Añadir")'));
}

export async function generateFixtures(page: Page): Promise<void> {
  await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Generar calendario")'));
}

export async function setDates(page: Page, startDate: string, endDate: string): Promise<void> {
  await page.fill('input[name="start_date"]', startDate);
  await page.fill('input[name="end_date"]', endDate);
  await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Guardar fechas")'));
}

export async function activate(page: Page): Promise<void> {
  await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Activar")'));
}

// markAllPairsPaid marks every pair in the current competition as paid via the
// "Marcar todos como pagado" control on the competition detail page. Reloads
// first so the payment section reflects the pairs just added over HTMX.
export async function markAllPairsPaid(page: Page): Promise<void> {
  await page.reload();
  await page.waitForLoadState('domcontentloaded');
  const btn = page.getByRole('button', { name: /marcar todos como pagado/i });
  if (await btn.count() === 0) return;
  await Promise.all([
    page.waitForResponse((r) => r.url().includes('/payment-all')),
    btn.first().click(),
  ]);
  await expect(btn).toHaveCount(0);
}

export async function submitScore(page: Page, score: string): Promise<void> {
  const sets = score.split(/\s+/).map(s => s.split('-'));
  for (const [f, v] of [['s1a', sets[0][0]], ['s1b', sets[0][1]], ['s2a', sets[1][0]], ['s2b', sets[1][1]]]) {
    await page.$eval(`input[name="${f}"]`, (el, val) => { (el as HTMLInputElement).value = val; }, v);
  }
  if (sets.length === 3) {
    await page.evaluate(() => {
      for (const id of ['set3-header', 's3a-cell', 's3b-cell']) {
        const el = document.getElementById(id);
        if (el) el.style.display = '';
      }
    });
    for (const [f, v] of [['s3a', sets[2][0]], ['s3b', sets[2][1]]]) {
      await page.$eval(`input[name="${f}"]`, (el, val) => { (el as HTMLInputElement).value = val; }, v);
    }
  }
  await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Enviar resultado")'));
}

export async function confirmScore(page: Page): Promise<void> {
  await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Confirmar")'));
}

export async function disputeScore(page: Page): Promise<void> {
  await page.locator('button:has-text("Disputar")').click();
  // Provide the disputer's own claimed score so the admin sees both versions.
  await page.locator('input[name="disputed_scores"]').fill('6-4 4-6 5-7');
  await page.locator('textarea[name="dispute_notes"]').fill('El tercer set fue 5-7, no 7-5.');
  await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Enviar disputa")'));
}

export async function resolveDispute(page: Page, matchId: string, score: string): Promise<void> {
  const row = page.locator(`form[hx-post*="/admin/disputes/${matchId}/resolve"]`).first();
  await row.locator('input[name="score"]').fill(score);
  await clickAndWaitForHxRedirect(page, row.locator('button:has-text("Resolver")'));
}

export async function approveWalkover(page: Page, matchId: string): Promise<void> {
  const row = page.locator(`form[hx-post*="/admin/disputes/${matchId}/walkover-approve"]`).first();
  await clickAndWaitForHxRedirect(page, row.locator('button:has-text("Aprobar")'));
}

export async function setPlayoffDate(page: Page, matchId: string, date: string): Promise<void> {
  const row = page.locator(`tr:has(a[href="/match/${matchId}"])`).first();
  await row.locator('input[name="date"]').fill(date);
  await clickAndWaitForHxRedirect(page, row.locator('button:has-text("Guardar")'));
}

// ---------------------------------------------------------------------------
// Document helpers
// ---------------------------------------------------------------------------

export async function createDocument(
  page: Page,
  title: string,
  url: string,
  options?: { mandatory?: boolean; isDefault?: boolean },
): Promise<void> {
  await page.locator('button:has-text("Nuevo documento")').click();
  const dialog = page.locator('dialog#modal-create-doc');
  await dialog.locator('input[name="title"]').fill(title);
  await dialog.locator('input[name="url"]').fill(url);
  if (options?.mandatory) {
    await dialog.locator('input[name="is_mandatory"]').check();
  }
  if (options?.isDefault) {
    await dialog.locator('input[name="is_default"]').check();
  }
  await clickAndWaitForHxRedirect(page, dialog.locator('button[type="submit"]'));
}

export async function attachDocumentToCompetition(
  page: Page,
  docTitle: string,
): Promise<void> {
  const select = page.locator('select[name="document"]');
  await select.selectOption({ label: docTitle });
  await clickAndWaitForHxRedirect(page, page.locator('button:has-text("Adjuntar")'));
}

export async function acceptDocsGate(page: Page): Promise<void> {
  await expect(page.getByRole('heading', { name: 'Documentos obligatorios' })).toBeVisible({ timeout: 5000 });
  const mandatoryLinks = page.locator('[data-mandatory-id]');
  const count = await mandatoryLinks.count();
  for (let i = 0; i < count; i++) {
    await mandatoryLinks.nth(i).click();
  }
  await clickAndWaitForHxRedirect(page, page.locator('#accept-btn'));
}

// ---------------------------------------------------------------------------
// Assertion helpers
// ---------------------------------------------------------------------------

export async function assertFinalStandings(
  page: Page,
  compId: string,
  pairNames: Record<PairId, string>,
  expected: ExpectedRow[],
  hasPenalties: boolean,
): Promise<void> {
  await page.goto(`/competition/${compId}`);
  await page.locator('input[aria-label="Clasificación"]').click();
  await page.waitForSelector('table.table-zebra tbody tr', { timeout: 5000 });

  const rows = page.locator('table.table-zebra tbody tr');
  const count = await rows.count();
  expect(count).toBe(expected.length);

  for (let i = 0; i < expected.length; i++) {
    const row = rows.nth(i);
    const cells = row.locator('td');
    const exp = expected[i];
    const name = pairNames[exp.pair];

    await expect(cells.nth(0)).toContainText(String(exp.position));
    await expect(cells.nth(1)).toContainText(name);
    await expect(cells.nth(2)).toContainText(String(exp.played));
    await expect(cells.nth(3)).toContainText(String(exp.wins));
    await expect(cells.nth(4)).toContainText(String(exp.losses));
    await expect(cells.nth(5)).toContainText(`${exp.setsWon}/${exp.setsLost}`);
    await expect(cells.nth(6)).toContainText(`${exp.gamesWon}/${exp.gamesLost}`);
    await expect(cells.nth(7)).toContainText(String(exp.points));

    if (hasPenalties && exp.penalty > 0) {
      await expect(cells.nth(8)).toContainText(`-${exp.penalty}`);
    }
  }
}

export async function assertPlayoffChampion(
  page: Page,
  playoffCompId: string,
  expectedPairName: string,
): Promise<void> {
  await page.goto(`/competition/${playoffCompId}`);
  // The bracket's last round column contains the final match.
  // The winner has `font-bold text-accent` styling.
  const bracketColumns = page.locator('.flex.gap-4 > .flex.flex-col');
  const lastColumn = bracketColumns.last();
  const winner = lastColumn.locator('.font-bold.text-accent').first();
  await expect(winner).toContainText(expectedPairName);
}

// ---------------------------------------------------------------------------
// API helpers (superuser-level, used during setup)
// ---------------------------------------------------------------------------

export async function lookupPlayerId(
  request: APIRequestContext,
  suToken: string,
  email: string,
): Promise<string> {
  const resp = await request.get(
    `/api/collections/users/records?filter=email='${email}'`,
    { headers: { Authorization: suToken } },
  );
  const data = await resp.json();
  const id = data.items?.[0]?.id;
  if (!id) throw new Error(`Player not found: ${email}`);
  return id;
}

export async function getRoundMatches(
  request: APIRequestContext,
  suToken: string,
  compId: string,
  round: number,
): Promise<any[]> {
  const resp = await request.get(
    `/api/collections/matches/records?filter=competition='${compId}'&sort=created&perPage=50`,
    { headers: { Authorization: suToken } },
  );
  const items = (await resp.json()).items;
  return items.filter((m: any) => Number(m.round_number) === round);
}

export async function getMatchById(
  request: APIRequestContext,
  suToken: string,
  id: string,
): Promise<any> {
  const resp = await request.get(
    `/api/collections/matches/records/${id}`,
    { headers: { Authorization: suToken } },
  );
  return await resp.json();
}
