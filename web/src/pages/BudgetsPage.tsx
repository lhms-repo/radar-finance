import { ChevronLeft, Check } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import { Button } from "@/components/ui/button";
import { Card, CardDescription } from "@/components/ui/card";
import { Money } from "@/components/ui/money";
import { MoneyInput } from "@/components/ui/money-input";
import { useCategories, type Category } from "@/features/categories/api";
import { CategoryIcon } from "@/features/categories/icons";
import { useBudgets, useSetBudget, type BudgetStatus } from "@/features/overview/api";
import { percent } from "@/features/overview/charts";
import { monthName, monthParam } from "@/lib/format";
import { useMonth } from "@/lib/month";
import { cn } from "@/lib/utils";

/** Monthly budget of each top level category, with the spending of the selected month (RF-51, RF-52). */
export function BudgetsPage() {
  const { year, month } = useMonth();
  const budgets = useBudgets(monthParam(year, month));
  const categories = useCategories();
  const byCategory = new Map((budgets.data ?? []).map((b) => [b.categoryId, b]));
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-0.5">
        <Link to="/settings" className="flex items-center gap-0.5 text-sm text-primary">
          <ChevronLeft className="size-4" />
          Ajustes
        </Link>
        <h1 className="px-1 text-lg font-semibold">Orçamentos</h1>
      </div>
      <CardDescription className="px-1">
        Valor mensal por categoria principal. O Radar avisa quando o gasto chega a 80% e a 100%. Gasto de {monthName(year, month).toLowerCase()} ao lado.
      </CardDescription>
      <Card className="p-2">
        <ul className="flex flex-col">
          {(categories.data ?? []).map((c) => (
            <BudgetRow key={c.id} category={c} status={byCategory.get(c.id)} />
          ))}
        </ul>
      </Card>
    </div>
  );
}

/** One category: current budget status and the field to change it; zero removes the budget. */
function BudgetRow({ category, status }: { category: Category; status?: BudgetStatus }) {
  const save = useSetBudget();
  const [draft, setDraft] = useState<number | null>(null);
  const value = draft ?? status?.budgetCents ?? 0;
  const changed = draft !== null && draft !== (status?.budgetCents ?? 0);
  return (
    <li className="flex flex-wrap items-center gap-3 border-b px-2 py-3 last:border-b-0">
      <span className="grid size-10 shrink-0 place-items-center rounded-xl bg-primary/10">
        <CategoryIcon name={category.icon} className="size-5 text-primary" />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-medium">{category.name}</span>
        {status ? (
          <span className={cn("block text-xs", status.level === "over" ? "text-destructive" : status.level === "warning" ? "text-warning" : "text-muted-foreground")}>
            <Money cents={status.spentCents} /> · {percent(status.usedBp)}
            {status.level === "over" ? " · estourado" : status.level === "warning" ? " · perto do limite" : ""}
          </span>
        ) : (
          <span className="block text-xs text-muted-foreground">Sem orçamento</span>
        )}
      </span>
      <div className="flex w-full items-center gap-2 sm:w-auto">
        <div className="flex-1 sm:w-44">
          <MoneyInput ariaLabel={`Orçamento de ${category.name}`} value={value} onChange={setDraft} />
        </div>
        <Button
          size="icon"
          aria-label={`Salvar orçamento de ${category.name}`}
          disabled={!changed || save.isPending}
          onClick={() => save.mutate({ categoryId: category.id, amountCents: value }, { onSuccess: () => setDraft(null) })}
        >
          <Check />
        </Button>
      </div>
      {save.error && <p className="w-full text-sm text-destructive">{save.error.message}</p>}
    </li>
  );
}
