import { defineConfig, devices } from '@playwright/test'
import { dirname, resolve } from 'node:path'
import process from 'node:process'
import { fileURLToPath } from 'node:url'

const CONFIG_DIR = dirname(fileURLToPath(import.meta.url))
const BASE_DIR = resolve(CONFIG_DIR, '../e2e')
const OUTPUT_DIR = resolve(BASE_DIR, '.output/oidc-integration')
const E2E_PORT = parsePort(process.env.OIDC_E2E_PORT, 2889)

export default defineConfig({
  testDir: BASE_DIR,
  testMatch: /oidc-integration\.spec\.ts/,
  outputDir: resolve(OUTPUT_DIR, 'test-results'),
  fullyParallel: false,
  forbidOnly: true,
  retries: 0,
  workers: 1,
  timeout: 45_000,
  reporter: [
    ['html', { open: 'never', outputFolder: resolve(OUTPUT_DIR, 'html-report') }],
    ['json', { outputFile: resolve(OUTPUT_DIR, 'results.json') }],
  ],
  use: {
    baseURL: `http://127.0.0.1:${E2E_PORT}`,
    trace: 'retain-on-first-failure',
    screenshot: 'only-on-failure',
  },
  projects: [{ name: 'chromium-oidc', use: { ...devices['Desktop Chrome'] } }],
  webServer: {
    command: `yarn dev --port ${E2E_PORT} --strictPort`,
    url: `http://127.0.0.1:${E2E_PORT}`,
    reuseExistingServer: false,
  },
})

function parsePort(value: string | undefined, fallback: number) {
  if (!value) {
    return fallback
  }
  const port = Number.parseInt(value, 10)
  if (!Number.isSafeInteger(port) || port < 1024 || port > 65535) {
    throw new Error('OIDC_E2E_PORT must be a valid non-privileged TCP port')
  }
  return port
}
