import { expect, test } from "@playwright/test";
import { USER, login } from "./helpers";

test("redirects to login without a session", async ({ page }) => {
  await page.goto("/settings");
  await expect(page).toHaveURL(/\/login$/);
  await expect(page.getByRole("button", { name: "Entrar" })).toBeVisible();
});

test("shows an error for wrong credentials (RF-01)", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("Usuário").fill("ninguem");
  await page.getByLabel("Senha").fill("senha-errada");
  await page.getByRole("button", { name: "Entrar" }).click();
  await expect(page.getByRole("alert")).toContainText("Usuário ou senha incorretos");
});

test("logs in, keeps the session on reload and logs out", async ({ page }) => {
  await login(page);
  await page.reload();
  await expect(page).toHaveURL(/\/$/);
  await expect(page.getByText("Chega na fase 5").first()).toBeVisible();

  await page.goto("/settings");
  await page.getByRole("button", { name: new RegExp(`Sair \\(${USER.login}\\)`) }).click();
  await expect(page).toHaveURL(/\/login$/);
  await page.goto("/");
  await expect(page).toHaveURL(/\/login$/);
});
