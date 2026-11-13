import { expect, test } from "@playwright/test";
import { isPhone, login, openShortcuts, switchSpace } from "./helpers";

test.beforeEach(async ({ page }) => {
  await login(page);
});

test("alelo: register, buy from the quick entry in Casa, confirm the credit and carry the balance in the personal space (RF-31 to RF-34, RN-15, RF-83)", async ({ page }, testInfo) => {
  test.slow();
  // Projects share one database, so names carry the project to stay unique.
  const name = `Alelo ${testInfo.project.name}`;
  const purchase = `Mercado do bairro ${testInfo.project.name}`;

  await page.goto("/alelo");
  await page.getByRole("button", { name: "Cartão Alelo" }).first().click();
  const form = page.getByRole("dialog", { name: "Novo cartão Alelo" });
  await form.getByLabel("Nome").fill(name);
  await form.getByLabel("Titular").selectOption({ label: "Ana Ficticia" });
  await form.getByLabel("Crédito mensal").fill("80000");
  await form.getByLabel("Cai dia").fill("5");
  await form.getByLabel("Saldo hoje").fill("12000");
  await form.getByRole("button", { name: "Salvar" }).click();
  await expect(form).toBeHidden();

  // Gasto and Alelo: no type, no category, only card, description and value (section 5.6).
  await openShortcuts(page);
  await page.getByRole(isPhone(page) ? "button" : "menuitem", { name: "Compra no Alelo" }).click();
  const sheet = page.getByRole("dialog", { name: "Novo gasto" });
  await expect(sheet.getByRole("radiogroup", { name: "Tipo" })).toBeHidden();
  await expect(sheet.getByRole("radiogroup", { name: "Categoria" })).toBeHidden();
  await sheet.getByLabel("Valor").fill("45390");
  await sheet.getByLabel("Cartão Alelo").selectOption({ label: name });
  await sheet.getByLabel("Descrição").fill(purchase);
  await sheet.getByRole("button", { name: "Confirmar" }).click();
  await expect(sheet).toBeHidden();

  // Casa reads its purchase without the credit; the balance and the full month stay with the holder (RF-83).
  await page.goto("/alelo");
  await page.getByRole("option", { name }).click();
  await expect(page.getByText(purchase)).toBeVisible();
  await expect(page.getByText("Compras deste espaço").locator("..")).toContainText("R$ 453,90");
  await expect(page.getByRole("button", { name: "Caiu" })).toBeHidden();

  // In her personal space Ana confirms the credit and reads every space: R$ 120,00 + R$ 800,00 - R$ 453,90.
  await switchSpace(page, "Pessoal");
  await page.goto("/alelo");
  await page.getByRole("option", { name }).click();
  await page.getByRole("button", { name: "Caiu" }).click();
  await expect(page.getByText(/Crédito recebido em/)).toBeVisible();
  await expect(page.getByText(purchase).locator("..")).toContainText("Casa");
  await expect(page.getByText("Fica para o próximo").locator("..")).toContainText("R$ 466,10");
});
