import { defineConfig, devices } from '@playwright/test'
import { dirname, resolve } from 'node:path'
import process from 'node:process'
import { fileURLToPath } from 'node:url'

/**
 * Read environment variables from file.
 * https://github.com/motdotla/dotenv
 */
// import dotenv from 'dotenv';
// import path from 'path';
// dotenv.config({ path: path.resolve(__dirname, '.env') });

const CONFIG_DIR = dirname(fileURLToPath(import.meta.url))
const BASE_DIR = resolve(CONFIG_DIR, '../e2e')
const OUTPUT_DIR = resolve(BASE_DIR, '.output')
const E2E_PORT = parsePort(process.env.E2E_PORT)
const E2E_BASE_URL = `http://localhost:${E2E_PORT}`
const E2E_REPORT_DIR = process.env.E2E_REPORT_DIR || resolve(OUTPUT_DIR, 'html-report')
const E2E_WORKERS = parseWorkers(process.env.E2E_WORKERS)

/**
 * See https://playwright.dev/docs/test-configuration.
 */
export default defineConfig({
  testDir: BASE_DIR,
  /* output directory for test artifacts. */
  outputDir: resolve(OUTPUT_DIR, 'test-results'),
  /* Run tests in files in parallel */
  fullyParallel: true,
  /* Fail the build on CI if you accidentally left test.only in the source code. */
  forbidOnly: !!process.env.CI,
  /* Stability gates never retry a failed scenario. */
  retries: 0,
  /* CI is serialized; local runs may opt into an explicit worker count. */
  workers: process.env.CI ? 1 : E2E_WORKERS,
  /* Reporter to use. See https://playwright.dev/docs/test-reporters */
  reporter: [
    ['html', { open: 'never', outputFolder: E2E_REPORT_DIR }],
  ],
  /* Shared settings for all the projects below. See https://playwright.dev/docs/api/class-testoptions. */
  use: {
    /* Base URL to use in actions like `await page.goto('/')`. */
    baseURL: E2E_BASE_URL,
    /* Collect trace when retrying the failed test. See https://playwright.dev/docs/trace-viewer */
    trace: 'retain-on-first-failure',
    /* Collect video when retrying the failed test. See https://playwright.dev/docs/screenshots */
    screenshot: 'only-on-failure',
    /* Collect video when retrying the failed test. See https://playwright.dev/docs/videos */
    // video: 'retain-on-failure',
  },

  /* Configure projects for major browsers */
  projects: [
    {
      name: 'chromium',
      testIgnore: /oidc-integration\.spec\.ts/,
      use: { ...devices['Desktop Chrome'] },
    },

    {
      name: 'firefox-enterprise',
      testMatch: /enterprise-matrix\.spec\.ts/,
      use: { ...devices['Desktop Firefox'] },
    },
    {
      name: 'webkit-enterprise',
      testMatch: /enterprise-matrix\.spec\.ts/,
      use: { ...devices['Desktop Safari'] },
    },

    /* Test against mobile viewports. */
    {
      name: 'mobile-chromium-enterprise',
      testMatch: /enterprise-matrix\.spec\.ts/,
      use: { ...devices['Pixel 5'] },
    },

    /* Test against branded browsers. */
    // {
    //   name: 'Microsoft Edge',
    //   use: { ...devices['Desktop Edge'], channel: 'msedge' },
    // },
    // {
    //   name: 'Google Chrome',
    //   use: { ...devices['Desktop Chrome'], channel: 'chrome' },
    // },
  ],

  /* Run your local dev server before starting the tests */
  webServer: {
    command: `yarn dev --port ${E2E_PORT} --strictPort`,
    url: E2E_BASE_URL,
    env: {
      VITE_APP_TITLE: 'MineAdmin',
      VITE_APP_PORT: String(E2E_PORT),
      VITE_APP_ROOT_BASE: '/',
      VITE_APP_API_BASEURL: '/dev',
      VITE_APP_ROUTE_MODE: 'hash',
      VITE_APP_STORAGE_PREFIX: 'mine_',
      VITE_OPEN_PROXY: 'false',
      VITE_PROXY_PREFIX: '/dev',
      VITE_SECURITY_CSRF: 'true',
      VITE_OPEN_DEVTOOLS: 'false',
    },
    reuseExistingServer: false,
  },
})

function parseWorkers(value: string | undefined) {
  if (!value) {
    return undefined
  }
  const workers = Number.parseInt(value, 10)
  if (!Number.isSafeInteger(workers) || workers < 1) {
    throw new Error('E2E_WORKERS must be a positive integer')
  }
  return workers
}

function parsePort(value: string | undefined) {
  if (!value) {
    return 2889
  }
  const port = Number.parseInt(value, 10)
  if (!Number.isSafeInteger(port) || port < 1024 || port > 65535) {
    throw new Error('E2E_PORT must be a valid non-privileged TCP port')
  }
  return port
}
