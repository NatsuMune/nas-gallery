import { defineConfig, devices } from '@playwright/test';

const isCI = Boolean((globalThis as { process?: { env?: Record<string, string | undefined> } }).process?.env?.CI);

export default defineConfig({
  testDir: './e2e',
  testMatch: '**/*.spec.ts',
  fullyParallel: false,
  forbidOnly: isCI,
  retries: isCI ? 1 : 0,
  workers: 1,
  reporter: 'list',
  use: {
    baseURL: 'http://127.0.0.1:4173',
    trace: 'on-first-retry',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: [
    {
      command: 'node e2e/mock/server.mjs',
      url: 'http://127.0.0.1:8787/api/health',
      env: { PORT: '8787' },
      reuseExistingServer: !isCI,
      timeout: 15_000,
    },
    {
      command: 'npm run preview -- --port 4173 --host 127.0.0.1 --strictPort',
      url: 'http://127.0.0.1:4173/',
      reuseExistingServer: !isCI,
      timeout: 30_000,
    },
  ],
});
