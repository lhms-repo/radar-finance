import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

/** Text input in the shadcn/ui style, 16px on phones to avoid the iOS zoom. */
export function Input({ className, ...props }: ComponentProps<"input">) {
  return (
    <input
      className={cn(
        "h-11 w-full rounded-xl border border-input bg-card px-3 text-base outline-none transition-shadow placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/20 aria-invalid:border-destructive md:text-sm",
        className,
      )}
      {...props}
    />
  );
}

/** Form label. */
export function Label({ className, ...props }: ComponentProps<"label">) {
  return <label className={cn("text-sm font-medium", className)} {...props} />;
}
