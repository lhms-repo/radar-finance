import { expect, test } from "@playwright/test";
import { USERS, login } from "./helpers";

test("an administrator registers a person who then enters with the PIN (RF-80, RN-27)", async ({ page }, testInfo) => {
  test.slow();
  // Projects share one database, so the new person carries the project name.
  const tag = testInfo.project.name;
  const name = `Carla ${tag}`;
  const loginName = `carla-${tag}`;

  await login(page);
  await page.goto("/settings/group");
  await expect(page.getByRole("heading", { name: "Grupo e pessoas" })).toBeVisible();
  await page.getByRole("button", { name: "Cadastrar pessoa" }).click();
  const form = page.getByRole("dialog", { name: "Cadastrar pessoa" });
  await form.getByLabel("Nome").fill(name);
  await form.getByLabel("Login").fill(loginName);
  await form.getByLabel("PIN").fill("9264");
  await form.getByRole("button", { name: "Cadastrar" }).click();
  await expect(form).toBeHidden();
  await expect(page.getByText(`@${loginName}`)).toBeVisible();

  // The new person enters on the entry screen of the same group.
  await page.goto("/settings");
  await page.getByRole("button", { name: `Trocar de usuário (${USERS[0]!.name})` }).click();
  await expect(page).toHaveURL(/\/login$/);
  await page.getByRole("button", { name }).click();
  await page.getByLabel("PIN").fill("9264");
  await page.getByRole("button", { name: "Entrar" }).click();
  await expect(page).toHaveURL(/\/$/);
});

test("a member sees the people but no management (RF-80)", async ({ page }) => {
  await login(page, USERS[1]!.name);
  await page.goto("/settings/group");
  await expect(page.getByText(`@${USERS[0]!.login}`)).toBeVisible();
  await expect(page.getByRole("button", { name: "Cadastrar pessoa" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Renomear" })).toHaveCount(0);
});
