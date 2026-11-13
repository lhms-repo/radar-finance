import { expect, test, type Page } from "@playwright/test";
import { isPhone, login, openShortcuts } from "./helpers";

test.beforeEach(async ({ page }) => {
  await login(page);
});

/** Opens one screen and then another, so the back button has an earlier screen to return to. */
async function from(page: Page, first: string, then: string) {
  await page.goto(first);
  await expect(page.locator("main")).toBeVisible();
  await page.goto(then);
  await expect(page.locator("main")).toBeVisible();
}

test("back closes the open sheet or dialog before leaving the screen (RNF-26)", async ({ page }) => {
  await from(page, "/", "/alelo");

  // A dialog: back closes it and the screen stays.
  await page.getByRole("button", { name: "Cartão Alelo" }).first().click();
  const form = page.getByRole("dialog", { name: "Novo cartão Alelo" });
  await expect(form).toBeVisible();
  await page.goBack();
  await expect(form).toBeHidden();
  await expect(page).toHaveURL(/\/alelo$/);

  // The quick entry, opened by Add.
  await page.getByRole("button", { name: "Adicionar" }).click();
  const sheet = page.getByRole("dialog", { name: "Novo gasto" });
  await expect(sheet).toBeVisible();
  await page.goBack();
  await expect(sheet).toBeHidden();
  await expect(page).toHaveURL(/\/alelo$/);

  // Closing by the screen leaves no extra step behind: the next back goes to the earlier screen.
  await page.getByRole("button", { name: "Cartão Alelo" }).first().click();
  await expect(form).toBeVisible();
  await form.getByRole("button", { name: "Cancelar" }).click();
  await expect(form).toBeHidden();
  await page.goBack();
  await expect(page).toHaveURL(/\/$/);
});

test("back closes the More sheet and the Add shortcuts on the phone (RNF-26)", async ({ page }) => {
  test.skip(!isPhone(page), "The More sheet and the fan of shortcuts are the phone bar.");
  await from(page, "/", "/cards");

  await page.getByRole("button", { name: "Mais" }).click();
  const more = page.getByRole("dialog", { name: "Mais telas" });
  await expect(more).toBeVisible();
  await page.goBack();
  await expect(more).toBeHidden();
  await expect(page).toHaveURL(/\/cards$/);

  await openShortcuts(page);
  const shortcut = page.getByRole("button", { name: "Compra no Alelo" });
  await expect(shortcut).toBeVisible();
  await page.goBack();
  await expect(shortcut).toBeHidden();
  await expect(page).toHaveURL(/\/cards$/);

  // Leaving through the More sheet replaces its step, so back returns to the screen it was opened on.
  await page.getByRole("button", { name: "Mais" }).click();
  await more.getByRole("link", { name: "Mercado" }).click();
  await expect(page).toHaveURL(/\/market$/);
  await page.goBack();
  await expect(page).toHaveURL(/\/cards$/);
});

test("the system bar follows the theme chosen in the app (RNF-23)", async ({ page }, testInfo) => {
  // The theme is saved for the user, and the projects run at the same time as Ana: only Android, whose bar it colors.
  test.skip(testInfo.project.name !== "android-chromium", "Only the Chrome of Android colors the system bar.");
  /** Reads every theme color the browser may apply to the system bar. */
  const colors = () => page.evaluate(() => [...document.querySelectorAll<HTMLMetaElement>('meta[name="theme-color"]')].map((m) => m.content));
  await page.goto("/settings");
  const saved = page.waitForResponse((r) => r.url().endsWith("/api/me/preferences") && r.ok());
  await page.getByRole("radiogroup", { name: "Tema" }).getByRole("radio", { name: "Escuro" }).click();
  await saved;
  await expect.poll(colors).toEqual(["#0b0d12"]);
  await page.reload();
  await expect.poll(colors).toEqual(["#0b0d12"]);
  await page.getByRole("radiogroup", { name: "Tema" }).getByRole("radio", { name: "Claro" }).click();
  await expect.poll(colors).toEqual(["#f4f5f7"]);
  // The theme is saved for the user, so the other tests get the system one back.
  await page.getByRole("radiogroup", { name: "Tema" }).getByRole("radio", { name: "Sistema" }).click();
  await expect.poll(colors).toEqual(["#f4f5f7"]);
});
