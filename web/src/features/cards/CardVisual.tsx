import { Nfc } from "lucide-react";
import type { CardColor, CardFinish } from "@/features/cards/api";
import { formatAmount } from "@/lib/format";
import { cn } from "@/lib/utils";

/** Background of each card color, from the darker corner to the lighter one. */
const BACKGROUNDS: Record<CardColor, string> = {
  graphite: "linear-gradient(135deg, #3b3e44 0%, #1a1b1e 55%, #2e3035 100%)",
  purple: "linear-gradient(135deg, #8a2be2 0%, #5b1a9e 60%, #7a2fd0 100%)",
  blue: "linear-gradient(135deg, #2f6bff 0%, #1740b8 60%, #2a5ef0 100%)",
  green: "linear-gradient(135deg, #1f9d61 0%, #0f6b40 60%, #1c8c56 100%)",
  red: "linear-gradient(135deg, #e5484d 0%, #a51d26 60%, #d23a40 100%)",
  orange: "linear-gradient(135deg, #ff7a1a 0%, #e05a00 60%, #ff8a33 100%)",
  gold: "linear-gradient(135deg, #e8c66a 0%, #b08a2e 55%, #d9b456 100%)",
  silver: "linear-gradient(135deg, #e6e8ec 0%, #a9aeb6 55%, #d4d7dc 100%)",
};

/** Colors light enough to need dark text. */
const LIGHT: Partial<Record<CardColor, boolean>> = { gold: true, silver: true };

/** Brushed metal streaks laid over the color for the metal finish. */
const METAL =
  "repeating-linear-gradient(90deg, rgb(255 255 255 / 0.035) 0px, rgb(255 255 255 / 0.035) 1px, transparent 1px, transparent 3px), radial-gradient(120% 80% at 20% 0%, rgb(255 255 255 / 0.18), transparent 60%)";

/** Short labels of the invoice states shown on the card. */
const STATUS_LABEL = { open: "Aberta", closed: "Fechada", overdue: "Vencida", paid: "Paga" } as const;

/** Physical card drawing with name, holder, last digits, due date and free limit (section 5.4). */
export function CardVisual({
  name,
  personName,
  lastDigits,
  color,
  finish,
  dueLabel,
  limitFreeCents,
  status,
  className,
}: {
  name: string;
  personName: string;
  lastDigits?: string;
  color: CardColor;
  finish: CardFinish;
  dueLabel?: string;
  limitFreeCents?: number;
  status?: keyof typeof STATUS_LABEL;
  className?: string;
}) {
  const light = LIGHT[color];
  return (
    <div
      className={cn(
        "relative flex aspect-[1.586] w-full flex-col justify-between overflow-hidden rounded-2xl p-4 shadow-lg select-none sm:p-5",
        light ? "text-neutral-900" : "text-white",
        className,
      )}
      style={{ background: finish === "metal" ? `${METAL}, ${BACKGROUNDS[color]}` : BACKGROUNDS[color] }}
    >
      <div className="flex items-start justify-between gap-2">
        <span className="truncate text-sm font-semibold tracking-tight">{personName}</span>
        {status && (
          <span className={cn("shrink-0 rounded-full px-2 py-0.5 text-[0.65rem] font-medium backdrop-blur", light ? "bg-black/10" : "bg-white/15")}>
            {STATUS_LABEL[status]}
          </span>
        )}
      </div>
      <div className="flex items-center gap-3">
        <Chip />
        <span className="min-w-0 flex-1 truncate text-center text-sm font-medium tracking-[0.35em] uppercase opacity-90">{name}</span>
        <Nfc className="size-5 shrink-0 opacity-80" aria-hidden />
      </div>
      <div className="flex items-end justify-between gap-2 text-[0.65rem] tracking-wider uppercase">
        <div>
          <div className="opacity-60">{lastDigits ? `•••• ${lastDigits}` : "Vence"}</div>
          <div className="text-sm font-medium tracking-normal normal-case">{dueLabel ?? ""}</div>
        </div>
        {limitFreeCents !== undefined && (
          <div className="text-right">
            <div className="opacity-60">Limite livre</div>
            <div className="sensitive text-sm font-medium tracking-normal normal-case tabular-nums">
              {limitFreeCents < 0 ? "−" : ""}R$ {formatAmount(limitFreeCents)}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

/** Contact chip drawn in SVG. */
function Chip() {
  return (
    <svg viewBox="0 0 40 30" className="h-7 w-9 shrink-0" aria-hidden>
      <defs>
        <linearGradient id="chip-gold" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#f3dc9b" />
          <stop offset="1" stopColor="#b8913a" />
        </linearGradient>
      </defs>
      <rect x="1" y="1" width="38" height="28" rx="6" fill="url(#chip-gold)" />
      <path d="M1 10h12M1 20h12M27 10h12M27 20h12M13 1v28M27 1v28M13 15h14" stroke="#8a6a22" strokeWidth="1.2" fill="none" opacity="0.6" />
    </svg>
  );
}
