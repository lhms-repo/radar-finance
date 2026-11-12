import type { Transition } from "motion/react";
import { useSyncExternalStore } from "react";

/** Spring used by sliding indicators across the navigation (section 5.5). */
export const spring: Transition = { type: "spring", stiffness: 500, damping: 38, mass: 0.8 };

/** Softer spring for larger surfaces like sheets and the tooltip. */
export const softSpring: Transition = { type: "spring", stiffness: 380, damping: 34 };

/** Short ease for fades, inside the 150 to 400 ms range of section 8.6. */
export const fade: Transition = { duration: 0.18, ease: "easeOut" };

/** Media query of the system setting for less motion. */
const REDUCED_QUERY = "(prefers-reduced-motion: reduce)";

/** Subscribes to changes of the reduced motion setting. */
function subscribeReduced(onChange: () => void) {
  if (typeof window === "undefined" || !window.matchMedia) return () => {};
  const mq = window.matchMedia(REDUCED_QUERY);
  mq.addEventListener("change", onChange);
  return () => mq.removeEventListener("change", onChange);
}

/** Reads the reduced motion setting now. */
function readReduced() {
  return typeof window !== "undefined" && !!window.matchMedia && window.matchMedia(REDUCED_QUERY).matches;
}

/** Tells whether the system asks for less motion, following changes while the app is open; Motion useReducedMotion reads it only once. */
export function usePrefersReducedMotion() {
  return useSyncExternalStore(subscribeReduced, readReduced, () => false);
}
