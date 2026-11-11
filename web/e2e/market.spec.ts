import { expect, test } from "@playwright/test";
import { login } from "./helpers";

test.beforeEach(async ({ page }) => {
  await login(page);
});

test("market: register market and branded product, record and correct prices (RF-65 to RF-67, RF-70, RF-76)", async ({ page }, testInfo) => {
  test.slow();
  // Projects share one database, so names carry the project to stay unique.
  const tag = testInfo.project.name;
  const market = `Mercado ${tag}`;
  const label = `Café Marca ${tag} 500 g`;

  await page.goto("/market?tab=markets");
  await page.getByRole("button", { name: "Mercado", exact: true }).click();
  const mf = page.getByRole("dialog", { name: "Novo mercado" });
  await mf.getByLabel("Nome").fill(market);
  await mf.getByLabel("Bairro").fill("Centro");
  await mf.getByRole("button", { name: "Salvar" }).click();
  await expect(mf).toBeHidden();
  await expect(page.getByText(market, { exact: true })).toBeVisible();

  await page.getByRole("radio", { name: "Produtos" }).click();
  await page.getByRole("button", { name: "Produto", exact: true }).click();
  const pf = page.getByRole("dialog", { name: "Novo produto" });
  await pf.getByLabel("Produto").fill("Café");
  await pf.getByLabel("Marca").fill(`Marca ${tag}`);
  await pf.getByLabel("Tamanho").fill("500");
  await pf.getByLabel("Unidade").selectOption("g");
  await pf.getByRole("button", { name: "Salvar" }).click();
  await expect(pf).toBeHidden();

  // A product without brand is refused (RF-66).
  await page.getByRole("button", { name: "Produto", exact: true }).click();
  await pf.getByLabel("Produto").fill("Arroz");
  await pf.getByLabel("Tamanho").fill("5");
  await pf.getByRole("button", { name: "Salvar" }).click();
  await expect(pf.getByText(/Informe a marca/)).toBeVisible();
  await pf.getByRole("button", { name: "Cancelar" }).click();

  await page.getByLabel("Buscar produto").fill(tag);
  await page.getByRole("button", { name: `Preços de ${label}` }).click();
  const history = page.getByRole("dialog", { name: label });
  await history.getByLabel("Mercado").selectOption({ label: `${market}, Centro` });
  await history.getByLabel("Preço").fill("1899");
  await history.getByRole("button", { name: "Registrar" }).click();
  await expect(history.getByText("R$ 18,99")).toBeVisible();

  // Correction keeps a single price, now R$ 17,49 (RF-70).
  await history.getByRole("button", { name: /Corrigir preço/ }).click();
  const fix = page.getByRole("dialog", { name: "Corrigir preço" });
  await fix.getByLabel("Preço").fill("1749");
  await fix.getByRole("button", { name: "Salvar" }).click();
  await expect(fix).toBeHidden();
  await expect(history.getByText("R$ 17,49")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("button", { name: `Preços de ${label}` })).toContainText("R$ 17,49");
});

/** Fixture address of the fictional receipt, with an access key and CNPJ of the project. */
function receiptLink(project: string, n: number) {
  const idx = { "iphone-webkit": 1, "desktop-webkit": 2, "desktop-chromium": 3 }[project] ?? 9;
  const key = String(idx).repeat(43) + n;
  return `https://nfce.fixture.gov.br/?p=fictional%7C${key}%7C1122233300014${idx}`;
}

test("market: read a receipt, link lines to products and recognize them on the next one (RF-68, RF-69)", async ({ page }, testInfo) => {
  test.slow();
  const tag = testInfo.project.name;
  const market = `Mercado NF ${tag}`;

  await page.goto("/market");
  await page.getByRole("button", { name: "Ler nota" }).click();
  await page.getByLabel("Ou cole o link da nota").fill(receiptLink(tag, 1));
  await page.getByRole("button", { name: "Ler", exact: true }).click();
  await expect(page.getByText("Mercado Ficticio Ltda")).toBeVisible();
  await expect(page.getByLabel("Nome do mercado")).toHaveValue("Mercado Ficticio Ltda");
  await expect(page.getByLabel("Bairro")).toHaveValue("Bairro Teste");
  await page.getByLabel("Nome do mercado").fill(market);

  await page.getByLabel("Produto de CAFE TRES COR 500G").selectOption({ label: "Novo produto..." });
  await page.getByLabel("Nome do produto de CAFE TRES COR 500G").fill("Café");
  await page.getByLabel("Marca de CAFE TRES COR 500G").fill(`Marca NF ${tag}`);
  await page.getByLabel("Tamanho de CAFE TRES COR 500G").fill("500");
  await page.getByLabel("Unidade de CAFE TRES COR 500G").selectOption("g");
  await page.getByLabel("Produto de BANANA PRATA KG").selectOption({ label: "Ignorar sempre neste mercado" });
  await page.getByRole("button", { name: "Registrar preços" }).click();
  await expect(page.getByText("Nota registrada")).toBeVisible();
  await expect(page).toHaveURL(/tab=receipts/);
  await expect(page.getByText(`${market}, 05/01/2026`)).toBeVisible();

  // The same receipt again is refused.
  await page.goto("/market/receipt");
  await page.getByLabel("Ou cole o link da nota").fill(receiptLink(tag, 1));
  await page.getByRole("button", { name: "Ler", exact: true }).click();
  await expect(page.getByText("Essa nota já foi lida")).toBeVisible();
  await expect(page.getByRole("button", { name: "Registrar preços" })).toBeDisabled();

  // Another receipt of the same store comes with the market and the lines already decided (RF-69).
  await page.getByRole("button", { name: "Ler outra" }).click();
  await page.getByLabel("Ou cole o link da nota").fill(receiptLink(tag, 2));
  await page.getByRole("button", { name: "Ler", exact: true }).click();
  await expect(page.getByLabel("Mercado", { exact: true })).toHaveValue(/\d+/);
  await expect(page.getByLabel("Produto de CAFE TRES COR 500G")).toHaveValue(/\d+/);
  await expect(page.getByLabel("Produto de BANANA PRATA KG")).toHaveValue("ignore");
  await page.getByRole("button", { name: "Registrar preços" }).click();
  await expect(page.getByText("Nota registrada")).toBeVisible();

  await page.getByRole("radio", { name: "Produtos" }).click();
  await page.getByLabel("Buscar produto").fill(`NF ${tag}`);
  await expect(page.getByRole("button", { name: `Preços de Café Marca NF ${tag} 500 g` })).toContainText("R$ 18,99");
});

test("market: a shopping list is shared live between the two users (RF-71)", async ({ page, browser }, testInfo) => {
  test.slow();
  const tag = testInfo.project.name;
  const brand = `Marca L ${tag}`;
  for (const p of [
    { name: "Arroz", brand, byWeight: false, sizeMilli: 5000, unit: "kg" },
    { name: "Banana", brand, byWeight: true, sizeMilli: 1000, unit: "kg" },
  ]) {
    expect((await page.request.post("/api/market/products", { data: p })).ok()).toBeTruthy();
  }

  await page.goto("/market");
  await page.getByLabel("Nome da nova lista").fill(`Compra ${tag}`);
  await page.getByRole("button", { name: "Criar" }).click();
  await page.getByRole("link", { name: new RegExp(`Compra ${tag}`) }).click();
  await expect(page.getByRole("heading", { name: `Compra ${tag}` })).toBeVisible();
  const listURL = page.url();

  // Bruno opens the same list on the other iPhone.
  const other = await browser.newContext({ baseURL: testInfo.project.use.baseURL, viewport: page.viewportSize() });
  const bruno = await other.newPage();
  await login(bruno, "Bruno Ficticio");
  await bruno.goto(listURL);
  await expect(bruno.getByText("Adicione os produtos da compra.")).toBeVisible();

  await page.getByLabel("Produto", { exact: true }).selectOption({ label: `Arroz ${brand} 5 kg` });
  await page.getByLabel("Quantidade", { exact: true }).fill("2");
  await page.getByRole("button", { name: "Pôr na lista" }).click();
  await page.getByLabel("Produto", { exact: true }).selectOption({ label: `Banana ${brand}, por kg` });
  await page.getByLabel("Kg", { exact: true }).fill("1,5");
  await page.getByRole("button", { name: "Pôr na lista" }).click();
  await expect(page.getByText("1,5 kg")).toBeVisible();

  // Without reloading, Bruno sees the items and picks the rice; Ana sees who picked it.
  await expect(bruno.getByText("2 un")).toBeVisible();
  await expect(bruno.getByText("1,5 kg")).toBeVisible();
  await bruno.getByRole("checkbox", { name: `Peguei Arroz ${brand} 5 kg` }).click();
  await expect(page.getByRole("checkbox", { name: `Peguei Arroz ${brand} 5 kg` })).toHaveAttribute("aria-checked", "true");
  await expect(page.getByText("pego por Bruno")).toBeVisible();
  await expect(page.getByText("1 de 2 itens pegos")).toBeVisible();

  await page.getByRole("button", { name: `Mais Arroz ${brand} 5 kg` }).click();
  await expect(bruno.getByText("3 un")).toBeVisible();
  await other.close();
});

test("market: ranking of the markets and best split of a list (RF-72, RF-73, RN-23, RN-24)", async ({ page }, testInfo) => {
  test.slow();
  const tag = testInfo.project.name;
  const day = new Date(Date.now() - 3 * 86_400_000);
  const date = `${day.getFullYear()}-${String(day.getMonth() + 1).padStart(2, "0")}-${String(day.getDate()).padStart(2, "0")}`;
  /** Posts JSON with the session of the page and returns the body. */
  const post = async (path: string, data: object) => {
    const res = await page.request.post(path, { data });
    expect(res.ok(), `${path}: ${await res.text()}`).toBeTruthy();
    return res.status() === 204 ? { id: 0 } : ((await res.json()) as { id: number });
  };
  const m1 = await post("/api/market/markets", { name: `Mercado Um ${tag}`, neighborhood: "" });
  const m2 = await post("/api/market/markets", { name: `Mercado Dois ${tag}`, neighborhood: "" });
  const p1 = await post("/api/market/products", { name: "Feijão", brand: `Marca R ${tag}`, byWeight: false, sizeMilli: 1000, unit: "kg" });
  const p2 = await post("/api/market/products", { name: "Azeite", brand: `Marca R ${tag}`, byWeight: false, sizeMilli: 500_000, unit: "ml" });
  // Um: 10,00 + 30,00 = 40,00. Dois: 25,00 + 14,00 = 39,00. Split: feijão no Um e azeite no Dois = 24,00.
  for (const [m, p, cents] of [
    [m1, p1, 1000],
    [m1, p2, 3000],
    [m2, p1, 2500],
    [m2, p2, 1400],
  ] as const) {
    await post("/api/market/prices", { productId: p.id, marketId: m.id, priceCents: cents, date });
  }
  const list = await post("/api/market/lists", { name: `Ranking ${tag}` });
  await post(`/api/market/lists/${list.id}/items`, { productId: p1.id, quantityMilli: 1000 });
  await post(`/api/market/lists/${list.id}/items`, { productId: p2.id, quantityMilli: 1000 });

  await page.goto(`/market/lists/${list.id}`);
  const ranking = page.getByRole("region", { name: "Ranking dos mercados" }).getByRole("listitem");
  await expect(ranking.nth(0)).toContainText(`Mercado Dois ${tag}`);
  await expect(ranking.nth(0)).toContainText("R$ 39,00");
  await expect(ranking.nth(1)).toContainText(`Mercado Um ${tag}`);
  await expect(page.getByText("Economia de R$ 15,00 sobre o melhor mercado único")).toBeVisible();
  await expect(page.getByText("Em 2 mercados, até 2 permitidos")).toBeVisible();
});
