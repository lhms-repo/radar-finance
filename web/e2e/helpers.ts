import { expect, type Page } from "@playwright/test";

/** Fictional users created by e2e/server.sh. */
export const USERS = [
  { login: "ana", name: "Ana Ficticia" },
  { login: "bruno", name: "Bruno Ficticio" },
];

/** Picks a user on the "who is using" screen and waits for the app shell. */
export async function login(page: Page, name = USERS[0]!.name) {
  await page.goto("/login");
  await page.getByRole("button", { name }).click();
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
