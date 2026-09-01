import { test, expect } from '@playwright/test';
import { loginAs, scratchMatchId, loadTestData, PLAYER1_EMAIL, PLAYER1_PASSWORD, ADMIN_EMAIL, ADMIN_PASSWORD } from '../helpers';

test.describe('match thread', () => {
  test('player can view match thread', async ({ page }) => {
    const data = loadTestData();
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/match/${data.matchIds[0]}`);
    await expect(page.getByRole('heading', { name: 'Hilo del partido' })).toBeVisible({ timeout: 10000 });
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
    await loginAs(page, PLAYER1_EMAIL, PLAYER1_PASSWORD);
    await page.goto(`/match/${scratchMatchId("propose-schedule", testInfo.project.name)}`);
    // When no date+place is set, the scheduling form appears as a prominent card
    const scheduleHeading = page.getByText('Proponer fecha y lugar');
    await expect(scheduleHeading).toBeVisible({ timeout: 10000 });
    await expect(page.locator('input[type="date"]')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('input[type="time"]')).toBeVisible({ timeout: 3000 });
  });

  test('admin non-participant can post in thread', async ({ page }) => {
    const data = loadTestData();
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    const msg = `Admin msg ${Date.now()}`;
    const resp = await page.request.post(`/match/${data.matchIds[0]}/thread/message`, {
      form: { content: msg, type: 'chat' },
    });
    expect(resp.status()).toBeLessThan(400);
    await page.goto(`/match/${data.matchIds[0]}`);
    await page.waitForFunction(
      (text) => document.body.innerText.includes(text),
      msg,
      { timeout: 15000 }
    );
    await expect(page.locator('[name="content"]')).toBeVisible({ timeout: 5000 });
  });
});
