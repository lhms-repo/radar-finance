import { motion } from "motion/react";
import { prefersReducedMotion } from "@/lib/utils";

/** Colors of the confetti pieces, the success greens with a touch of the brand blue. */
const COLORS = ["#16a34a", "#4ade80", "#22c55e", "#2563eb", "#facc15"];

/** Pieces with fixed pseudo random directions, so renders are stable. */
const PIECES = Array.from({ length: 28 }, (_, i) => ({
  angle: (i * 137.5 * Math.PI) / 180,
  distance: 60 + ((i * 37) % 70),
  rotate: (i * 53) % 360,
  color: COLORS[i % COLORS.length]!,
  size: 5 + (i % 3) * 2,
}));

/** Burst of confetti from the center of the parent, for a finished savings box (section 8.6). */
export function Confetti() {
  if (prefersReducedMotion()) return null;
  return (
    <div className="pointer-events-none absolute inset-0 overflow-visible" aria-hidden>
      {PIECES.map((p, i) => (
        <motion.span
          key={i}
          className="absolute left-1/2 top-1/2 rounded-[2px]"
          style={{ width: p.size, height: p.size * 1.6, background: p.color }}
          initial={{ x: 0, y: 0, opacity: 1, rotate: 0 }}
          animate={{ x: Math.cos(p.angle) * p.distance, y: [0, Math.sin(p.angle) * p.distance - 30, Math.sin(p.angle) * p.distance + 60], opacity: [1, 1, 0], rotate: p.rotate + 360 }}
          transition={{ duration: 1.2, ease: "easeOut" }}
        />
      ))}
    </div>
  );
}
