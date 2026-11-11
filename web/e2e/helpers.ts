import { expect, type Page } from "@playwright/test";

/** Fictional credentials created by e2e/server.sh. */
export const USER = { login: "teste", password: "senha-ficticia-123", name: "Pessoa Teste" };

/** Logs in through the form and waits for the app shell. */
export async function login(page: Page) {
  await page.goto("/login");
  await page.getByLabel("Usuário").fill(USER.login);
  await page.getByLabel("Senha").fill(USER.password);
  await page.getByRole("button", { name: "Entrar" }).click();
  await expect(page).toHaveURL(/\/$/);
}

/** Reports whether the project runs with a phone viewport. */
export function isPhone(page: Page) {
  return (page.viewportSize()?.width ?? 1280) < 768;
}
