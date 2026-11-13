import { expect, type Page } from "@playwright/test";

/** Fictional users created by e2e/server.sh in the group "Casa", with their fictional PINs. */
export const USERS = [
  { login: "ana", name: "Ana Ficticia", pin: "4821" },
  { login: "bruno", name: "Bruno Ficticio", pin: "7351" },
];

/** Picks the group "Casa" on the entry screen; its step shows only when other tests created more groups (RF-01). */
export async function chooseHome(page: Page, name = USERS[0]!.name) {
  const home = page.getByRole("button", { name: "Casa", exact: true });
  const person = page.getByRole("button", { name });
  await expect(home.or(person)).toBeVisible();
  if (await home.isVisible()) await home.click();
  await expect(person).toBeVisible();
}

/** Enters in the group "Casa", then picks the person and types the PIN (RF-01). */
export async function login(page: Page, name = USERS[0]!.name) {
  const user = USERS.find((u) => u.name === name)!;
  await page.goto("/login");
  await chooseHome(page, name);
  await page.getByRole("button", { name }).click();
  await page.getByLabel("PIN").fill(user.pin);
  await page.getByRole("button", { name: "Entrar" }).click();
  await expect(page).toHaveURL(/\/$/);
}

/** Opens the Add shortcuts: press and hold on the iPhone, the arrow beside the button on desktop (section 8.6). */
export async function openShortcuts(page: Page) {
  if (isPhone(page)) {
    const add = page.getByRole("button", { name: "Adicionar" });
    await add.dispatchEvent("pointerdown");
    await page.waitForTimeout(600);
    await add.dispatchEvent("pointerup");
  } else {
    await page.getByRole("button", { name: "Atalhos" }).click();
  }
}

/** Reports whether the project runs with a phone viewport. */
export function isPhone(page: Page) {
  return (page.viewportSize()?.width ?? 1280) < 768;
}

/** Switches the active space by its name in the selector of Ajustes, the personal one included (RF-81, RF-83). */
export async function switchSpace(page: Page, name: string) {
  await page.goto("/settings");
  await page.locator("main").getByLabel("Grupo").selectOption({ label: name });
  await expect(page).toHaveURL(/\/$/);
}
