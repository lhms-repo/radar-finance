import { expect, test } from "@playwright/test";
import { login } from "./helpers";

test.beforeEach(async ({ page }) => {
  await login(page);
});

test("savings: create, confirm a deposit up to the target, finish and withdraw (RF-43 to RF-49, RN-12)", async ({ page }, testInfo) => {
  test.slow();
  // Projects share one database, so names carry the project to stay unique.
  const tag = testInfo.project.name;
  const account = `Conta S ${tag}`;
  const name = `Viagem ${tag}`;

  await page.goto("/settings/accounts");
  await page.getByRole("button", { name: "Nova conta" }).first().click();
  const accountForm = page.getByRole("dialog", { name: "Nova conta" });
  await accountForm.getByLabel("Nome").fill(account);
  await accountForm.getByLabel("Titular").selectOption({ label: "Ana Ficticia" });
  await accountForm.getByLabel("Saldo inicial").fill("100000");
  await accountForm.getByRole("button", { name: "Salvar" }).click();
  await expect(accountForm).toBeHidden();

  await page.goto("/savings");
  await page.getByRole("button", { name: "Nova caixinha" }).click();
  const form = page.getByRole("dialog", { name: "Nova caixinha" });
  await form.getByLabel("Nome").fill(name);
  await form.getByLabel("Valor-alvo").fill("100000");
  await form.getByLabel("Percentual das entradas do mês").fill("1500");
  await expect(form.getByText("15%")).toBeVisible();
  await form.getByLabel("Já guardado hoje").fill("90000");
  await form.getByRole("button", { name: "Salvar" }).click();
  await expect(form).toBeHidden();

  const box = page.locator("section").filter({ hasText: name });
  await expect(box).toContainText("15% das entradas");
  await box.getByRole("button", { name: "Confirmar" }).click();
  const deposit = page.getByRole("dialog", { name: `Aporte em ${name}` });
  await deposit.getByLabel("Valor guardado").fill("10000");
  await deposit.getByLabel("Saiu da conta").selectOption({ label: account });
  await deposit.getByRole("button", { name: "Confirmar aporte" }).click();
  await expect(deposit).toBeHidden();
  await expect(box.getByText("Concluída")).toBeVisible();
  await expect(box.getByText("100%")).toBeVisible();

  await box.getByRole("button", { name: "Retirar" }).click();
  const withdraw = page.getByRole("dialog", { name: `Retirar de ${name}` });
  await withdraw.getByLabel("Valor retirado").fill("5000");
  await withdraw.getByLabel("Voltou para a conta").selectOption({ label: account });
  await withdraw.getByRole("button", { name: "Retirar" }).click();
  await expect(withdraw.getByText("Informe o motivo da retirada.")).toBeVisible();
  await withdraw.getByLabel("Motivo").fill("Passagem");
  await withdraw.getByRole("button", { name: "Retirar" }).click();
  await expect(withdraw).toBeHidden();
  await expect(box.getByText("Concluída")).toBeHidden();
  await expect(box.getByText("95%")).toBeVisible();

  // R$ 1.000,00 - R$ 100,00 deposited + R$ 50,00 withdrawn.
  await page.goto("/settings/accounts");
  await expect(page.getByRole("link", { name: new RegExp(account) })).toContainText("R$ 950,00");
});
