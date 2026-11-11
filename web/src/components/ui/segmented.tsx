import { motion } from "motion/react";
import { useId } from "react";
import { cn } from "@/lib/utils";
import { spring } from "@/lib/motion";

type Option<T extends string> = { value: T; label: string; icon?: React.ReactNode };

/** Segmented control whose highlight slides between options with a spring. */
export function Segmented<T extends string>({
  value,
  options,
  onChange,
  className,
  ariaLabel,
}: {
  value: T;
  options: Option<T>[];
  onChange: (value: T, event: React.MouseEvent<HTMLButtonElement>) => void;
  className?: string;
  ariaLabel: string;
}) {
  const id = useId();
  return (
    <div role="radiogroup" aria-label={ariaLabel} className={cn("inline-flex rounded-xl bg-muted p-1", className)}>
      {options.map((opt) => {
        const active = opt.value === value;
        return (
          <button
            key={opt.value}
            type="button"
            role="radio"
            aria-checked={active}
            onClick={(e) => onChange(opt.value, e)}
            className={cn(
              "relative flex flex-1 items-center justify-center gap-1.5 rounded-lg px-3 py-1.5 text-sm transition-colors",
              active ? "text-foreground" : "text-muted-foreground hover:text-foreground",
            )}
          >
            {active && (
              <motion.span
                layoutId={`segmented-${id}`}
                transition={spring}
                className="absolute inset-0 rounded-lg bg-card shadow-sm"
              />
            )}
            <span className="relative flex items-center gap-1.5">
              {opt.icon}
              {opt.label}
            </span>
          </button>
        );
      })}
    </div>
  );
}
