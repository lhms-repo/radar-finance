import { expect, test } from "@playwright/test";
import { login } from "./helpers";

test.beforeEach(async ({ page }) => {
  await login(page);
});

test("overview shows the cards of section 5.3 and the bell opens the notifications (RF-58, RF-64)", async ({ page }) => {
  for (const title of ["Saldo previsto", "Contas a pagar", "Cartões", "Moradia", "Valores a receber", "Caixinhas", "Financiamentos", "Evolução dos gastos", "Orçamento"]) {
    await expect(page.getByRole("heading", { name: title, exact: true })).toBeVisible();
  }
  await expect(page.getByText("Sobra")).toBeVisible();
  await page.getByRole("radio", { name: "Só meu salário" }).click();
  await expect(page.getByText(/do seu salário/)).toBeVisible();

  await page.getByRole("button", { name: /^Notificações/ }).click();
  await expect(page.getByRole("dialog").getByText("Notificações", { exact: true })).toBeVisible();
});

test("cash flow shows closing, net worth and the search filters the entries (RF-61 to RF-63)", async ({ page }, testInfo) => {
  // A long flow: under three parallel browser engines it gets near the default 30 s.
  test.slow();
  const account = `Conta F ${testInfo.project.name}`;
  await page.goto("/settings/accounts");
  await page.getByRole("button", { name: "Nova conta" }).first().click();
  const form = page.getByRole("dialog", { name: "Nova conta" });
  await form.getByLabel("Nome").fill(account);
  await form.getByLabel("Titular").selectOption({ label: "Ana Ficticia" });
  await form.getByLabel("Saldo inicial").fill("100000");
  await form.getByRole("button", { name: "Salvar" }).click();
  await expect(form).toBeHidden();
  await page.getByRole("link", { name: new RegExp(account) }).click();
  await page.getByRole("button", { name: "Saída" }).click();
  const expense = page.getByRole("dialog", { name: `Saída de ${account}` });
  await expense.getByLabel("Valor").fill("1234");
  await expense.getByLabel("Categoria").selectOption({ label: "Delivery" });
  await expense.getByLabel("Descrição").fill(`Busca ${testInfo.project.name}`);
  await expense.getByRole("button", { name: "Lançar" }).click();
  await expect(expense).toBeHidden();

  await page.goto("/cash-flow");
  await expect(page.getByText(/Fechamento de/)).toBeVisible();
  await expect(page.getByText("Patrimônio líquido")).toBeVisible();
  await page.getByLabel("Buscar texto").fill(`Busca ${testInfo.project.name}`);
  await expect(page.getByText(`Busca ${testInfo.project.name}`, { exact: true })).toBeVisible();
  await expect(page.getByRole("listitem").filter({ hasText: `Busca ${testInfo.project.name}` })).toContainText("R$ 12,34");
});

test("budgets are set per top level category (RF-51)", async ({ page }, testInfo) => {
  // Projects share one database; each one writes a different value so the change is always real.
  const value = { "iphone-webkit": "50000", "desktop-webkit": "51000", "desktop-chromium": "52000" }[testInfo.project.name] ?? "53000";
  await page.goto("/settings/budgets");
  await page.getByLabel("Orçamento de Lazer", { exact: true }).fill(value);
  await page.getByRole("button", { name: "Salvar orçamento de Lazer" }).click();
  await expect(page.getByRole("listitem").filter({ hasText: "Lazer" })).toContainText("0%");
});
