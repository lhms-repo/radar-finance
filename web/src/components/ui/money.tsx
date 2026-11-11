import { formatAmount } from "@/lib/format";
import { cn } from "@/lib/utils";

/** Money with smaller cents, as in the dashboard reference, hidden by privacy mode (section 5). */
export function Money({
  cents,
  className,
  signed = false,
}: {
  cents: number;
  className?: string;
  /** Shows a plus sign on positive values, for statement lines. */
  signed?: boolean;
}) {
  const [whole, frac] = formatAmount(cents).split(",");
  const sign = cents < 0 ? "−" : signed && cents > 0 ? "+" : "";
  return (
    <span className={cn("sensitive whitespace-nowrap tabular-nums", className)}>
      {sign}R$ {whole}
      <span className="text-[0.7em] opacity-70">,{frac}</span>
    </span>
  );
}
