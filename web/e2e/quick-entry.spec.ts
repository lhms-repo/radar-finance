import { expect, test, type Page } from "@playwright/test";
import { isPhone, login, openShortcuts } from "./helpers";

test.beforeEach(async ({ page }) => {
  await login(page);
});

/** Opens a shortcut of the Add button, from the fan on the iPhone or the menu on desktop. */
async function shortcut(page: Page, name: string) {
  await openShortcuts(page);
  await page.getByRole(isPhone(page) ? "button" : "menuitem", { name }).click();
}

test("quick entry: card purchase, cash expense and one off income (section 5.6)", async ({ page }, testInfo) => {
  test.slow();
  // Projects share one database, so names carry the project to stay unique.
  const tag = testInfo.project.name;
  const account = `Conta Q ${tag}`;
  const cardName = `Cartão Q ${tag}`;

  await page.goto("/settings/accounts");
  await page.getByRole("button", { name: "Nova conta" }).first().click();
  const accountForm = page.getByRole("dialog", { name: "Nova conta" });
  await accountForm.getByLabel("Nome").fill(account);
  await accountForm.getByLabel("Titular").selectOption({ label: "Ana Ficticia" });
  await accountForm.getByLabel("Saldo inicial").fill("100000");
  await accountForm.getByRole("button", { name: "Salvar" }).click();
  await expect(accountForm).toBeHidden();

  await page.goto("/cards");
  await page.getByRole("button", { name: "Novo cartão" }).first().click();
  const cardForm = page.getByRole("dialog", { name: "Novo cartão" });
  await cardForm.getByLabel("Nome").fill(cardName);
  await cardForm.getByLabel("Titular").selectOption({ label: "Ana Ficticia" });
  await cardForm.getByLabel("Limite total").fill("100000");
  await cardForm.getByRole("button", { name: "Salvar" }).click();
  await expect(cardForm).toBeHidden();

  // Card purchase in four taps (RNF-13): Add, category, subcategory, confirm; the rest is typing.
  await page.getByRole("button", { name: "Adicionar" }).click();
  const sheet = page.getByRole("dialog", { name: "Novo gasto" });
  await sheet.getByLabel("Cartão").selectOption({ label: cardName }); // the database is shared by the projects
  await sheet.getByLabel("Valor").focus();
  await page.keyboard.type("4590");
  await sheet.getByRole("radio", { name: "Alimentação" }).click();
  await sheet.getByRole("radio", { name: "Delivery" }).click();
  await expect(sheet.getByLabel("Estabelecimento")).toBeFocused();
  await page.keyboard.type("Pizzaria Ficticia");
  await sheet.getByRole("button", { name: "Confirmar" }).click();
  await expect(sheet).toBeHidden();

  await page.goto("/cards");
  await page.getByRole("option", { name: cardName }).click();
  await page.getByRole("button", { name: /R\$ 45,90/ }).first().click();
  await expect(page.getByText("Pizzaria Ficticia")).toBeVisible();

  // Cash expense: no type selector, account comes in the details.
  await shortcut(page, "Despesa");
  await expect(sheet.getByRole("radiogroup", { name: "Tipo" })).toBeHidden();
  await sheet.getByLabel("Valor").fill("2000");
  await sheet.getByRole("radio", { name: "Transporte" }).click();
  await sheet.getByRole("radio", { name: "Aplicativos de corrida" }).click();
  await sheet.getByLabel("Conta de origem").selectOption({ label: account });
  await sheet.getByRole("button", { name: "Confirmar" }).click();
  await expect(sheet).toBeHidden();

  // One off income: no category grid.
  await shortcut(page, "Receita");
  const incomeSheet = page.getByRole("dialog", { name: "Nova receita" });
  await expect(incomeSheet.getByRole("radiogroup", { name: "Categoria" })).toBeHidden();
  await incomeSheet.getByLabel("Valor").fill("15000");
  await incomeSheet.getByLabel("Conta de destino").selectOption({ label: account });
  await incomeSheet.getByLabel("Descrição").fill(`Venda Q ${tag}`);
  await incomeSheet.getByRole("button", { name: "Confirmar" }).click();
  await expect(incomeSheet).toBeHidden();

  // R$ 1.000,00 - R$ 20,00 + R$ 150,00.
  await page.goto("/settings/accounts");
  await expect(page.getByRole("link", { name: new RegExp(account) })).toContainText("R$ 1.130,00");
});

test("quick entry: missing value and category are reported in the card (section 5.6)", async ({ page }) => {
  await page.getByRole("button", { name: "Adicionar" }).click();
  const sheet = page.getByRole("dialog", { name: "Novo gasto" });
  await sheet.getByRole("button", { name: "Confirmar" }).click();
  await expect(sheet.getByText("Digite o valor.")).toBeVisible();
  await expect(sheet.getByText("Escolha a categoria.")).toBeVisible();
});
