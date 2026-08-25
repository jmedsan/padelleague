import { test, expect } from '@playwright/test';
import { loginAs, scratchMatchId, loadTestData, PLAYER1_EMAIL, PLAYER1_PASSWORD } from '../helpers';

test.describe('match thread', () => {
  test('player can view match thread', async ({ page }) => {
    const data = loadTestData();
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/match/${data.matchIds[0]}`);
    await expect(page.getByText('Hilo del partido')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('[name="content"]')).toBeVisible({ timeout: 10000 });
  });

  test('player can post a message', async ({ page }) => {
    const data = loadTestData();
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    const msg = `E2E msg ${Date.now()}`;
    const resp = await page.request.post(`/match/${data.matchIds[0]}/thread/message`, {
      form: { content: msg },
    });
    expect(resp.status()).toBeLessThan(400);
    await page.goto(`/match/${data.matchIds[0]}`);
    await page.waitForFunction(
      (text) => document.body.innerText.includes(text),
      msg,
      { timeout: 15000 }
    );
  });

  test('player can propose a schedule', async ({ page }, testInfo) => {
    // Needs a match still pending: the proposal form disappears once a score
    // is submitted, and another test submits one to the shared match.
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/match/${scratchMatchId("propose-schedule", testInfo.project.name)}`);
    const collapseTitle = page.locator('.collapse-title', { hasText: /proponer fecha/i });
    await expect(collapseTitle).toBeVisible({ timeout: 10000 });
    await collapseTitle.locator('..').locator('input[type="checkbox"]').click();
    await expect(page.locator('input[type="date"]')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('input[type="time"]')).toBeVisible({ timeout: 3000 });
  });
});
