import { expect, test } from "@playwright/test";
import { USERS, login } from "./helpers";

test("redirects to the user picker without a session", async ({ page }) => {
  await page.goto("/settings");
  await expect(page).toHaveURL(/\/login$/);
  await expect(page.getByRole("heading", { name: "Quem está usando?" })).toBeVisible();
  for (const u of USERS) {
    await expect(page.getByRole("button", { name: u.name })).toBeVisible();
  }
});

test("enters without password, keeps the session and switches user (RF-01)", async ({ page }) => {
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
