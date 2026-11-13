import { defineConfig, devices } from "@playwright/test";

const PORT = 8090;

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: [["list"], ["html", { open: "never" }]],
  // Three browser engines share the CPU and one server, so 5 s waits sometimes ran out on a healthy run.
  expect: { timeout: 10_000 },
  use: {
    baseURL: `http://localhost:${PORT}`,
    locale: "pt-BR",
    timezoneId: "America/Sao_Paulo",
    trace: "retain-on-failure",
  },
  projects: [
    { name: "iphone-webkit", use: { ...devices["iPhone 15"] } },
    { name: "desktop-webkit", use: { ...devices["Desktop Safari"] } },
    { name: "desktop-chromium", use: { ...devices["Desktop Chrome"] } },
    // Chrome of Android with the screen and touch of a Pixel 7 (RNF-20).
    { name: "android-chromium", use: { ...devices["Pixel 7"] } },
  ],
  webServer: {
    command: "bash e2e/server.sh",
    url: `http://localhost:${PORT}/api/ping`,
    timeout: 180_000,
    reuseExistingServer: false,
    env: { E2E_PORT: String(PORT) },
  },
});
