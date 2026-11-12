import { expect, type Page } from "@playwright/test";

/** Fictional users created by e2e/server.sh in the group "Casa", with their fictional PINs. */
export const USERS = [
  { login: "ana", name: "Ana Ficticia", pin: "4821" },
  { login: "bruno", name: "Bruno Ficticio", pin: "7351" },
];

/** Enters as a member of the only group: the group step is skipped, then the person and the PIN (RF-01). */
export async function login(page: Page, name = USERS[0]!.name) {
  const user = USERS.find((u) => u.name === name)!;
  await page.goto("/login");
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
