import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

/** White rounded surface, the base of every dashboard block. */
export function Card({ className, ...props }: ComponentProps<"section">) {
  return <section className={cn("rounded-2xl bg-card p-5 text-card-foreground shadow-card", className)} {...props} />;
}

/** Small bold title at the top of a card. */
export function CardTitle({ className, ...props }: ComponentProps<"h2">) {
  return <h2 className={cn("text-sm font-semibold", className)} {...props} />;
}

/** Muted helper text inside a card. */
export function CardDescription({ className, ...props }: ComponentProps<"p">) {
  return <p className={cn("text-sm text-muted-foreground", className)} {...props} />;
}
