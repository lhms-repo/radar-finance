import { expect, test } from "@playwright/test";
import { login } from "./helpers";

test.beforeEach(async ({ page }) => {
  await login(page);
});

/** Returns an ISO date in the browser time zone, shifted by months, on the given day. */
function isoDay(monthsAgo: number, day: number) {
  const now = new Date();
  const d = new Date(now.getFullYear(), now.getMonth() - monthsAgo, day);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

test("imports: Nubank CSV with preview, alias rule, duplicates and undo (RF-53 to RF-57)", async ({ page }, testInfo) => {
  test.slow();
  // Projects share one database, so the card and the merchants carry the project name.
  const tag = testInfo.project.name;
  const cardName = `Cartão I ${tag}`;
  const pizza = `PIZZARIA FICT ${tag}`;
  const csv = [
    "date,title,amount",
    // Same shape as the current Nubank export: Brazilian amounts in quotes and "- " before the payment.
    `${isoDay(1, 5)},${pizza},"52,00"`,
    `${isoDay(1, 6)},Loja Ficticia ${tag} - Parcela 2/4,"100,00"`,
    `${isoDay(1, 9)},Estorno Loja ${tag},"-10,00"`,
    `${isoDay(1, 15)},Pagamento recebido,"- 1.500,00"`,
  ].join("\n");
  const file = { name: "fatura-ficticia.csv", mimeType: "text/csv", buffer: Buffer.from(csv) };

  await page.goto("/cards");
  await page.getByRole("button", { name: "Novo cartão" }).first().click();
  const form = page.getByRole("dialog", { name: "Novo cartão" });
  await form.getByLabel("Nome").fill(cardName);
  await form.getByLabel("Titular").selectOption({ label: "Ana Ficticia" });
  await form.getByLabel("Limite total").fill("500000");
  await form.getByLabel("Fecha dia").fill("1");
  await form.getByLabel("Vence dia").fill("8");
  await form.getByRole("button", { name: "Salvar" }).click();
  await expect(form).toBeHidden();

  await page.goto("/settings");
  await page.getByRole("link", { name: /Importar fatura/ }).click();
  await page.getByLabel("Cartão").selectOption({ label: cardName });
  await page.locator('input[type="file"]').setInputFiles(file);

  // The payment is already recorded by the invoice payment, so it stays out (RN-07).
  await expect(page.getByText("4 linhas para a fatura")).toBeVisible();
  await expect(page.getByText("Pagamento, já registrado pela fatura")).toBeVisible();
  await expect(page.getByText("Parcela 2/4", { exact: true })).toBeVisible();
  await expect(page.getByText("3 para importar")).toBeVisible();

  // A new name and category are remembered as a rule (RF-54, RF-55).
  await page.getByLabel(`Nome de ${pizza}`).fill(`Pizzaria ${tag}`);
  await page.getByLabel(`Categoria de ${pizza}`).selectOption({ label: "Delivery" });
  await page.getByRole("button", { name: "Importar", exact: true }).click();
  await expect(page.getByText("Fatura importada")).toBeVisible();
  const history = page.getByRole("listitem").filter({ hasText: cardName });
  await expect(history).toContainText("3 lançamentos");

  // The same file again: the rule applies and every line is flagged as a duplicate (RF-55, RF-56, RN-20).
  await page.locator('input[type="file"]').setInputFiles(file);
  await expect(page.getByLabel(`Nome de ${pizza}`)).toBeHidden();
  await expect(page.getByText("Possível duplicata")).toHaveCount(3);
  await expect(page.getByText("0 para importar")).toBeVisible();
  await page.getByLabel(`Importar ${pizza}`).check();
  await expect(page.getByLabel(`Nome de ${pizza}`)).toHaveValue(`Pizzaria ${tag}`);
  await page.getByRole("button", { name: "Cancelar" }).click();

  await history.getByRole("button", { name: "Desfazer" }).click();
  await expect(page.getByText("Importação desfeita")).toBeVisible();
  await expect(history).toBeHidden();

  // The rule is listed in Ajustes.
  await page.goto("/settings/rules");
  await expect(page.getByText(`Pizzaria ${tag}`)).toBeVisible();
  await expect(page.getByText(`${pizza.toLowerCase()} · Delivery`)).toBeVisible();
});
