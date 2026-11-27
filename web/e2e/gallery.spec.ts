import { expect, test } from '@playwright/test';

const SERVER_URL = 'http://127.0.0.1:8787';

test('login, browse folders, sort, favorite, video player +30s/-30s, download, and PWA SW reload', async ({
  page,
  context,
}) => {
  await page.goto('/');
  await expect(page).toHaveURL(/#\/login$/);

  // Login against the mock server.
  await page.getByLabel('Server URL').fill(SERVER_URL);
  await page.getByLabel('Username').fill('demo');
  await page.getByLabel('Password').fill('demo');
  await page.getByRole('button', { name: 'Sign in' }).click();

  await expect(page).toHaveURL(/#\/folders\/root$/);
  await expect(page.getByText('Mock NAS')).toBeVisible();

  // Browse folders: All media -> Photos -> 2025.
  await page.locator('.folder-tile', { hasText: 'Photos' }).click();
  await expect(page).toHaveURL(/#\/folders\/f_photos$/);
  await page.locator('.folder-tile', { hasText: '2025' }).click();
  await expect(page).toHaveURL(/#\/folders\/f_2025$/);
  await expect(page.getByText('130 items')).toBeVisible();

  // Change sort to Name ascending and confirm first tile updates.
  await page.locator('#toolbar-sort').selectOption('name');
  await page.getByRole('button', { name: 'Sort ascending' }).click();
  await expect(page.locator('.tile').first()).toHaveAttribute('aria-label', /IMG_20250101_0001\.jpg/);

  // Toggle favorite on the first item, then verify in Favorites view.
  const firstFavBtn = page.locator('.tile-fav').first();
  await expect(firstFavBtn).toHaveAttribute('aria-pressed', 'false');
  await firstFavBtn.click();
  await expect(firstFavBtn).toHaveAttribute('aria-pressed', 'true');

  await page.getByRole('button', { name: 'Favorites' }).first().click();
  await expect(page).toHaveURL(/#\/favorites$/);
  await expect(page.locator('.tile[aria-label*="IMG_20250101_0001.jpg"]')).toBeVisible();

  // Navigate to Videos via sidebar tree and open sunset.mp4.
  await page.locator('.tree-label', { hasText: 'Videos' }).click();
  await expect(page).toHaveURL(/#\/folders\/f_videos$/);
  await page.locator('.tile[aria-label*="sunset.mp4"]').click();

  const dialog = page.getByRole('dialog');
  await expect(dialog).toBeVisible();
  const video = dialog.locator('video');
  await expect(video).toBeVisible();

  // Wait for video metadata (40s fixture) so seeking is clamped to real duration.
  await expect
    .poll(async () => video.evaluate((v: HTMLVideoElement) => v.duration))
    .toBeGreaterThan(35);

  // Play, jump +30s, verify time >= 29s, then jump -30s.
  await dialog.getByRole('button', { name: 'Play', exact: true }).click();
  await dialog.getByRole('button', { name: 'Forward 30 seconds' }).click();
  await expect
    .poll(async () => video.evaluate((v: HTMLVideoElement) => v.currentTime))
    .toBeGreaterThanOrEqual(29);

  await dialog.getByRole('button', { name: 'Back 30 seconds' }).click();
  await expect
    .poll(async () => video.evaluate((v: HTMLVideoElement) => v.currentTime))
    .toBeLessThan(5);

  // Verify download link has signed streamUrl + download=1 and responds 200 with attachment header.
  const dlLink = dialog.locator('a.lightbox-download');
  const href = await dlLink.getAttribute('href');
  expect(href).toMatch(/\/api\/media\/m_\d+\/stream\?.*download=1/);
  const dlResp = await page.request.get(href!);
  expect(dlResp.status()).toBe(200);
  expect(dlResp.headers()['content-disposition']).toContain('attachment');

  // Switch to broken.avi and verify fallback UI appears.
  await dialog.getByRole('button', { name: 'Next item' }).click();
  await dialog.getByRole('button', { name: 'Next item' }).click();
  await expect(dialog.getByText('Playback failed')).toBeVisible({ timeout: 10_000 });
  await expect(dialog.getByRole('link', { name: 'Download to watch' })).toBeVisible();

  await dialog.getByRole('button', { name: 'Close viewer' }).click();
  await expect(dialog).toBeHidden();

  // Confirm PWA service worker activates, controls the page after reload, and
  // serves the app shell while offline.
  await expect
    .poll(
      async () =>
        page.evaluate(async () => {
          if (!('serviceWorker' in navigator)) return false;
          const reg = await navigator.serviceWorker.ready;
          return Boolean(reg.active);
        }),
      { timeout: 10_000 },
    )
    .toBe(true);

  await page.reload();
  await expect
    .poll(() => page.evaluate(() => Boolean(navigator.serviceWorker.controller)), { timeout: 10_000 })
    .toBe(true);

  await context.setOffline(true);
  await page.reload();
  await expect(page.getByText('NAS Gallery')).toBeVisible();
  await context.setOffline(false);
});
