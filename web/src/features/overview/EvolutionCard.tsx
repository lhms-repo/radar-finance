import { useState } from "react";
import { CardDescription, CardTitle } from "@/components/ui/card";
import { Money } from "@/components/ui/money";
import { NativeSelect } from "@/components/ui/native-select";
import { CategoryIcon } from "@/features/categories/icons";
import { useSpending } from "@/features/overview/api";
import { SpendingBars } from "@/features/overview/charts";
import { monthParam } from "@/lib/format";

/** Spending of the last six months, total or of one category (RF-59). */
export function EvolutionCard({ year, month }: { year: number; month: number }) {
  const to = monthParam(year, month);
  const fromIndex = year * 12 + month - 1 - 5;
  const from = monthParam(Math.floor(fromIndex / 12), (fromIndex % 12) + 1);
  const spending = useSpending(from, to);
  const [categoryId, setCategoryId] = useState(0);
  const months = spending.data ?? [];
  const categories = new Map<number, string>();
  for (const m of months) for (const c of m.categories) if (c.id !== 0) categories.set(c.id, c.name);
  const data = months.map((m) => ({ month: m.month, cents: categoryId ? (m.categories.find((c) => c.id === categoryId)?.cents ?? 0) : m.totalCents }));
  const current = months.find((m) => m.month === to);
  const title = categoryId ? `Gastos em ${categories.get(categoryId) ?? ""}` : "Gastos do casal";
  return (
    <>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <CardTitle>Evolução dos gastos</CardTitle>
          <CardDescription>Só a parte do casal; cotas de terceiros ficam de fora.</CardDescription>
        </div>
        <div className="w-full sm:w-52">
          <NativeSelect aria-label="Categoria do gráfico" value={categoryId} onChange={(e) => setCategoryId(Number(e.target.value))}>
            <option value={0}>Todas as categorias</option>
            {[...categories].map(([id, name]) => (
              <option key={id} value={id}>
                {name}
              </option>
            ))}
          </NativeSelect>
        </div>
      </div>
      <SpendingBars data={data} selected={to} title={title} />
      {current && !categoryId && (
        <ul className="grid gap-x-6 gap-y-1.5 sm:grid-cols-2">
          {current.categories.slice(0, 6).map((c) => (
            <li key={c.id} className="flex items-center gap-2 text-sm">
              <CategoryIcon name={c.icon} className="size-4 shrink-0 text-muted-foreground" />
              <span className="min-w-0 flex-1 truncate">{c.name}</span>
              <Money cents={c.cents} className="font-medium" />
            </li>
          ))}
        </ul>
      )}
    </>
  );
}
