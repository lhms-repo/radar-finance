import { expect, test } from "@playwright/test";
import { USERS, login, switchSpace } from "./helpers";

/** Returns the 2nd of the current month as an ISO date, in the browser time zone. */
function secondOfMonth() {
  const now = new Date();
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}-02`;
}

test("a purchase of the beach group on the card of Ana shows only there, and the whole invoice only to Ana (RF-83, Fase 9)", async ({ page }, testInfo) => {
  test.slow();
  // Projects share one database, so names carry the project to stay unique.
  const tag = testInfo.project.name;
  const beach = `Praia ${tag}`;
  const cardName = `Cartão P ${tag}`;
  const kiosk = `Quiosque ${tag}`;
  const bakery = `Padaria ${tag}`;

  await login(page);
  // The card of Ana, registered in Casa, with a purchase of Casa.
  await page.goto("/cards");
  await page.getByRole("button", { name: "Novo cartão" }).first().click();
  const form = page.getByRole("dialog", { name: "Novo cartão" });
  await form.getByLabel("Nome").fill(cardName);
  await form.getByLabel("Titular").selectOption({ label: USERS[0]!.name });
  await form.getByLabel("Limite total").fill("500000");
  // Closes on the 20th and is due on the 28th, and the purchases are on the 2nd, so they land on the invoice
  // of this month, the one the page opens on.
  await form.getByLabel("Fecha dia").fill("20");
  await form.getByLabel("Vence dia").fill("28");
  await form.getByRole("radio", { name: "Roxo" }).click();
  await form.getByRole("button", { name: "Salvar" }).click();
  await expect(form).toBeHidden();

  /** Registers a purchase today on the card, in the active space. */
  const buy = async (merchant: string) => {
    await page.goto("/cards");
    await page.getByRole("option", { name: cardName }).click();
    await page.getByRole("button", { name: "Compra", exact: true }).click();
    const purchase = page.getByRole("dialog", { name: `Compra no ${cardName}` });
    await purchase.getByLabel("Estabelecimento").fill(merchant);
    await purchase.getByLabel("Valor total").fill("4000");
    await purchase.getByLabel("Categoria").selectOption({ label: "Delivery" });
    await purchase.getByLabel("Data da compra").fill(secondOfMonth());
    await purchase.getByRole("button", { name: "Lançar" }).click();
    await expect(purchase).toBeHidden();
  };
  await buy(bakery);

  // Ana creates the beach group and buys there with the same card.
  await page.goto("/settings/group");
  await page.getByRole("button", { name: "Criar grupo" }).click();
  const create = page.getByRole("dialog", { name: "Criar grupo" });
  await create.getByLabel("Nome").fill(beach);
  await create.getByRole("button", { name: "Criar" }).click();
  await create.getByRole("button", { name: `Ir para ${beach}` }).click();
  await expect(page).toHaveURL(/\/$/);
  await buy(kiosk);

  /** Opens the card in the active space and returns the text of its invoice. */
  const invoice = async () => {
    await page.goto("/cards");
    await page.getByRole("option", { name: cardName }).click();
    await expect(page.getByText(`${cardName} · ${USERS[0]!.name}`)).toBeVisible();
  };

  // Casa reads only its own line.
  await switchSpace(page, "Casa");
  await invoice();
  await expect(page.getByText(bakery)).toBeVisible();
  await expect(page.getByText(kiosk)).toHaveCount(0);

  // The beach group reads only the kiosk.
  await switchSpace(page, beach);
  await invoice();
  await expect(page.getByText(kiosk)).toBeVisible();
  await expect(page.getByText(bakery)).toHaveCount(0);

  // Ana reads the whole invoice in her personal space, with the space of each line.
  await switchSpace(page, "Pessoal");
  await invoice();
  await expect(page.getByText(kiosk)).toBeVisible();
  await expect(page.getByText(bakery)).toBeVisible();
  await expect(page.getByText(new RegExp(`^${beach} · `))).toBeVisible();

  // Bruno, in Casa, sees the card without the beach line and without the use of the limit.
  await page.goto("/settings");
  await page.getByRole("button", { name: `Trocar de usuário (${USERS[0]!.name})` }).click();
  await login(page, USERS[1]!.name);
  await invoice();
  await expect(page.getByText(bakery)).toBeVisible();
  await expect(page.getByText(kiosk)).toHaveCount(0);
  await expect(page.getByText(`O limite usado aparece só para ${USERS[0]!.name}.`)).toBeVisible();
  // In his personal space the card of Ana is not there.
  await switchSpace(page, "Pessoal");
  await page.goto("/cards");
  await expect(page.getByRole("option", { name: cardName })).toHaveCount(0);
});
