import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

/** Lime of the Alelo Tudo card. */
const LIME = "#c3d238";
/** Green of every drawing and text on the Alelo Tudo card. */
const GREEN = "#067854";

/** Vertical Alelo Tudo card: the line runs through the chip and becomes the stem of the "d" in "tudo". */
export function AleloTudoVisual({ personName, name, className, children }: { personName: string; name: string; className?: string; children?: ReactNode }) {
  return (
    <div
      className={cn("@container relative aspect-[0.633] w-full overflow-hidden rounded-2xl shadow-lg select-none", className)}
      style={{ background: `radial-gradient(120% 70% at 20% 0%, rgb(255 255 255 / 0.18), transparent 60%), ${LIME}` }}
    >
      <svg viewBox="0 0 100 158" className="absolute inset-0 size-full" aria-hidden>
        <defs>
          <linearGradient id="alelo-chip" x1="0" y1="0" x2="1" y2="1">
            <stop offset="0" stopColor="#f2d9a2" />
            <stop offset="1" stopColor="#c9a165" />
          </linearGradient>
        </defs>
        <text x="4" y="37" fill={GREEN} fontSize="3.4" fontWeight="700" textLength="38.5" lengthAdjust="spacingAndGlyphs">
          USE DO SEU JEITO =)
        </text>
        <g fill="none" stroke={GREEN} strokeWidth="1.3" strokeLinecap="round">
          <path d="M51 -1 V121" strokeLinecap="butt" />
          <path d="M16 103 V117.5 Q16 121 19.5 121 H21 M12.5 109 H20.5" />
          <path d="M24.5 109 V115 A6 6 0 0 0 36.5 115 M36.5 109 V121" />
          <circle cx="45" cy="115" r="6" />
          <circle cx="61" cy="115" r="6" />
        </g>
        <rect x="45" y="23" width="13" height="18" rx="2.6" fill="url(#alelo-chip)" />
        <path d="M45 29h4.5M45 35h4.5M53.5 29h4.5M53.5 35h4.5M49.5 23v18M53.5 23v18M49.5 32h4" stroke="#9b7a45" strokeWidth="0.45" fill="none" />
        <g fill={GREEN}>
          <circle cx="20" cy="88" r="11.5" />
          <circle cx="35" cy="86.5" r="9.5" />
          <rect x="20" y="78.5" width="15" height="17" />
        </g>
        <text x="27.5" y="91.2" textAnchor="middle" fill="#fff" fontSize="10" fontWeight="700" letterSpacing="-0.3">
          alel<tspan fill={LIME}>o</tspan>
        </text>
        <text x="13" y="152" fill={GREEN} fontSize="2.6" letterSpacing="0.2">
          VÁLIDO SOMENTE NO BRASIL
        </text>
      </svg>
      <div className="absolute inset-x-[13%] top-[80%] flex flex-col leading-tight" style={{ color: GREEN }}>
        <span className="text-[4.2cqw]">de</span>
        <span className="truncate text-[5cqw] font-bold tracking-wider uppercase">{personName}</span>
        <span className="truncate text-[4.2cqw] tracking-wider uppercase">{name}</span>
      </div>
      {children}
    </div>
  );
}
