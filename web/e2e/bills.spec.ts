import { expect, test } from "@playwright/test";
import { login } from "./helpers";

test.beforeEach(async ({ page }) => {
  await login(page);
});

test("bills and financings: register, real amount, pay and undo (RF-35 to RF-42)", async ({ page }, testInfo) => {
  test.slow();
  // Projects share one database, so names carry the project to stay unique.
  const tag = testInfo.project.name;
  const account = `Conta Fixas ${tag}`;
  const bill = `Energia ${tag}`;
  const loan = `Casa ${tag}`;

  await page.goto("/settings/accounts");
  await page.getByRole("button", { name: "Nova conta" }).first().click();
  const accountForm = page.getByRole("dialog", { name: "Nova conta" });
  await accountForm.getByLabel("Nome").fill(account);
  await accountForm.getByLabel("Titular").selectOption({ label: "Ana Ficticia" });
  await accountForm.getByLabel("Saldo inicial").fill("1000000");
  await accountForm.getByRole("button", { name: "Salvar" }).click();
  await expect(accountForm).toBeHidden();

  await page.goto("/bills");
  await page.getByRole("button", { name: "Conta fixa", exact: true }).click();
  const billForm = page.getByRole("dialog", { name: "Nova conta fixa" });
  await billForm.getByLabel("Nome").fill(bill);
  await billForm.getByLabel("Categoria").selectOption({ label: "Energia" });
  await billForm.getByLabel("Conta de pagamento").selectOption({ label: account });
  await billForm.getByLabel("Vence dia").fill("28");
  await billForm.getByLabel("Valor previsto").fill("18000");
  await billForm.getByRole("button", { name: "Salvar" }).click();
  await expect(billForm).toBeHidden();

  // Real amount replaces the expected one (RN-14).
  await page.getByRole("button", { name: `Informar valor real de ${bill}` }).click();
  const actual = page.getByRole("dialog", { name: `Valor real de ${bill}` });
  await actual.getByLabel("Valor real").fill("17450");
  await actual.getByRole("button", { name: "Salvar" }).click();
  await expect(actual).toBeHidden();
  const row = page.getByRole("listitem").filter({ has: page.getByRole("button", { name: `Pagar ${bill}` }) });
  await expect(row).toContainText("R$ 174,50");

  // Paying draws the check; undoing brings it back.
  await page.getByRole("button", { name: `Pagar ${bill}` }).click();
  const pay = page.getByRole("dialog", { name: `Pagar ${bill}` });
  await expect(pay.getByLabel("Valor pago")).toHaveValue("174,50");
  await pay.getByRole("button", { name: "Pagar" }).click();
  await expect(pay).toBeHidden();
  await page.getByRole("button", { name: `Desfazer pagamento de ${bill}` }).click();
  await expect(page.getByRole("button", { name: `Pagar ${bill}` })).toBeVisible();

  // Financing: 360 installments, 24 paid, next one this month.
  await page.getByRole("button", { name: "Financiamento", exact: true }).click();
  const loanForm = page.getByRole("dialog", { name: "Novo financiamento" });
  await loanForm.getByLabel("Nome").fill(loan);
  await loanForm.getByLabel("Categoria").selectOption({ label: "Financiamento da casa" });
  await loanForm.getByLabel("Conta de pagamento").selectOption({ label: account });
  await loanForm.getByLabel("Vence dia").fill("20");
  await loanForm.getByLabel("Valor da parcela").fill("150000");
  await loanForm.getByLabel("Saldo devedor hoje").fill("28000000");
  await loanForm.getByLabel("Total de parcelas").fill("360");
  await loanForm.getByLabel("Já pagas").fill("24");
  await loanForm.getByRole("button", { name: "Salvar" }).click();
  await expect(loanForm).toBeHidden();

  // Paying suggests balance minus installment; the bank shows another value (RN-16).
  await page.getByRole("button", { name: `Pagar ${loan}` }).click();
  const payLoan = page.getByRole("dialog", { name: "Pagar parcela 25 de 360" });
  await expect(payLoan.getByLabel("Saldo devedor depois do pagamento")).toHaveValue("278.500,00");
  await payLoan.getByLabel("Saldo devedor depois do pagamento").fill("27860000");
  await payLoan.getByRole("button", { name: "Pagar" }).click();
  await expect(payLoan).toBeHidden();
  const card = page.getByText(loan, { exact: true }).locator("xpath=ancestor::div[contains(@class,'rounded-xl')][1]");
  await expect(card).toContainText("25 de 360 · faltam 335");
  await expect(card).toContainText("R$ 278.600,00");
});
