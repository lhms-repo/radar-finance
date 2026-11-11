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

test("cards: register, purchase in installments, recurrence and invoice payment (RF-14 to RF-23)", async ({ page }, testInfo) => {
  // Long end to end flow; WebKit needs more than the default 30 seconds when the whole suite runs in parallel.
  test.slow();
  // Projects share one database, so names carry the project to stay unique.
  const tag = testInfo.project.name;
  const cardName = `Cartão ${tag}`;
  const account = `Conta C ${tag}`;

  await page.goto("/settings/accounts");
  await page.getByRole("button", { name: "Nova conta" }).first().click();
  const accountForm = page.getByRole("dialog", { name: "Nova conta" });
  await accountForm.getByLabel("Nome").fill(account);
  await accountForm.getByLabel("Titular").selectOption({ label: "Ana Ficticia" });
  await accountForm.getByLabel("Saldo inicial").fill("500000");
  await accountForm.getByRole("button", { name: "Salvar" }).click();
  await expect(accountForm).toBeHidden();

  await page.goto("/cards");
  await page.getByRole("button", { name: "Novo cartão" }).first().click();
  const form = page.getByRole("dialog", { name: "Novo cartão" });
  await form.getByLabel("Nome").fill(cardName);
  await form.getByLabel("Titular").selectOption({ label: "Ana Ficticia" });
  await form.getByLabel("Limite total").fill("500000");
  // Closes on the 1st and is due on the 8th, so last month's invoice is already overdue.
  await form.getByLabel("Fecha dia").fill("1");
  await form.getByLabel("Vence dia").fill("8");
  await form.getByLabel("Final").fill("4321");
  await form.getByRole("radio", { name: "Roxo" }).click();
  await form.getByRole("button", { name: "Salvar" }).click();
  await expect(form).toBeHidden();

  await page.getByRole("option", { name: cardName }).click();
  await expect(page.getByText(`${cardName} · Ana Ficticia`)).toBeVisible();

  // Purchase of R$ 100,00 in 3 installments, today: R$ 33,34 on the current invoice (RN-03).
  await page.getByRole("button", { name: "Compra", exact: true }).click();
  const purchase = page.getByRole("dialog", { name: `Compra no ${cardName}` });
  await purchase.getByLabel("Estabelecimento").fill("Loja Ficticia");
  await purchase.getByLabel("Valor total").fill("10000");
  await purchase.getByRole("button", { name: "Mais parcelas" }).click();
  await purchase.getByRole("button", { name: "Mais parcelas" }).click();
  await purchase.getByLabel("Categoria").selectOption({ label: "Delivery" });
  await purchase.getByRole("button", { name: "Lançar" }).click();
  await expect(purchase).toBeHidden();

  // The purchase may fall into this month's or next month's invoice, depending on today.
  const tile = page.getByRole("button", { name: /R\$ 33,34/ }).first();
  await tile.click();
  await expect(page.getByText("Loja Ficticia")).toBeVisible();
  await expect(page.getByText("1/3")).toBeVisible();
  await expect(page.getByText("Usado, com parcelas futuras").locator("..")).toContainText("R$ 100,00");

  // Recurrence shows up on the list.
  await page.getByRole("button", { name: "Nova", exact: true }).click();
  const recurrence = page.getByRole("dialog", { name: `Recorrência no ${cardName}` });
  await recurrence.getByLabel("Descrição").fill("Streaming ficticio");
  await recurrence.getByLabel("Valor").fill("2190");
  await recurrence.getByLabel("Categoria").selectOption({ label: "Streaming" });
  await recurrence.getByRole("button", { name: "Salvar" }).click();
  await expect(recurrence).toBeHidden();
  await expect(page.getByRole("button", { name: "Editar Streaming ficticio" })).toBeVisible();

  // A purchase on day 2 two months ago belongs to last month's invoice, already overdue.
  await page.getByRole("button", { name: "Compra", exact: true }).click();
  await purchase.getByLabel("Estabelecimento").fill("Mercado Antigo");
  await purchase.getByLabel("Valor total").fill("5000");
  await purchase.getByLabel("Categoria").selectOption({ label: "Delivery" });
  await purchase.getByLabel("Data da compra").fill(isoDay(2, 2));
  await purchase.getByRole("button", { name: "Lançar" }).click();
  await expect(purchase).toBeHidden();

  await page.goto("/cards");
  await page.getByRole("option", { name: cardName }).click();
  await page.getByRole("button", { name: "Mês anterior" }).click();
  await expect(page.getByText("Mercado Antigo")).toBeVisible();
  await page.getByRole("button", { name: "Pagar fatura" }).click();
  const pay = page.getByRole("dialog", { name: /Pagar fatura/ });
  await pay.getByLabel("Sai da conta").selectOption({ label: account });
  await pay.getByRole("button", { name: "Pagar" }).click();
  await expect(pay).toBeHidden();
  await expect(page.getByText(`com ${account}`)).toBeVisible();

  await page.goto("/settings/accounts");
  await expect(page.getByRole("link", { name: new RegExp(account) })).toContainText("R$ 4.950,00");
});
