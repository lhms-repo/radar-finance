import { useId, useState } from "react";
import { formatAmount } from "@/lib/format";
import { cn } from "@/lib/utils";

/** Largest number of digits accepted, enough for billions in cents. */
const MAX_DIGITS = 12;

/** Amount field in integer cents: each digit typed enters from the right, like bank apps (RN-21). */
export function MoneyInput({
  value,
  onChange,
  allowNegative = false,
  id,
  invalid,
  autoFocus,
  ariaLabel,
}: {
  value: number;
  onChange: (cents: number) => void;
  allowNegative?: boolean;
  id?: string;
  invalid?: boolean;
  autoFocus?: boolean;
  ariaLabel?: string;
}) {
  const fallbackId = useId();
  // Zero has no sign, so the sign chosen here is kept with the value it was chosen for. A zero that
  // came from outside, like a form reset after a negative amount, starts positive again.
  const [chosen, setChosen] = useState({ negative: false, at: 0 });
  const negative = value === 0 ? allowNegative && chosen.negative && chosen.at === 0 : value < 0;
  return (
    <div
      className={cn(
        "flex h-11 items-center gap-2 rounded-xl border border-input bg-card px-3 focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/20",
        invalid && "border-destructive",
      )}
    >
      {allowNegative && (
        <button
          type="button"
          onClick={() => {
            setChosen({ negative: !negative, at: -value });
            if (value !== 0) onChange(-value);
          }}
          aria-label={negative ? "Tornar positivo" : "Tornar negativo"}
          aria-pressed={negative}
          className={cn("rounded-lg px-2 py-0.5 text-sm font-medium", negative ? "bg-destructive/10 text-destructive" : "bg-muted text-muted-foreground")}
        >
          {negative ? "−" : "+"}
        </button>
      )}
      <span className="text-muted-foreground">R$</span>
      <input
        id={id ?? fallbackId}
        aria-label={ariaLabel}
        aria-invalid={invalid}
        autoFocus={autoFocus}
        inputMode="numeric"
        autoComplete="off"
        className="h-full min-w-0 flex-1 bg-transparent text-base tabular-nums outline-none md:text-sm"
        value={formatAmount(value)}
        onChange={(e) => {
          const digits = e.target.value.replace(/\D/g, "").replace(/^0+/, "").slice(0, MAX_DIGITS);
          const cents = digits === "" ? 0 : Number(digits);
          const next = negative ? -cents : cents;
          setChosen({ negative, at: next });
          onChange(next);
        }}
      />
    </div>
  );
}
