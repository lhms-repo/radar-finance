import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { flushSync } from "react-dom";
import { setHapticsEnabled } from "./haptics";
import { prefersReducedMotion, readStorage, writeStorage } from "./utils";
import type { Theme } from "@/api/client";

const THEME_KEY = "radar.theme";
const PRIVACY_KEY = "radar.privacy";
const HAPTICS_KEY = "radar.haptics";

type Preferences = {
  theme: Theme;
  resolvedTheme: "light" | "dark";
  setTheme: (theme: Theme, origin?: { x: number; y: number }) => void;
  privacy: boolean;
  togglePrivacy: () => void;
  haptics: boolean;
  setHaptics: (value: boolean) => void;
};

const PreferencesContext = createContext<Preferences | null>(null);

/** Resolves "system" to the current operating system theme. */
function resolveTheme(theme: Theme): "light" | "dark" {
  if (theme === "system") {
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  }
  return theme;
}

/** Background of each theme, also used by Android for the system bar (RNF-23); public/theme-init.js repeats it. */
const THEME_COLORS = { light: "#f4f5f7", dark: "#0b0d12" } as const;

/** Applies the resolved theme to the root element and to the color of the system bar. */
function applyTheme(resolved: "light" | "dark") {
  document.documentElement.classList.toggle("dark", resolved === "dark");
  applyThemeColor(resolved);
}

/** Leaves a single theme color with the theme chosen in the app, instead of the one of the operating system. */
function applyThemeColor(resolved: "light" | "dark") {
  const metas = document.querySelectorAll<HTMLMetaElement>('meta[name="theme-color"]');
  metas.forEach((m, i) => {
    if (i > 0) m.remove();
  });
  let meta = metas[0];
  if (!meta) {
    meta = document.createElement("meta");
    meta.name = "theme-color";
    document.head.appendChild(meta);
  }
  meta.removeAttribute("media");
  meta.content = THEME_COLORS[resolved];
}

/** Runs a DOM update with a circular reveal from origin, when supported (section 8.6). */
function withCircularReveal(update: () => void, origin?: { x: number; y: number }) {
  const doc = document as Document & { startViewTransition?: (cb: () => void) => { ready: Promise<void> } };
  if (!origin || !doc.startViewTransition || prefersReducedMotion()) {
    update();
    return;
  }
  const transition = doc.startViewTransition(() => flushSync(update));
  const radius = Math.hypot(Math.max(origin.x, innerWidth - origin.x), Math.max(origin.y, innerHeight - origin.y));
  transition.ready
    .then(() => {
      document.documentElement.animate(
        { clipPath: [`circle(0px at ${origin.x}px ${origin.y}px)`, `circle(${radius}px at ${origin.x}px ${origin.y}px)`] },
        { duration: 400, easing: "cubic-bezier(0.4, 0, 0.2, 1)", pseudoElement: "::view-transition-new(root)" },
      );
    })
    .catch(() => undefined);
}

/** Provides theme, privacy mode and haptics to the whole app. */
export function PreferencesProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(() => (readStorage(THEME_KEY) as Theme | null) ?? "system");
  const [resolvedTheme, setResolvedTheme] = useState<"light" | "dark">(() => resolveTheme(theme));
  const [privacy, setPrivacy] = useState(() => readStorage(PRIVACY_KEY) === "1");
  const [haptics, setHapticsState] = useState(() => readStorage(HAPTICS_KEY) !== "0");

  useEffect(() => {
    if (theme !== "system") return;
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const onChange = () => {
      const next = resolveTheme("system");
      applyTheme(next);
      setResolvedTheme(next);
    };
    media.addEventListener("change", onChange);
    return () => media.removeEventListener("change", onChange);
  }, [theme]);

  useEffect(() => {
    document.documentElement.classList.toggle("privacy", privacy);
  }, [privacy]);

  useEffect(() => {
    setHapticsEnabled(haptics);
  }, [haptics]);

  const setTheme = useCallback((next: Theme, origin?: { x: number; y: number }) => {
    writeStorage(THEME_KEY, next);
    const resolved = resolveTheme(next);
    withCircularReveal(() => {
      applyTheme(resolved);
      setThemeState(next);
      setResolvedTheme(resolved);
    }, origin);
  }, []);

  const togglePrivacy = useCallback(() => {
    setPrivacy((p) => {
      writeStorage(PRIVACY_KEY, p ? "0" : "1");
      return !p;
    });
  }, []);

  const setHaptics = useCallback((value: boolean) => {
    writeStorage(HAPTICS_KEY, value ? "1" : "0");
    setHapticsState(value);
  }, []);

  const value = useMemo(
    () => ({ theme, resolvedTheme, setTheme, privacy, togglePrivacy, haptics, setHaptics }),
    [theme, resolvedTheme, setTheme, privacy, togglePrivacy, haptics, setHaptics],
  );
  return <PreferencesContext.Provider value={value}>{children}</PreferencesContext.Provider>;
}

/** Returns the app preferences. */
export function usePreferences() {
  const ctx = useContext(PreferencesContext);
  if (!ctx) throw new Error("usePreferences must be used inside PreferencesProvider");
  return ctx;
}
