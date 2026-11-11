import { AnimatePresence, motion } from "motion/react";
import type { ReactNode } from "react";
import { Label } from "@/components/ui/input";
import { fade } from "@/lib/motion";

/** Label, control and error message stacked, the base of every form row. */
export function Field({ label, htmlFor, error, children }: { label: string; htmlFor?: string; error?: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={htmlFor}>{label}</Label>
      {children}
      <FieldError message={error} />
    </div>
  );
}

/** Red helper text under a field, animated in and out. */
export function FieldError({ message }: { message?: string }) {
  return (
    <AnimatePresence initial={false}>
      {message && (
        <motion.p
          initial={{ opacity: 0, y: -4 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0 }}
          transition={fade}
          role="alert"
          className="text-sm text-destructive"
        >
          {message}
        </motion.p>
      )}
    </AnimatePresence>
  );
}
