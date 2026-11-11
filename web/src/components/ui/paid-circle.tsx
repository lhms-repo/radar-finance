import { motion } from "motion/react";

/** Circle that draws a check when the item is paid (section 8.6). */
export function PaidCircle({ paid, label, onClick }: { paid: boolean; label: string; onClick: () => void }) {
  return (
    <button type="button" onClick={onClick} aria-label={label} aria-pressed={paid} className="grid size-7 shrink-0 place-items-center">
      <svg viewBox="0 0 24 24" className="size-6" aria-hidden>
        <motion.circle
          cx="12"
          cy="12"
          r="10"
          fill="none"
          strokeWidth="1.8"
          className={paid ? "fill-success stroke-success" : "stroke-muted-foreground/50"}
          initial={false}
          animate={{ scale: paid ? [1, 1.12, 1] : 1 }}
          transition={{ duration: 0.3 }}
        />
        <motion.path
          d="M7.5 12.5l3 3 6-6.5"
          fill="none"
          stroke="white"
          strokeWidth="2.2"
          strokeLinecap="round"
          strokeLinejoin="round"
          initial={false}
          animate={{ pathLength: paid ? 1 : 0, opacity: paid ? 1 : 0 }}
          transition={{ duration: 0.3, ease: "easeOut" }}
        />
      </svg>
    </button>
  );
}
