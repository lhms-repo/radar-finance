import { cn } from "@/lib/utils";

/** Radar logo: a radar sweep over a blue tile. */
export function Logo({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 512 512" className={cn("rounded-xl", className)} aria-hidden="true">
      <defs>
        <linearGradient id="radar-logo-bg" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#3b82f6" />
          <stop offset="1" stopColor="#1d4ed8" />
        </linearGradient>
        <linearGradient id="radar-logo-sweep" x1="0" y1="1" x2="1" y2="0">
          <stop offset="0" stopColor="#4ade80" stopOpacity="0" />
          <stop offset="1" stopColor="#4ade80" stopOpacity="0.9" />
        </linearGradient>
      </defs>
      <rect width="512" height="512" fill="url(#radar-logo-bg)" />
      <g fill="none" stroke="#fff" strokeLinecap="round">
        <circle cx="256" cy="256" r="150" strokeWidth="26" strokeOpacity="0.35" />
        <circle cx="256" cy="256" r="92" strokeWidth="26" strokeOpacity="0.6" />
      </g>
      <path d="M256 256 L256 106 A150 150 0 0 1 386 181 Z" fill="url(#radar-logo-sweep)" />
      <line x1="256" y1="256" x2="386" y2="181" stroke="#fff" strokeWidth="26" strokeLinecap="round" />
      <circle cx="256" cy="256" r="30" fill="#fff" />
      <circle cx="330" cy="150" r="22" fill="#4ade80" />
    </svg>
  );
}
