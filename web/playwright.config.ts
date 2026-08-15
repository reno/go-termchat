import { defineConfig, devices } from "@playwright/test";

const PORT = 18099;

export default defineConfig({
  testDir: "../e2e",
  timeout: 30_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI === undefined ? 0 : 1,
  reporter: process.env.CI === undefined ? [["list"]] : [["list"], ["github"]],
  use: {
    baseURL: `http://127.0.0.1:${String(PORT)}`,
    trace: "retain-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: {
    command: `../bin/server`,
    url: `http://127.0.0.1:${String(PORT)}/healthz`,
    reuseExistingServer: false,
    stdout: "pipe",
    stderr: "pipe",
    env: {
      TERMCHAT_ADDR: `127.0.0.1:${String(PORT)}`,
      TERMCHAT_ENV: "development",
      TERMCHAT_MESSAGES_PER_CHANNEL: "10",
      TERMCHAT_SEND_BURST: "200",
    },
  },
});
