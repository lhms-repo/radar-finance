import { motion } from "motion/react";
import { AlertTriangle, Clock, Sparkles, Store, Trophy } from "lucide-react";
import { Card, CardDescription, CardTitle } from "@/components/ui/card";
import { Money } from "@/components/ui/money";
import { MoneyFlow } from "@/components/ui/money-flow";
import { useListComparison } from "@/features/market/api";
import { spring } from "@/lib/motion";
import { cn } from "@/lib/utils";

/** Where the list is cheaper: ranking of the markets and the best split, as the backend computed (RF-72 to RF-75). */
export function ListComparison({ listId }: { listId: number }) {
  const cmp = useListComparison(listId);
  const c = cmp.data;
  if (!c || (c.ranking.length === 0 && c.uncovered.length === 0)) return null;
  const split = c.split;
  return (
    <div className="grid gap-4 md:grid-cols-2">
      {split && (
        <Card className="flex flex-col gap-3" aria-label="Melhor divisão">
          <div className="flex items-start justify-between gap-2">
            <div>
              <CardTitle className="flex items-center gap-1.5">
                <Sparkles className="size-4 text-primary" />
                Melhor divisão
              </CardTitle>
              <CardDescription>
                {split.markets.length === 1 ? `Tudo no ${split.markets[0]!.name}` : `Em ${split.markets.length} mercados, até ${c.settings.maxMarkets} permitidos`}
              </CardDescription>
            </div>
            <MoneyFlow cents={split.totalCents} className="text-xl font-semibold" />
          </div>
          {split.markets.length > 1 && split.savingsCents !== undefined && (
            <p className="rounded-xl bg-success/10 px-3 py-2 text-sm text-success">
              Economia de <Money cents={split.savingsCents} /> sobre o melhor mercado único
            </p>
          )}
          {split.markets.length > 1 && split.savingsCents === undefined && (
            <p className="text-xs text-muted-foreground">Nenhum mercado tem sozinho todos os itens com preço.</p>
          )}
          <ul className="flex flex-col gap-3">
            {split.markets.map((m) => (
              <li key={m.marketId} className="rounded-xl border p-3">
                <div className="flex items-center justify-between gap-2 text-sm font-medium">
                  <span className="flex min-w-0 items-center gap-1.5">
                    <Store className="size-4 shrink-0 text-primary" />
                    <span className="truncate">{m.name}</span>
                  </span>
                  <Money cents={m.totalCents} />
                </div>
                <ul className="mt-1.5 flex flex-col gap-0.5">
                  {m.items.map((it) => (
                    <li key={it.itemId} className="flex justify-between gap-2 text-xs text-muted-foreground">
                      <span className="truncate">{it.label}</span>
                      <Money cents={it.costCents} />
                    </li>
                  ))}
                </ul>
              </li>
            ))}
          </ul>
          {c.uncovered.length > 0 && (
            <p className="flex items-start gap-1.5 text-xs text-warning">
              <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
              Fora da conta, sem preço em nenhum mercado: {c.uncovered.join(", ")}.
            </p>
          )}
        </Card>
      )}
      <Card className="flex flex-col gap-2" aria-label="Ranking dos mercados">
        <CardTitle className="flex items-center gap-1.5">
          <Trophy className="size-4 text-primary" />
          Ranking dos mercados
        </CardTitle>
        <ol className="flex flex-col">
          {c.ranking.map((r, i) => (
            <motion.li key={r.marketId} layout transition={spring} className="flex items-start gap-3 border-b py-2.5 last:border-b-0">
              <span
                className={cn(
                  "grid size-7 shrink-0 place-items-center rounded-full text-xs font-semibold",
                  i === 0 && r.missingCount === 0 ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground",
                )}
              >
                {i + 1}
              </span>
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm font-medium">
                  {r.name}
                  {r.neighborhood ? <span className="font-normal text-muted-foreground">, {r.neighborhood}</span> : null}
                </span>
                {r.missingCount > 0 && (
                  <span className="flex items-start gap-1 text-xs text-warning" title={r.missing.join(", ")}>
                    <AlertTriangle className="mt-0.5 size-3 shrink-0" />
                    {r.missingCount === 1 ? "Falta 1 item" : `Faltam ${r.missingCount} itens`}: {r.missing.join(", ")}
                  </span>
                )}
                {r.staleCount > 0 && (
                  <span className="flex items-center gap-1 text-xs text-muted-foreground">
                    <Clock className="size-3 shrink-0" />
                    {r.staleCount === 1 ? "1 preço" : `${r.staleCount} preços`} com mais de {c.settings.staleDays} dias
                  </span>
                )}
              </span>
              <Money cents={r.totalCents} className="shrink-0 text-sm font-medium" />
            </motion.li>
          ))}
        </ol>
        {c.ranking.length === 0 && <p className="py-4 text-center text-sm text-muted-foreground">Nenhum mercado tem preço dos itens da lista.</p>}
      </Card>
    </div>
  );
}
