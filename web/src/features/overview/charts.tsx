import { motion } from "motion/react";
import { Bar, BarChart, CartesianGrid, Cell, ResponsiveContainer, Tooltip, XAxis, YAxis, type TooltipContentProps } from "recharts";
import type { NameType, ValueType } from "recharts/types/component/DefaultTooltipContent";
import { Money } from "@/components/ui/money";
import type { ForecastGroup } from "@/features/overview/api";
import { formatMoney, monthName } from "@/lib/format";
import { spring, usePrefersReducedMotion } from "@/lib/motion";
import { cn } from "@/lib/utils";

/** Labels and series slots of the forecast groups, in the order of the bar (section 5.3). */
export const GROUPS: Record<ForecastGroup["key"] | "balance", { label: string; color: string }> = {
  housing: { label: "Moradia", color: "var(--series-1)" },
  invoices: { label: "Faturas", color: "var(--series-2)" },
  bills: { label: "Contas e financiamentos", color: "var(--series-3)" },
  day_to_day: { label: "Dia a dia", color: "var(--series-4)" },
  savings: { label: "Caixinhas", color: "var(--series-5)" },
  balance: { label: "Sobra", color: "var(--series-6)" },
};

/** Formats basis points as a whole percentage with comma decimals, like 30,5%. */
export function percent(bp: number) {
  return `${(bp / 100).toLocaleString("pt-BR", { maximumFractionDigits: 1 })}%`;
}

/** Stacked bar from the income to what is left, with a 2px gap between groups and every value labeled (RF-58). */
export function ForecastBar({ groups, balanceCents, balanceBp, incomeCents }: { groups: ForecastGroup[]; balanceCents: number; balanceBp: number; incomeCents: number }) {
  const parts = [
    ...groups.map((g) => ({ key: g.key, cents: g.cents, bp: g.shareBp })),
    { key: "balance" as const, cents: Math.max(0, balanceCents), bp: Math.max(0, balanceBp) },
  ].filter((p) => p.cents > 0);
  // The bar spans the income, or everything that goes out when it is larger.
  const span = Math.max(incomeCents, parts.reduce((s, p) => s + p.cents, 0), 1);
  return (
    <div className="flex flex-col gap-3">
      <div className="flex h-3 w-full gap-0.5 overflow-hidden rounded-full bg-muted" role="img" aria-label="Divisão da renda do mês">
        {parts.map((p, i) => (
          <motion.span
            key={p.key}
            className={cn("h-full", i === 0 && "rounded-l-full", i === parts.length - 1 && "rounded-r-full")}
            style={{ background: GROUPS[p.key].color }}
            initial={{ width: 0 }}
            animate={{ width: `${(p.cents / span) * 100}%` }}
            transition={{ ...spring, delay: 0.05 * i }}
          />
        ))}
      </div>
      <ul className="grid grid-cols-2 gap-x-4 gap-y-2 sm:grid-cols-3">
        {[...groups.map((g) => ({ key: g.key, cents: g.cents, bp: g.shareBp })), { key: "balance" as const, cents: balanceCents, bp: balanceBp }].map((p) => (
          <li key={p.key} className="flex flex-col">
            <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <span className="size-2 shrink-0 rounded-full" style={{ background: GROUPS[p.key].color }} aria-hidden />
              {GROUPS[p.key].label}
            </span>
            <Money
              cents={p.key === "balance" ? p.cents : -p.cents}
              className={cn("text-sm font-medium", p.key === "balance" && (p.cents >= 0 ? "text-success" : "text-destructive"))}
            />
            <span className="text-xs text-muted-foreground">{percent(p.bp)} da renda</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

/** Short month label for chart axes, like nov 26. */
export function shortMonth(m: string) {
  const [y, mm] = m.split("-").map(Number);
  return `${monthName(y!, mm!).slice(0, 3).toLowerCase()} ${String(y).slice(2)}`;
}

/** Compact reais for an axis, like R$ 1,2 mil. */
function axisMoney(cents: number) {
  const reais = cents / 100;
  if (Math.abs(reais) >= 1000) return `R$ ${(reais / 1000).toLocaleString("pt-BR", { maximumFractionDigits: 1 })} mil`;
  return `R$ ${reais.toLocaleString("pt-BR", { maximumFractionDigits: 0 })}`;
}

/** Tooltip of a month bar: month name and value. */
function MonthTooltip({ active, payload }: TooltipContentProps<ValueType, NameType>) {
  if (!active || !payload?.length) return null;
  const row = payload[0]!.payload as { month: string; cents: number; label?: string };
  return (
    <div className="rounded-xl border bg-card px-3 py-2 text-xs shadow-lg">
      <p className="text-muted-foreground">{shortMonth(row.month)}</p>
      <p className="font-semibold">{row.label ?? formatMoney(row.cents)}</p>
    </div>
  );
}

/** Spending of each month as single series bars; the selected month is highlighted (RF-59). */
export function SpendingBars({ data, selected, title }: { data: { month: string; cents: number }[]; selected: string; title: string }) {
  // Recharts animates from JavaScript, so the reduced motion setting is passed by hand.
  const reduced = usePrefersReducedMotion();
  return (
    <figure className="flex flex-col gap-2">
      <div className="h-44 w-full" aria-hidden>
        <ResponsiveContainer width="100%" height="100%">
          <BarChart data={data} margin={{ top: 8, right: 4, bottom: 0, left: 4 }} barCategoryGap="28%">
            <CartesianGrid vertical={false} stroke="var(--border)" />
            <XAxis dataKey="month" tickFormatter={shortMonth} tickLine={false} axisLine={false} tick={{ fill: "var(--muted-foreground)", fontSize: 11 }} />
            <YAxis tickFormatter={axisMoney} tickLine={false} axisLine={false} width={64} tick={{ fill: "var(--muted-foreground)", fontSize: 11 }} />
            <Tooltip content={MonthTooltip} cursor={{ fill: "var(--muted)", radius: 6 }} isAnimationActive={!reduced} />
            <Bar dataKey="cents" radius={[4, 4, 0, 0]} maxBarSize={28} isAnimationActive={!reduced}>
              {data.map((d) => (
                <Cell key={d.month} fill="var(--series-1)" fillOpacity={d.month === selected ? 1 : 0.45} />
              ))}
            </Bar>
          </BarChart>
        </ResponsiveContainer>
      </div>
      <table className="sr-only">
        <caption>{title}</caption>
        <tbody>
          {data.map((d) => (
            <tr key={d.month}>
              <th scope="row">{shortMonth(d.month)}</th>
              <td>{formatMoney(d.cents)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </figure>
  );
}

/** Share of the expected income already owed in each coming month, as bars labeled with the percentage (RF-60). */
export function CommitmentBars({ data }: { data: { month: string; bp: number; totalCents: number }[] }) {
  const rows = data.map((d) => ({ month: d.month, cents: d.bp, label: `${percent(d.bp)} da renda, ${formatMoney(d.totalCents)}` }));
  const reduced = usePrefersReducedMotion();
  return (
    <figure className="flex flex-col gap-2">
      <div className="h-40 w-full" aria-hidden>
        <ResponsiveContainer width="100%" height="100%">
          <BarChart data={rows} margin={{ top: 18, right: 4, bottom: 0, left: 4 }} barCategoryGap="28%">
            <CartesianGrid vertical={false} stroke="var(--border)" />
            <XAxis dataKey="month" tickFormatter={shortMonth} tickLine={false} axisLine={false} tick={{ fill: "var(--muted-foreground)", fontSize: 11 }} />
            <YAxis tickFormatter={(bp: number) => percent(bp)} tickLine={false} axisLine={false} width={44} tick={{ fill: "var(--muted-foreground)", fontSize: 11 }} />
            <Tooltip content={MonthTooltip} cursor={{ fill: "var(--muted)", radius: 6 }} isAnimationActive={!reduced} />
            <Bar dataKey="cents" fill="var(--series-1)" radius={[4, 4, 0, 0]} maxBarSize={28} isAnimationActive={!reduced} />
          </BarChart>
        </ResponsiveContainer>
      </div>
      <table className="sr-only">
        <caption>Comprometimento da renda nos próximos meses</caption>
        <tbody>
          {rows.map((r) => (
            <tr key={r.month}>
              <th scope="row">{shortMonth(r.month)}</th>
              <td>{r.label}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </figure>
  );
}
