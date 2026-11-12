import { expect, test } from "@playwright/test";
import { login } from "./helpers";

test.beforeEach(async ({ page }) => {
  await login(page);
});

test("incomes: register, confirm the receipt, undo and one off income (RF-09 to RF-13, RN-13)", async ({ page }, testInfo) => {
  // A long flow: under three parallel browser engines it gets near the default 30 s.
  test.slow();
  // Projects share one database, so names carry the project to stay unique.
  const account = `Conta R ${testInfo.project.name}`;
  const salary = `Salário ${testInfo.project.name}`;
  const sale = `Venda ${testInfo.project.name}`;

  await page.goto("/settings/accounts");
  await page.getByRole("button", { name: "Nova conta" }).first().click();
  const accountForm = page.getByRole("dialog", { name: "Nova conta" });
  await accountForm.getByLabel("Nome").fill(account);
  await accountForm.getByLabel("Titular").selectOption({ label: "Ana Ficticia" });
  await accountForm.getByRole("button", { name: "Salvar" }).click();
  await expect(accountForm).toBeHidden();

  await page.goto("/settings");
  await page.getByRole("link", { name: /Rendas/ }).click();
  await expect(page).toHaveURL(/\/settings\/incomes$/);
  await page.getByRole("button", { name: "Nova renda" }).click();
  const form = page.getByRole("dialog", { name: "Nova renda" });
  await form.getByLabel("Nome").fill(salary);
  await form.getByLabel("De quem").selectOption({ label: "Ana Ficticia" });
  await form.getByLabel("Conta que recebe").selectOption({ label: account });
  await form.getByLabel("Valor líquido").fill("450000");
  await form.getByRole("button", { name: "Salvar" }).click();
  await expect(form).toBeHidden();

  await page.getByRole("button", { name: `Confirmar ${salary}` }).click();
  const confirm = page.getByRole("dialog", { name: `Recebi ${salary}` });
  await expect(confirm.getByLabel("Valor recebido")).toHaveValue("4.500,00");
  await confirm.getByRole("button", { name: "Confirmar" }).click();
  await expect(confirm).toBeHidden();
  const undo = page.getByRole("button", { name: `Desfazer recebimento de ${salary}` });
  await expect(undo).toBeVisible();

  await page.goto("/settings/accounts");
  await expect(page.getByRole("link", { name: new RegExp(account) })).toContainText("R$ 4.500,00");

  await page.goto("/settings/incomes");
  await page.getByRole("button", { name: `Desfazer recebimento de ${salary}` }).click();
  await expect(page.getByRole("button", { name: `Confirmar ${salary}` })).toBeVisible();

  await page.getByRole("button", { name: "Renda avulsa" }).click();
  const oneOff = page.getByRole("dialog", { name: "Renda avulsa" });
  await oneOff.getByLabel("Descrição").fill(sale);
  await oneOff.getByLabel("Valor").fill("20000");
  await oneOff.getByLabel("Conta").selectOption({ label: account });
  await oneOff.getByRole("button", { name: "Lançar" }).click();
  await expect(oneOff).toBeHidden();
  await expect(page.getByText(sale, { exact: true })).toBeVisible();
});
