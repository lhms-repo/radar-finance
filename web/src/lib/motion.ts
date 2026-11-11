import type { Transition } from "motion/react";

/** Spring used by sliding indicators across the navigation (section 5.5). */
export const spring: Transition = { type: "spring", stiffness: 500, damping: 38, mass: 0.8 };

/** Softer spring for larger surfaces like sheets and the tooltip. */
export const softSpring: Transition = { type: "spring", stiffness: 380, damping: 34 };

/** Short ease for fades, inside the 150 to 400 ms range of section 8.6. */
export const fade: Transition = { duration: 0.18, ease: "easeOut" };
