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
  nubank_croma: "linear-gradient(135deg, #dcd6c4 0%, #c4bda3 42%, #d2ccb8 68%, #b9b199 100%)",
};

/** Purple of the Nubank logo printed on the Croma. */
const NU_PURPLE = "#7029b8";

/** Fine grain of the Croma chrome paint, as SVG noise. */
const GRAIN =
  "url(\"data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='140' height='140'%3E%3Cfilter id='g'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='2' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23g)'/%3E%3C/svg%3E\")";

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
  if (color === "nubank_croma")
    return <CromaVisual personName={personName} lastDigits={lastDigits} dueLabel={dueLabel} limitFreeCents={limitFreeCents} status={status} className={className} />;
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

/** Nubank Croma face: chrome gray with grain, purple logo, Mastercard mark and outlined chip. */
function CromaVisual({
  personName,
  lastDigits,
  dueLabel,
  limitFreeCents,
  status,
  className,
}: {
  personName: string;
  lastDigits?: string;
  dueLabel?: string;
  limitFreeCents?: number;
  status?: keyof typeof STATUS_LABEL;
  className?: string;
}) {
  return (
    <div
      className={cn("relative flex aspect-[1.586] w-full flex-col justify-between overflow-hidden rounded-2xl p-4 shadow-lg select-none sm:p-5", className)}
      style={{ background: BACKGROUNDS.nubank_croma, color: "#4e2a7d" }}
    >
      <div className="pointer-events-none absolute inset-0 opacity-[0.22] mix-blend-overlay" style={{ backgroundImage: GRAIN }} aria-hidden />
      <div
        className="pointer-events-none absolute inset-0"
        style={{ background: "linear-gradient(115deg, transparent 25%, rgb(255 255 255 / 0.28) 42%, transparent 58%)" }}
        aria-hidden
      />
      <div className="relative flex items-start justify-between gap-2">
        <NuLogo className="h-6 sm:h-7" />
        <span className="flex items-center gap-2">
          {status && <span className="rounded-full bg-black/10 px-2 py-0.5 text-[0.65rem] font-medium">{STATUS_LABEL[status]}</span>}
          <MastercardMark className="h-7 sm:h-8" />
        </span>
      </div>
      <div className="relative flex items-center justify-between gap-2">
        <div className="text-[0.65rem] tracking-wider uppercase">
          <div className="opacity-70">{lastDigits ? `•••• ${lastDigits}` : "Vence"}</div>
          <div className="text-sm font-medium tracking-normal normal-case">{dueLabel ?? ""}</div>
        </div>
        <OutlineChip />
      </div>
      <div className="relative flex items-end justify-between gap-2">
        <span className="truncate text-sm font-medium tracking-wide uppercase">{personName}</span>
        {limitFreeCents !== undefined && (
          <div className="shrink-0 text-right text-[0.65rem] tracking-wider uppercase">
            <div className="opacity-70">Limite livre</div>
            <div className="sensitive text-sm font-medium tracking-normal normal-case tabular-nums">
              {limitFreeCents < 0 ? "−" : ""}R$ {formatAmount(limitFreeCents)}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

/** Nubank "nu" wordmark: the "u" is the "n" turned upside down. */
function NuLogo({ className }: { className?: string }) {
  const n = "M0 58V4Q0 0 4 0H26A20 20 0 0 1 46 20V58H34V22A10 10 0 0 0 24 12H12V58Z";
  return (
    <svg viewBox="0 0 100 58" className={cn("w-auto", className)} fill={NU_PURPLE} aria-label="Nubank" role="img">
      <path d={n} />
      <path d={n} transform="translate(100 58) rotate(180)" />
    </svg>
  );
}

/** Mastercard mark: red and yellow circles with the orange overlap. */
function MastercardMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 40 26" className={cn("w-auto", className)} aria-label="Mastercard" role="img">
      <circle cx="13" cy="13" r="12" fill="#e2452a" />
      <circle cx="27" cy="13" r="12" fill="#eeab3d" />
      <path d="M20 3.25A12 12 0 0 1 20 22.75A12 12 0 0 1 20 3.25Z" fill="#ea7a2c" />
    </svg>
  );
}

/** Chip drawn only in outline, as printed on the Croma. */
function OutlineChip() {
  return (
    <svg viewBox="0 0 40 30" className="mr-[12%] h-7 w-9 shrink-0" aria-hidden>
      <rect x="1" y="1" width="38" height="28" rx="5" fill="rgb(255 255 255 / 0.12)" stroke="#3f3a35" strokeWidth="1.3" />
      <path d="M1 10h10M1 20h10M29 10h10M29 20h10M14 29V13a6 6 0 0 1 12 0v16" stroke="#3f3a35" strokeWidth="1.3" fill="none" />
    </svg>
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
