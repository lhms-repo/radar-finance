import { expect, test } from "@playwright/test";
import { USERS, login } from "./helpers";

test.beforeEach(async ({ page }) => {
  await login(page);
});

/** A date of the current month in the browser time zone, as the statement writes it and as the API takes it. */
function day(n: number) {
  const now = new Date();
  const d = new Date(now.getFullYear(), now.getMonth(), Math.min(n, now.getDate()));
  const dd = String(d.getDate()).padStart(2, "0");
  const mm = String(d.getMonth() + 1).padStart(2, "0");
  return { br: `${dd}/${mm}/${d.getFullYear()}`, iso: `${d.getFullYear()}-${mm}-${dd}`, month: `${d.getFullYear()}-${mm}` };
}

test("statement: Santander CSV detected, salary confirming the income, duplicates on a second read and undo (RF-86 to RF-90, Fase 13)", async ({ page }, testInfo) => {
  test.slow();
  // Projects share one database, so the account, the income and the merchants carry the project name.
  const tag = testInfo.project.name;
  const account = `Conta S ${tag}`;
  const salary = `Salário S ${tag}`;
  const bakery = `PADARIA FICT ${tag}`;
  const persons = (await (await page.request.get("/api/persons")).json()) as { id: number; name: string }[];
  const ana = persons.find((p) => p.name === USERS[0]!.name)!;
  const created = await page.request.post("/api/accounts", { data: { name: account, personId: ana.id, initialBalanceCents: 0 } });
  expect(created.ok()).toBeTruthy();
  const { id: accountId } = (await created.json()) as { id: number };
  const income = await page.request.post("/api/incomes", {
    data: { kind: "recurring", name: salary, personId: ana.id, accountId, amountCents: 250_000, day: 1, startMonth: day(1).month },
  });
  expect(income.ok(), await income.text()).toBeTruthy();

  const csv = [
    "Extrato de conta corrente",
    "Cliente;CLIENTE FICTICIO",
    "",
    "Lançamentos:",
    "Data;Descrição;Detalhe;Valor;Saldo",
    `${day(3).br};Compra cartao deb mc;${day(2).br.slice(0, 5)} ${bakery};-R$ 23,50;R$ 2.334,03`,
    `${day(2).br};Tarifa mensalidade pacote servicos;;-R$ 32,90;R$ 2.357,53`,
    `${day(1).br};Pagamento cartao credito bce;;-R$ 450,00;R$ 2.390,43`,
    `${day(1).br};Credito de salario;EMPRESA FICTICIA;R$ 2.500,00;R$ 2.840,43`,
  ].join("\n");
  const file = { name: "extrato-ficticio.csv", mimeType: "text/csv", buffer: Buffer.from(csv) };

  await page.goto("/settings/import");
  await page.getByRole("radiogroup", { name: "Tipo de arquivo" }).getByRole("radio", { name: "Extrato do Santander" }).click();
  await page.getByLabel("Conta").selectOption({ label: account });
  await page.locator('input[type="file"]').setInputFiles(file);

  // Each line says what it becomes; the card payment has no closed invoice with that total, so it stays out (RF-89).
  await expect(page.getByText(`4 linhas para ${account}`)).toBeVisible();
  await expect(page.getByText(`Salário, confirma ${salary}`)).toBeVisible();
  await expect(page.getByText("Pagamento de fatura sem fatura fechada com esse valor")).toBeVisible();
  await expect(page.getByLabel(`Categoria de Tarifa mensalidade pacote servicos`)).toHaveValue(/\d+/);
  await expect(page.getByText("3 para importar")).toBeVisible();
  await page.getByLabel(`Nome de ${bakery}`).fill(`Padaria ${tag}`);
  await page.getByLabel(`Categoria de ${bakery}`).selectOption({ label: "Delivery" });
  await page.getByRole("button", { name: "Importar", exact: true }).click();
  await expect(page.getByText("Extrato importado")).toBeVisible();

  // The closing balance of the statement is compared with the account (RF-90).
  await expect(page.getByText("Saldo no extrato").locator("..")).toContainText("R$ 2.334,03");
  const history = page.getByRole("listitem").filter({ hasText: account });
  await expect(history).toContainText("3 lançamentos");

  // The income is now confirmed.
  const incomes = (await (await page.request.get(`/api/incomes/month?month=${day(1).month}`)).json()) as { occurrences: { name: string; confirmed: boolean }[] };
  expect(incomes.occurrences.find((o) => o.name === salary)?.confirmed).toBe(true);

  // The same statement again: the rule applies and the imported lines are duplicates (RN-32).
  await page.goto("/settings/import");
  await page.getByRole("radiogroup", { name: "Tipo de arquivo" }).getByRole("radio", { name: "Extrato do Santander" }).click();
  await page.getByLabel("Conta").selectOption({ label: account });
  await page.locator('input[type="file"]').setInputFiles(file);
  await expect(page.getByText("Possível duplicata")).toHaveCount(3);
  await expect(page.getByText("0 para importar")).toBeVisible();
  await page.getByRole("button", { name: "Cancelar" }).click();

  await history.getByRole("button", { name: /Desfazer/ }).click();
  await expect(page.getByText("Importação desfeita")).toBeVisible();
  await expect(history).toBeHidden();
});
