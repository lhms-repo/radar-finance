import { expect, test, type Page } from "@playwright/test";
import { isPhone, login, openShortcuts, switchSpace } from "./helpers";

/** Fixture address of the fictional receipt, with an access key and a CNPJ of the project apart from market.spec.ts. */
function receiptLink(project: string) {
  const idx = { "iphone-webkit": 1, "desktop-webkit": 2, "desktop-chromium": 3 }[project] ?? 9;
  return `https://nfce.fixture.gov.br/?p=fictional%7C${String(idx).repeat(43)}7%7C1122233300015${idx}`;
}

/** Reads the fictional receipt in the active group and links the coffee line to a new product of the given brand. */
async function readReceipt(page: Page, project: string, market: string, brand: string, otherBrand: string) {
  await page.goto("/market/receipt");
  await page.getByLabel("Ou cole o link da nota").fill(receiptLink(project));
  await page.getByRole("button", { name: "Ler", exact: true }).click();
  // The receipt is new to this group: no "already read", no market and no line already decided (RF-85).
  await expect(page.getByText("Mercado Ficticio Ltda")).toBeVisible();
  await expect(page.getByText("Essa nota já foi lida")).toBeHidden();
  await expect(page.getByLabel("Nome do mercado")).toHaveValue("Mercado Ficticio Ltda");
  const coffee = page.getByLabel("Produto de CAFE TRES COR 500G");
  await expect(coffee).not.toHaveValue(/\d+/);
  await expect(coffee.locator("option", { hasText: otherBrand })).toHaveCount(0);

  await page.getByLabel("Nome do mercado").fill(market);
  await coffee.selectOption({ label: "Novo produto..." });
  await page.getByLabel("Nome do produto de CAFE TRES COR 500G").fill("Café");
  await page.getByLabel("Marca de CAFE TRES COR 500G").fill(brand);
  await page.getByLabel("Tamanho de CAFE TRES COR 500G").fill("500");
  await page.getByLabel("Unidade de CAFE TRES COR 500G").selectOption("g");
  await page.getByLabel("Produto de BANANA PRATA KG").selectOption({ label: "Ignorar sempre neste mercado" });
  await page.getByRole("button", { name: "Registrar preços" }).click();
  await expect(page.getByText("Nota registrada")).toBeVisible();
}

/** Checks the products of the active group: its own coffee with the price of the receipt, never the one of the other group. */
async function seesOnly(page: Page, tag: string, own: string, other: string) {
  await page.goto("/market?tab=products");
  await page.getByLabel("Buscar produto").fill(tag);
  await expect(page.getByRole("button", { name: `Preços de Café ${own} 500 g` })).toContainText("R$ 18,99");
  await expect(page.getByRole("button", { name: `Preços de Café ${other} 500 g` })).toHaveCount(0);
}

test("two groups read the same receipt without mixing products or prices, with one person in both (RF-85, Fase 11)", async ({ page }, testInfo) => {
  test.slow();
  // Projects share one database, so the group, markets and brands carry the project name.
  const tag = testInfo.project.name;
  const beach = `Praia ${tag}`;
  const brands = { home: `Marca Casa ${tag}`, beach: `Marca Praia ${tag}` };

  // Ana is in Casa and creates the beach group, where she reads the receipt first.
  await login(page);
  await page.goto("/settings/group");
  await page.getByRole("button", { name: "Criar grupo" }).click();
  const create = page.getByRole("dialog", { name: "Criar grupo" });
  await create.getByLabel("Nome").fill(beach);
  await create.getByRole("button", { name: "Criar" }).click();
  await create.getByRole("button", { name: `Ir para ${beach}` }).click();
  await expect(page).toHaveURL(/\/$/);
  await readReceipt(page, tag, `Mercado Praia ${tag}`, brands.beach, brands.home);

  // The same receipt in Casa is new there and goes to products of Casa.
  await switchSpace(page, "Casa");
  await readReceipt(page, tag, `Mercado Casa ${tag}`, brands.home, brands.beach);
  await seesOnly(page, tag, brands.home, brands.beach);
  await page.goto("/market?tab=markets");
  await expect(page.getByText(`Mercado Casa ${tag}`, { exact: true })).toBeVisible();
  await expect(page.getByText(`Mercado Praia ${tag}`, { exact: true })).toHaveCount(0);

  // Back in the beach group, only its own catalog shows.
  await switchSpace(page, beach);
  await seesOnly(page, tag, brands.beach, brands.home);
  await page.goto("/market?tab=receipts");
  await expect(page.getByText(new RegExp(`Mercado Praia ${tag}, `))).toBeVisible();
  await expect(page.getByText(new RegExp(`Mercado Casa ${tag}, `))).toHaveCount(0);
});

/** Screens read in every space, with the market tabs and the settings pages. */
const SCREENS = [
  "/",
  "/cash-flow",
  "/cards",
  "/alelo",
  "/bills",
  "/savings",
  "/receivables",
  "/settlement",
  "/market",
  "/market?tab=markets",
  "/market?tab=products",
  "/market?tab=receipts",
  "/market/receipt",
  "/settings",
  "/settings/categories",
  "/settings/accounts",
  "/settings/incomes",
  "/settings/budgets",
  "/settings/import",
  "/settings/rules",
  "/settings/group",
];

/** Fails when the page shows the word casal anywhere, after its data had time to load. */
async function noCouple(page: Page, where: string) {
  await expect(page.locator("main")).toBeVisible();
  // Queries answer quickly on the local server; give them a moment so loaded texts are read too.
  await page.waitForTimeout(400);
  const text = await page.locator("body").innerText();
  expect(text, `${where} fala em casal`).not.toMatch(/casal/i);
}

test("no screen talks about a couple, in a group or in the personal space (Fase 11)", async ({ page }) => {
  test.slow();
  await page.goto("/login");
  await expect(page.getByRole("button", { name: "Casa", exact: true }).or(page.getByRole("button", { name: "Ana Ficticia" }))).toBeVisible();
  expect(await page.locator("body").innerText()).not.toMatch(/casal/i);

  await login(page);
  for (const space of ["Casa", "Pessoal"]) {
    if (space !== "Casa") await switchSpace(page, space);
    for (const path of SCREENS) {
      await page.goto(path);
      await noCouple(page, `${path} em ${space}`);
    }
    // The quick entry with a split between other people, where the parts of the space are named.
    await page.goto("/");
    await page.getByRole("button", { name: "Adicionar" }).click();
    const sheet = page.getByRole("dialog", { name: "Novo gasto" });
    await expect(sheet).toBeVisible();
    const split = sheet.getByText("Dividir com outras pessoas");
    if (await split.isVisible()) await split.click();
    await noCouple(page, `lançamento rápido em ${space}`);
    await page.keyboard.press("Escape");
    await expect(sheet).toBeHidden();
  }
  // Keep the shortcuts menu in the crawl on the desktop, where it is a menu of its own.
  if (!isPhone(page)) {
    await openShortcuts(page);
    await noCouple(page, "atalhos");
  }
});
