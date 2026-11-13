import { expect, test, type Page } from "@playwright/test";
import { USERS, login } from "./helpers";

/** Records an expense from the account page, choosing how the members divide it. */
async function expense(page: Page, account: string, digits: string, divide: (dialog: ReturnType<Page["getByRole"]>) => Promise<void>) {
  await page.goto("/settings/accounts");
  await page.getByRole("link", { name: new RegExp(account) }).click();
  await page.getByRole("button", { name: "Saída" }).click();
  const dialog = page.getByRole("dialog", { name: `Saída de ${account}` });
  await dialog.getByLabel("Valor").fill(digits);
  await dialog.getByLabel("Categoria").selectOption({ label: "Delivery" });
  await divide(dialog);
  await dialog.getByRole("button", { name: "Lançar" }).click();
  await expect(dialog).toBeHidden();
}

test("three members settle to the cent and reach zero after the suggested payments (RF-84, RN-28 to RN-30, Fase 10)", async ({ page }, testInfo) => {
  test.slow();
  // Projects share one database, so the group, the third member and the accounts carry the project name.
  const tag = testInfo.project.name;
  const group = `Acerto ${tag}`;
  const carla = `Carla ${tag}`;
  const [ana, bruno] = [USERS[0]!, USERS[1]!];
  const accounts = { ana: `Conta Ana ${tag}`, bruno: `Conta Bruno ${tag}`, carla: `Conta Carla ${tag}` };

  await login(page);
  await page.goto("/settings/group");
  await page.getByRole("button", { name: "Criar grupo" }).click();
  const create = page.getByRole("dialog", { name: "Criar grupo" });
  await create.getByLabel("Nome").fill(group);
  await create.getByRole("button", { name: "Criar" }).click();
  await create.getByRole("button", { name: `Ir para ${group}` }).click();
  await expect(page).toHaveURL(/\/$/);

  // Bruno joins with his PIN and Carla is registered.
  await page.goto("/settings/group");
  await page.getByRole("button", { name: "Já cadastrada" }).click();
  const existing = page.getByRole("dialog", { name: "Adicionar pessoa já cadastrada" });
  await existing.getByLabel("Login").fill(bruno.login);
  await existing.getByLabel("PIN da pessoa").fill(bruno.pin);
  await existing.getByRole("button", { name: "Adicionar" }).click();
  await expect(existing).toBeHidden();
  await page.getByRole("button", { name: "Cadastrar pessoa" }).click();
  const person = page.getByRole("dialog", { name: "Cadastrar pessoa" });
  await person.getByLabel("Nome").fill(carla);
  await person.getByLabel("Login").fill(`carla-acerto-${tag}`);
  await person.getByLabel("PIN").fill("9264");
  await person.getByRole("button", { name: "Cadastrar" }).click();
  await expect(person).toBeHidden();

  // Weights of 50%, 30% and 20%.
  await page.getByRole("button", { name: "Pesos" }).click();
  const weights = page.getByRole("dialog", { name: "Pesos no acerto" });
  await weights.getByLabel(`${ana.name} (%)`).fill("50");
  await weights.getByLabel(`${bruno.name} (%)`).fill("30");
  await weights.getByLabel(`${carla} (%)`).fill("20");
  await weights.getByRole("button", { name: "Salvar" }).click();
  await expect(weights).toBeHidden();

  // One account for each member.
  await page.goto("/settings/accounts");
  for (const [holder, name] of [[ana.name, accounts.ana], [bruno.name, accounts.bruno], [carla, accounts.carla]] as const) {
    await page.getByRole("button", { name: "Nova conta" }).first().click();
    const form = page.getByRole("dialog", { name: "Nova conta" });
    await form.getByLabel("Nome").fill(name);
    await form.getByLabel("Titular").selectOption({ label: holder });
    await form.getByLabel("Saldo inicial").fill("100000");
    await form.getByRole("button", { name: "Salvar" }).click();
    await expect(form).toBeHidden();
  }

  // Ana pays R$ 100,00 by the weights: 50,00, 30,00 and 20,00.
  await expense(page, accounts.ana, "10000", async () => {});
  // Bruno pays R$ 90,01 in equal parts: the cent left goes to Ana, who joined first.
  await expense(page, accounts.bruno, "9001", async (d) => {
    await d.getByRole("radio", { name: "Iguais" }).click();
  });
  // Carla pays R$ 300,00 by value: Ana 50,00, Bruno 150,00 and Carla 100,00.
  await expense(page, accounts.carla, "30000", async (d) => {
    await d.getByRole("radio", { name: "Valores" }).click();
    await d.getByLabel(ana.name, { exact: true }).fill("5000");
    await d.getByLabel(bruno.name, { exact: true }).fill("15000");
    await d.getByLabel(carla, { exact: true }).fill("10000");
  });

  // Ana owes 30,01, Bruno owes 119,99 and Carla is owed 150,00: zero in total, to the cent.
  await page.goto("/settlement");
  const balances = page.getByRole("listitem").filter({ hasText: "cota" });
  await expect(balances.filter({ hasText: ana.name })).toContainText("−R$ 30,01");
  await expect(balances.filter({ hasText: bruno.name })).toContainText("−R$ 119,99");
  await expect(balances.filter({ hasText: carla })).toContainText("+R$ 150,00");

  // The two suggested payments close it.
  for (const from of [bruno.name, ana.name]) {
    await page.getByRole("button", { name: `Registrar ${from} paga ${carla}` }).click();
    const pay = page.getByRole("dialog", { name: "Pagamento de acerto" });
    await pay.getByRole("button", { name: "Registrar" }).click();
    await expect(pay).toBeHidden();
  }
  await expect(page.getByText("Tudo acertado. Ninguém deve nada.")).toBeVisible();
  for (const name of [ana.name, bruno.name, carla]) {
    await expect(balances.filter({ hasText: name })).toContainText("R$ 0,00");
  }
});
