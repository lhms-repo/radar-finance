import { expect, test } from "@playwright/test";
import { USERS, chooseHome, login } from "./helpers";

test("redirects to the user picker without a session", async ({ page }) => {
  await page.goto("/settings");
  await expect(page).toHaveURL(/\/login$/);
  await chooseHome(page);
  await expect(page.getByRole("heading", { name: "Quem está usando?" })).toBeVisible();
  for (const u of USERS) {
    await expect(page.getByRole("button", { name: u.name })).toBeVisible();
  }
});

test("a wrong PIN is refused and the right one enters (RF-01, RN-27)", async ({ page }) => {
  await page.goto("/login");
  await chooseHome(page);
  await page.getByRole("button", { name: USERS[0]!.name }).click();
  await page.getByLabel("PIN").fill("0000");
  await page.getByRole("button", { name: "Entrar" }).click();
  await expect(page.getByRole("alert")).toHaveText("PIN incorreto.");
  await expect(page.getByLabel("PIN")).toHaveValue("");
  await page.getByLabel("PIN").fill(USERS[0]!.pin);
  await page.getByRole("button", { name: "Entrar" }).click();
  await expect(page).toHaveURL(/\/$/);
});

test("enters with the PIN, keeps the session and switches user (RF-01)", async ({ page }) => {
  await login(page, USERS[1]!.name);
  await page.reload();
  await expect(page).toHaveURL(/\/$/);
  await expect(page.getByText("Saldo previsto")).toBeVisible();

  await page.goto("/settings");
  await page.getByRole("button", { name: `Trocar de usuário (${USERS[1]!.name})` }).click();
  await expect(page).toHaveURL(/\/login$/);
  await page.goto("/");
  await expect(page).toHaveURL(/\/login$/);
});
