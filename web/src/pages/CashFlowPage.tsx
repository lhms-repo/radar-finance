import { ArrowDownLeft, ArrowUpRight, HandCoins, Landmark, PiggyBank, Scale, Search } from "lucide-react";
import { useState } from "react";
import { Card, CardDescription, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Money } from "@/components/ui/money";
import { MoneyFlow } from "@/components/ui/money-flow";
import { NativeSelect } from "@/components/ui/native-select";
import { useAccounts, usePersons } from "@/features/accounts/api";
import { useCards } from "@/features/cards/api";
import { useCategories } from "@/features/categories/api";
import { useOverview, useSearch, type SearchFilter } from "@/features/overview/api";
import { CommitmentBars } from "@/features/overview/charts";
import { EvolutionCard } from "@/features/overview/EvolutionCard";
import { formatDayMonth, monthName, monthParam } from "@/lib/format";
import { useMonth } from "@/lib/month";
import { cn } from "@/lib/utils";

/** Cash flow: month closing, net worth, account balances, commitments, spending evolution and the entry search (RF-59 to RF-63). */
export function CashFlowPage() {
  const { year, month } = useMonth();
  const param = monthParam(year, month);
  const overview = useOverview(param);
  const accounts = useAccounts();
  const ov = overview.data;
  return (
    <div className="flex flex-col gap-4">
      <h1 className="px-1 text-lg font-semibold md:hidden">Fluxo de caixa</h1>
      {ov && (
        <div className="grid gap-4 lg:grid-cols-3">
          <Card className="flex flex-col gap-3 lg:col-span-2">
            <CardTitle>Fechamento de {monthName(year, month).toLowerCase()}</CardTitle>
            <dl className="grid grid-cols-2 gap-3 sm:grid-cols-4">
              <Stat icon={<ArrowDownLeft className="text-success" />} label="Entradas" cents={ov.closing.incomeCents} hint="renda que caiu" />
              <Stat icon={<ArrowUpRight className="text-destructive" />} label="Gastos" cents={ov.closing.spendingCents} hint="parte do casal" />
              <Stat icon={<PiggyBank className="text-primary" />} label="Guardado" cents={ov.closing.savedCents} hint="aportes menos retiradas" />
              <Stat icon={<HandCoins className="text-warning" />} label="A receber" cents={ov.closing.receivablePendingCents} hint={<>recebido <Money cents={ov.closing.receivedCents} /></>} />
            </dl>
          </Card>
          <Card className="flex flex-col gap-3">
            <div className="flex items-start justify-between gap-2">
              <div>
                <CardTitle>Patrimônio líquido</CardTitle>
                <CardDescription>Contas e caixinhas menos o que se deve.</CardDescription>
              </div>
              <Scale className="size-5 text-primary" />
            </div>
            <MoneyFlow cents={ov.netWorth.netCents} className={cn("text-3xl font-semibold tracking-tight", ov.netWorth.netCents < 0 && "text-destructive")} />
            <dl className="flex flex-col gap-1 text-sm">
              <Row label="Contas" cents={ov.netWorth.accountsCents} />
              <Row label="Caixinhas" cents={ov.netWorth.savingsCents} />
              <Row label="Financiamentos" cents={-ov.netWorth.financingCents} />
              <Row label="Faturas fechadas em aberto" cents={-ov.netWorth.cardDebtCents} />
            </dl>
          </Card>
        </div>
      )}

      <div className="grid gap-4 lg:grid-cols-3">
        <Card className="flex flex-col gap-2">
          <CardTitle>Saldos das contas</CardTitle>
          <ul className="flex flex-col">
            {(accounts.data ?? []).filter((a) => a.active).map((a) => (
              <li key={a.id} className="flex items-center gap-3 border-b py-2 last:border-b-0">
                <Landmark className="size-4 shrink-0 text-primary" />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm font-medium">{a.name}</span>
                  <span className="block text-xs text-muted-foreground">{a.personName}</span>
                </span>
                <Money cents={a.balanceCents} className={cn("text-sm font-semibold", a.balanceCents < 0 && "text-destructive")} />
              </li>
            ))}
          </ul>
        </Card>
        <Card className="flex flex-col gap-2 lg:col-span-2">
          <CardTitle>Renda já comprometida</CardTitle>
          <CardDescription>Parcelas de cartão e de financiamentos dos próximos meses sobre a renda prevista.</CardDescription>
          {ov && <CommitmentBars data={ov.commitments} />}
        </Card>
      </div>

      <Card className="flex flex-col gap-4">
        <EvolutionCard year={year} month={month} />
      </Card>

      <SearchCard year={year} month={month} />
    </div>
  );
}

/** One figure of the month closing. */
function Stat({ icon, label, cents, hint }: { icon: React.ReactNode; label: string; cents: number; hint: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-1 rounded-xl bg-muted p-3">
      <dt className="flex items-center gap-1.5 text-xs text-muted-foreground [&_svg]:size-4">
        {icon}
        {label}
      </dt>
      <dd>
        <MoneyFlow cents={cents} className="text-lg font-semibold" />
        <span className="block text-[0.7rem] text-muted-foreground">{hint}</span>
      </dd>
    </div>
  );
}

/** A label and a signed amount of the net worth. */
function Row({ label, cents }: { label: string; cents: number }) {
  return (
    <div className="flex justify-between gap-3">
      <dt className="text-muted-foreground">{label}</dt>
      <dd>
        <Money cents={cents} />
      </dd>
    </div>
  );
}

/** Search over every entry, card purchase and Alelo purchase, with filters in one row (RF-63). */
function SearchCard({ year, month }: { year: number; month: number }) {
  const first = `${monthParam(year, month)}-01`;
  const last = `${monthParam(year, month)}-${String(new Date(Date.UTC(year, month, 0)).getUTCDate()).padStart(2, "0")}`;
  const [filter, setFilter] = useState<Omit<SearchFilter, "from" | "to"> & { from?: string; to?: string }>({ categoryId: 0, accountId: 0, cardId: 0, personId: 0, text: "" });
  const f: SearchFilter = { ...filter, from: filter.from ?? first, to: filter.to ?? last };
  const result = useSearch(f);
  const categories = useCategories();
  const accounts = useAccounts();
  const cards = useCards();
  const persons = usePersons();

  /** Changes one filter. */
  const set = (patch: Partial<typeof filter>) => setFilter((x) => ({ ...x, ...patch }));

  return (
    <Card className="flex flex-col gap-3">
      <div className="flex items-center gap-2">
        <Search className="size-4 text-primary" />
        <CardTitle>Lançamentos</CardTitle>
      </div>
      <div className="grid grid-cols-2 gap-2 md:grid-cols-4 xl:grid-cols-7">
        <Input aria-label="Buscar texto" placeholder="Buscar" value={filter.text} onChange={(e) => set({ text: e.target.value })} className="col-span-2 md:col-span-4 xl:col-span-1" />
        <Input aria-label="De" type="date" value={f.from} onChange={(e) => set({ from: e.target.value || undefined })} />
        <Input aria-label="Até" type="date" value={f.to} onChange={(e) => set({ to: e.target.value || undefined })} />
        <NativeSelect aria-label="Categoria" value={filter.categoryId} onChange={(e) => set({ categoryId: Number(e.target.value) })}>
          <option value={0}>Categorias</option>
          {(categories.data ?? []).map((c) => (
            <optgroup key={c.id} label={c.name}>
              <option value={c.id}>Toda {c.name}</option>
              {(c.children ?? []).map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
            </optgroup>
          ))}
        </NativeSelect>
        <NativeSelect aria-label="Conta" value={filter.accountId} onChange={(e) => set({ accountId: Number(e.target.value) })}>
          <option value={0}>Contas</option>
          {(accounts.data ?? []).map((a) => (
            <option key={a.id} value={a.id}>
              {a.name}
            </option>
          ))}
        </NativeSelect>
        <NativeSelect aria-label="Cartão" value={filter.cardId} onChange={(e) => set({ cardId: Number(e.target.value) })}>
          <option value={0}>Cartões</option>
          {(cards.data ?? []).map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
            </option>
          ))}
        </NativeSelect>
        <NativeSelect aria-label="Pessoa" value={filter.personId} onChange={(e) => set({ personId: Number(e.target.value) })}>
          <option value={0}>Pessoas</option>
          {(persons.data ?? []).map((p) => (
            <option key={p.id} value={p.id}>
              {p.name}
            </option>
          ))}
        </NativeSelect>
      </div>
      <ul className="flex flex-col">
        {(result.data ?? []).map((l) => (
          <li key={`${l.source}-${l.id}`} className="flex items-center gap-3 border-b py-2 last:border-b-0">
            <span className="w-10 shrink-0 text-xs text-muted-foreground">{formatDayMonth(l.date)}</span>
            <span className="min-w-0 flex-1">
              <span className="block truncate text-sm font-medium">{l.description || l.categoryName || "Lançamento"}</span>
              <span className="block truncate text-xs text-muted-foreground">
                {[l.categoryName, l.cardName || l.accountName, l.personName].filter(Boolean).join(" · ")}
              </span>
            </span>
            <Money cents={l.amountCents} signed className={cn("text-sm font-medium", l.amountCents > 0 && "text-success")} />
          </li>
        ))}
      </ul>
      {result.data?.length === 0 && <p className="py-2 text-center text-sm text-muted-foreground">Nada encontrado com esses filtros.</p>}
      {result.data?.length === 500 && <p className="text-center text-xs text-muted-foreground">Mostrando os 500 mais recentes. Use os filtros para afinar.</p>}
    </Card>
  );
}
