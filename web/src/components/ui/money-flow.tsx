import NumberFlow from "@number-flow/react";
import { cn } from "@/lib/utils";

const format = { style: "currency", currency: "BRL" } as const;

/** Money that rolls digit by digit when the value changes, with smaller cents (section 8.6). */
export function MoneyFlow({ cents, className }: { cents: number; className?: string }) {
  return <NumberFlow value={cents / 100} locales="pt-BR" format={format} className={cn("money-flow sensitive tabular-nums", className)} />;
}
