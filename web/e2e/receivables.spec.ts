import { expect, test } from "@playwright/test";
import { login } from "./helpers";

test.beforeEach(async ({ page }) => {
  await login(page);
});

test("split purchase: shares on the invoice, receipt and charge message (RF-24 to RF-30, RN-05)", async ({ page }, testInfo) => {
  test.slow();
  // Projects share one database, so names carry the project to stay unique.
  const tag = testInfo.project.name;
  const account = `Conta R4 ${tag}`;
  const cardName = `Cartão R4 ${tag}`;
  const joao = `João ${tag}`;
  const maria = `Maria ${tag}`;

  await page.goto("/settings/accounts");
  await page.getByRole("button", { name: "Nova conta" }).first().click();
  const accountForm = page.getByRole("dialog", { name: "Nova conta" });
  await accountForm.getByLabel("Nome").fill(account);
  await accountForm.getByLabel("Titular").selectOption({ label: "Ana Ficticia" });
  await accountForm.getByRole("button", { name: "Salvar" }).click();
  await expect(accountForm).toBeHidden();

  await page.goto("/cards");
  await page.getByRole("button", { name: "Novo cartão" }).first().click();
  const cardForm = page.getByRole("dialog", { name: "Novo cartão" });
  await cardForm.getByLabel("Nome").fill(cardName);
  await cardForm.getByLabel("Titular").selectOption({ label: "Ana Ficticia" });
  await cardForm.getByLabel("Limite total").fill("1000000");
  await cardForm.getByRole("button", { name: "Salvar" }).click();
  await expect(cardForm).toBeHidden();

  // Air conditioner of R$ 3.000,00 in 3 installments, equal parts with two people and the couple as one part.
  await page.getByRole("button", { name: "Adicionar" }).click();
  const sheet = page.getByRole("dialog", { name: "Novo gasto" });
  await sheet.getByLabel("Cartão").selectOption({ label: cardName });
  await sheet.getByLabel("Valor").fill("300000");
  await sheet.getByRole("radio", { name: "Moradia" }).click();
  await sheet.getByRole("radio", { name: "Móveis e utilidades" }).click();
  await page.keyboard.type("Ar-condicionado");
  await sheet.getByRole("button", { name: "Mais parcelas" }).click();
  await sheet.getByRole("button", { name: "Mais parcelas" }).click();
  await sheet.getByText("Dividir com outras pessoas").click();
  await sheet.getByLabel("Pessoa 1", { exact: true }).fill(joao);
  await sheet.getByRole("button", { name: "Adicionar pessoa" }).click();
  await sheet.getByLabel("Pessoa 2", { exact: true }).fill(maria);
  await sheet.getByRole("button", { name: "Confirmar" }).click();
  await expect(sheet).toBeHidden();

  // The invoice shows each share (R$ 333,34 on the first installment) and the couple part (RN-05).
  await page.goto("/cards");
  await page.getByRole("option", { name: cardName }).click();
  await page.getByRole("button", { name: /R\$ 1\.000,00/ }).first().click();
  await expect(page.getByText(`${joao} R$ 333,34`)).toBeVisible();
  await expect(page.getByText("Parte do casal").locator("..")).toContainText("R$ 333,32");

  // The person owes R$ 1.000,00 in three installments; receiving the first leaves two.
  await page.goto("/receivables");
  const person = page.locator("section").filter({ hasText: joao });
  await expect(person.getByText("3 parcelas pendentes")).toBeVisible();
  await person.getByRole("checkbox", { name: "Ar-condicionado 1 de 3" }).check();
  await person.getByRole("button", { name: "Recebi" }).click();
  const receipt = page.getByRole("dialog", { name: `Recebi de ${joao}` });
  await receipt.getByLabel("Caiu na conta").selectOption({ label: account });
  await receipt.getByRole("button", { name: "Registrar" }).click();
  await expect(receipt).toBeHidden();
  await expect(person.getByText("2 parcelas pendentes")).toBeVisible();

  await person.getByRole("button", { name: "Cobrar" }).click();
  const message = page.getByRole("dialog", { name: `Cobrar ${joao}` });
  await expect(message).toContainText("parcela 2 de 3");
  await expect(message).toContainText("Total: R$ 666,66");
});
