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
