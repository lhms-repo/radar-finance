import { expect, test } from "@playwright/test";
import { login } from "./helpers";

test.beforeEach(async ({ page }) => {
  await login(page);
});

test("manages categories and subcategories from Settings (RF-50, RN-18)", async ({ page }, testInfo) => {
  // A long flow: under three parallel browser engines it gets near the default 30 s.
  test.slow();
  // Projects share one database, so names carry the project to stay unique.
  const name = `Doações ${testInfo.project.name}`;
  const sub = `Igreja ${testInfo.project.name}`;

  await page.goto("/settings");
  await page.getByRole("link", { name: /Categorias/ }).click();
  await expect(page).toHaveURL(/\/settings\/categories$/);
  await expect(page.getByRole("button", { name: /^Moradia/ })).toBeVisible();

  await page.getByRole("button", { name: "Nova categoria" }).click();
  const form = page.getByRole("dialog", { name: "Nova categoria" });
  await form.getByLabel("Nome").fill(name);
  await form.getByRole("button", { name: "Salvar" }).click();
  await expect(form.getByText("Escolha um ícone.")).toBeVisible();
  await form.getByRole("radio", { name: "gift" }).click();
  await form.getByRole("button", { name: "Salvar" }).click();
  await expect(form).toBeHidden();

  const row = page.getByRole("button", { name: new RegExp(`^${name}`) });
  await row.click();
  await expect(row).toHaveAttribute("aria-expanded", "true");
  await page.getByRole("button", { name: "Adicionar subcategoria" }).click();
  const subForm = page.getByRole("dialog", { name: `Nova subcategoria em ${name}` });
  await subForm.getByLabel("Nome").fill(sub);
  await subForm.getByRole("button", { name: "Salvar" }).click();
  await expect(subForm).toBeHidden();
  await expect(page.getByText(sub, { exact: true })).toBeVisible();

  await page.getByRole("button", { name: `Ações de ${sub}` }).click();
  await page.getByRole("menuitem", { name: "Desativar" }).click();
  await expect(page.getByRole("listitem").filter({ hasText: sub }).getByText("Desativada")).toBeVisible();

  await page.getByRole("button", { name: `Ações de ${sub}` }).click();
  await page.getByRole("menuitem", { name: "Excluir" }).click();
  const confirm = page.getByRole("dialog", { name: `Excluir ${sub}?` });
  await confirm.getByRole("button", { name: "Excluir" }).click();
  await expect(confirm).toBeHidden();
  await expect(page.getByText(sub, { exact: true })).toBeHidden();
});
