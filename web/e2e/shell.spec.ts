import { expect, test } from "@playwright/test";
import { isPhone, login, openShortcuts } from "./helpers";

test.beforeEach(async ({ page }) => {
  await login(page);
});

test("desktop sidebar navigates and collapses with the fluid tooltip (section 5.5)", async ({ page }) => {
  test.skip(isPhone(page), "desktop only");
  const menu = page.getByRole("complementary", { name: "Menu principal" });
  await menu.getByRole("link", { name: "Fluxo de caixa" }).click();
  await expect(page).toHaveURL(/\/cash-flow$/);
  await expect(page.getByText("Chega na fase 5")).toBeVisible();

  await menu.getByRole("button", { name: "Recolher menu" }).click();
  const caixinhas = menu.getByRole("link", { name: "Caixinhas" });
  await caixinhas.hover();
  await expect(menu.getByText("Caixinhas", { exact: true })).toBeVisible();
  await menu.getByRole("button", { name: "Abrir menu" }).click();
});

test("iPhone bottom bar and the More sheet (section 5.5)", async ({ page }) => {
  test.skip(!isPhone(page), "iPhone only");
  const bar = page.getByRole("navigation", { name: "Navegação" });
  await expect(bar).toBeVisible();
  await expect(page.getByRole("complementary", { name: "Menu principal" })).toBeHidden();

  await bar.getByRole("link", { name: "Caixinhas" }).click();
  await expect(page).toHaveURL(/\/savings$/);

  await bar.getByRole("button", { name: "Mais" }).click();
  const sheet = page.getByRole("dialog", { name: "Mais telas" });
  await expect(sheet).toBeVisible();
  await sheet.getByRole("link", { name: "Mercado" }).click();
  await expect(page).toHaveURL(/\/market$/);
  await expect(sheet).toBeHidden();
});

test("Add opens the card purchase and the shortcuts open apart (sections 5.6 and 8.6)", async ({ page }) => {
  await page.getByRole("button", { name: "Adicionar" }).click();
  const sheet = page.getByRole("dialog", { name: "Novo gasto" });
  await expect(sheet).toBeVisible();
  await expect(sheet.getByLabel("Valor")).toBeFocused();
  await sheet.getByRole("button", { name: "Fechar" }).click();
  await expect(sheet).toBeHidden();

  await openShortcuts(page);
  await expect(page.getByRole(isPhone(page) ? "button" : "menuitem", { name: "Receita" })).toBeVisible();
});

test("month selector moves between months", async ({ page }) => {
  const title = page.getByRole("button", { name: /^Mês selecionado:/ });
  const before = await title.getAttribute("data-month");
  expect(before).toMatch(/^[A-ZÇ][a-zç]+ \d{4}$/);
  await page.getByRole("button", { name: "Próximo mês" }).click();
  await expect(title).not.toHaveAttribute("data-month", before!);
  await page.getByRole("button", { name: "Mês anterior" }).click();
  await expect(title).toHaveAttribute("data-month", before!);
});

test("theme and privacy toggles (RNF-14, RNF-15)", async ({ page }) => {
  const html = page.locator("html");
  await page.getByRole("button", { name: /Ocultar valores/ }).click();
  await expect(html).toHaveClass(/privacy/);
  await page.getByRole("button", { name: /Mostrar valores/ }).click();
  await expect(html).not.toHaveClass(/privacy/);

  await page.goto("/settings");
  await page.getByRole("radio", { name: "Escuro" }).click();
  await expect(html).toHaveClass(/dark/);
  await page.getByRole("radio", { name: "Claro" }).click();
  await expect(html).not.toHaveClass(/dark/);
});

test("settings shows health and makes a manual backup (RNF-07)", async ({ page }) => {
  await page.goto("/settings");
  await expect(page.getByText("Saúde do sistema")).toBeVisible();
  await expect(page.getByText("Ligado há")).toBeVisible();
  await page.getByRole("button", { name: "Fazer agora" }).click();
  await expect(page.getByText("Backup feito")).toBeVisible();
  await expect(page.getByText("Manual").first()).toBeVisible();
});
