import { expect, test, type Page } from "@playwright/test";
import { login } from "./helpers";

test.beforeEach(async ({ page }) => {
  await login(page);
});

/** Creates an account from the list screen. */
async function createAccount(page: Page, name: string, initialDigits: string) {
  await page.getByRole("button", { name: "Nova conta" }).first().click();
  const form = page.getByRole("dialog", { name: "Nova conta" });
  await form.getByLabel("Nome").fill(name);
  await form.getByLabel("Titular").selectOption({ label: "Ana Ficticia" });
  await form.getByLabel("Saldo inicial").fill(initialDigits);
  await form.getByRole("button", { name: "Salvar" }).click();
  await expect(form).toBeHidden();
}

test("accounts: balance, expense, transfer, adjustment and delete (RF-04 to RF-07)", async ({ page }, testInfo) => {
  // Long flow; WebKit needs more than the default 30 seconds when the whole suite runs in parallel.
  test.slow();
  // Projects share one database, so names carry the project to stay unique.
  const a = `Conta A ${testInfo.project.name}`;
  const b = `Conta B ${testInfo.project.name}`;

  await page.goto("/settings");
  await page.getByRole("link", { name: /Contas bancárias/ }).click();
  await expect(page).toHaveURL(/\/settings\/accounts$/);
  await createAccount(page, a, "100000");
  await createAccount(page, b, "0");

  await page.getByRole("link", { name: new RegExp(a) }).click();
  const balance = page.getByText("Saldo atual").locator("xpath=..");
  await expect(balance).toContainText("R$ 1.000,00");

  await page.getByRole("button", { name: "Saída" }).click();
  const expense = page.getByRole("dialog", { name: `Saída de ${a}` });
  await expense.getByLabel("Valor").fill("2550");
  await expense.getByLabel("Categoria").selectOption({ label: "Delivery" });
  await expense.getByRole("button", { name: "Lançar" }).click();
  await expect(expense).toBeHidden();
  await expect(balance).toContainText("R$ 974,50");

  await page.getByRole("button", { name: "Transferir" }).click();
  const transfer = page.getByRole("dialog", { name: `Transferir de ${a}` });
  await transfer.getByLabel("Para").selectOption({ label: `${b} (Ana Ficticia)` });
  await transfer.getByLabel("Valor").fill("10000");
  await transfer.getByRole("button", { name: "Transferir" }).click();
  await expect(transfer).toBeHidden();
  await expect(balance).toContainText("R$ 874,50");
  await expect(page.getByText(`Transferência para ${b}`)).toBeVisible();

  await page.getByRole("button", { name: "Ajustar saldo" }).click();
  const adjust = page.getByRole("dialog", { name: "Ajustar saldo" });
  await adjust.getByLabel("Saldo real").fill("90000");
  await adjust.getByRole("button", { name: "Ajustar" }).click();
  await expect(adjust).toBeHidden();
  await expect(balance).toContainText("R$ 900,00");
  await expect(page.getByText("Ajuste de saldo")).toBeVisible();

  await page.getByRole("button", { name: `Excluir Transferência para ${b}` }).click();
  const confirm = page.getByRole("dialog", { name: "Excluir lançamento?" });
  await confirm.getByRole("button", { name: "Excluir" }).click();
  await expect(confirm).toBeHidden();
  await expect(balance).toContainText("R$ 1.000,00");
});
