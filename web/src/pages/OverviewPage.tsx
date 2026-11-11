import { motion, type Variants } from "motion/react";
import { ArrowDownRight, ArrowUpRight, Building2, CalendarClock, ChevronRight, CircleAlert, AlertTriangle, CreditCard, HandCoins, Home, PiggyBank, TrendingUp } from "lucide-react";
import { useState, type ReactNode } from "react";
import { Link } from "react-router";
import { Card, CardDescription, CardTitle } from "@/components/ui/card";
import { Money } from "@/components/ui/money";
import { MoneyFlow } from "@/components/ui/money-flow";
import { PaidCircle } from "@/components/ui/paid-circle";
import { Segmented } from "@/components/ui/segmented";
import { useAgenda, type AgendaItem } from "@/features/agenda/api";
import { AgendaPayDialog } from "@/features/bills/dialogs";
import { useCards, useInvoice } from "@/features/cards/api";
import { CardVisual } from "@/features/cards/CardVisual";
import { useFinancings } from "@/features/financings/api";
import { ForecastBar, percent } from "@/features/overview/charts";
import { EvolutionCard } from "@/features/overview/EvolutionCard";
import { useBudgets, useOverview, type Overview } from "@/features/overview/api";
import { useReceivables } from "@/features/receivables/api";
import { useSavingsMonth } from "@/features/savings/api";
import { formatDayMonth, monthName, monthParam } from "@/lib/format";
import { haptic } from "@/lib/haptics";
import { spring } from "@/lib/motion";
import { useMonth } from "@/lib/month";
import { cn, prefersReducedMotion } from "@/lib/utils";

/** First wave: card shells come in one after the other, from slightly smaller, transparent and blurred (section 8.6). */
const shells: Variants = {
  hidden: { opacity: 0, scale: 0.97, filter: "blur(6px)" },
  show: { opacity: 1, scale: 1, filter: "blur(0px)", transition: { duration: 0.4, ease: "easeOut" } },
};

/** Same entrance without movement or blur, for reduced motion. */
const shellsReduced: Variants = { hidden: { opacity: 0 }, show: { opacity: 1, transition: { duration: 0.2 } } };

/** Overview: the dashboard cards of section 5.3 for the selected month. */
export function OverviewPage() {
  const { year, month } = useMonth();
  const param = monthParam(year, month);
  const overview = useOverview(param);
  const ov = overview.data;
  const reduced = prefersReducedMotion();

  return (
    <motion.div
      className="grid gap-4 md:grid-cols-2 xl:grid-cols-3"
      initial="hidden"
      animate="show"
      variants={{ show: { transition: { staggerChildren: 0.07 } } }}
    >
      <Shell variants={reduced ? shellsReduced : shells} className="md:col-span-2">
        {ov ? <ForecastCard ov={ov} /> : <Loading />}
      </Shell>
      <Shell variants={reduced ? shellsReduced : shells} className="md:row-span-2">
        <BillsCard month={param} />
      </Shell>
      <Shell variants={reduced ? shellsReduced : shells}>
        <CardsCard month={param} />
      </Shell>
      <Shell variants={reduced ? shellsReduced : shells}>{ov ? <HousingCard ov={ov} /> : <Loading />}</Shell>
      <Shell variants={reduced ? shellsReduced : shells}>{ov ? <ReceivablesCard ov={ov} /> : <Loading />}</Shell>
      <Shell variants={reduced ? shellsReduced : shells}>
        <SavingsCard month={param} />
      </Shell>
      <Shell variants={reduced ? shellsReduced : shells}>
        <FinancingsCard />
      </Shell>
      <Shell variants={reduced ? shellsReduced : shells} className="md:col-span-2">
        <EvolutionCard year={year} month={month} />
      </Shell>
      <Shell variants={reduced ? shellsReduced : shells}>
        <BudgetCard month={param} />
      </Shell>
    </motion.div>
  );
}

/** Card shell of the first wave; the content fades in as the second wave. */
function Shell({ children, className, variants }: { children: ReactNode; className?: string; variants: Variants }) {
  return (
    <motion.div variants={variants} className={className}>
      <Card className="flex h-full flex-col gap-4">
        <motion.div className="flex h-full flex-col gap-4" initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ delay: 0.45, duration: 0.3 }}>
          {children}
        </motion.div>
      </Card>
    </motion.div>
  );
}

/** Placeholder while the indicators load. */
function Loading() {
  return <p className="text-sm text-muted-foreground">Carregando...</p>;
}

/** Card header with title, subtitle, an icon and an optional link to the full screen. */
function Header({ title, subtitle, icon, to }: { title: string; subtitle?: ReactNode; icon: ReactNode; to?: string }) {
  return (
    <div className="flex items-start justify-between gap-3">
      <div className="min-w-0">
        <div className="flex items-center gap-1">
          <CardTitle>{title}</CardTitle>
          {to && (
            <Link to={to} className="text-muted-foreground hover:text-foreground" aria-label={`Abrir ${title}`}>
              <ChevronRight className="size-4" />
            </Link>
          )}
        </div>
        {subtitle && <CardDescription>{subtitle}</CardDescription>}
      </div>
      <span className="grid size-11 shrink-0 place-items-center rounded-2xl bg-primary/10 text-primary [&_svg]:size-5">{icon}</span>
    </div>
  );
}

/** What is left of the income after every expected way out, with the stacked bar of groups (RF-58). */
function ForecastCard({ ov }: { ov: Overview }) {
  const f = ov.forecast;
  const positive = f.balanceCents >= 0;
  return (
    <>
      <Header title="Saldo previsto" subtitle="Valor estimado depois de pagar todas as contas do mês." icon={<TrendingUp />} />
      <div className="flex flex-wrap items-end gap-x-4 gap-y-1">
        <MoneyFlow cents={f.balanceCents} className={cn("text-4xl font-semibold tracking-tight md:text-5xl", !positive && "text-destructive")} />
        <span className={cn("flex items-center gap-1 text-sm font-medium", positive ? "text-success" : "text-destructive")}>
          {positive ? <ArrowUpRight className="size-4" /> : <ArrowDownRight className="size-4" />}
          {percent(f.balanceBp)} da renda livre
        </span>
      </div>
      <p className="text-xs text-muted-foreground">
        Renda do mês <Money cents={f.incomeCents} className="font-medium text-foreground" /> · recebida <Money cents={f.receivedCents} /> · a receber <Money cents={f.expectedCents} />
      </p>
      <ForecastBar groups={f.groups} balanceCents={f.balanceCents} balanceBp={f.balanceBp} incomeCents={f.incomeCents} />
    </>
  );
}

/** Bills of the month with the paid count, due day chips and a quick pay (section 5.3). */
function BillsCard({ month }: { month: string }) {
  const agenda = useAgenda(month);
  const [paying, setPaying] = useState<AgendaItem | null>(null);
  const a = agenda.data;
  const pending = (a?.items ?? []).filter((it) => !it.paid).slice(0, 6);
  return (
    <>
      <Header title="Contas a pagar" subtitle={a ? `${a.paidCount} de ${a.count} pagas` : undefined} icon={<CalendarClock />} to="/bills" />
      <MoneyFlow cents={a?.totalCents ?? 0} className="text-3xl font-semibold tracking-tight" />
      {a && a.days.length > 0 && (
        <div className="flex gap-1.5 overflow-x-auto pb-1 [scrollbar-width:none]">
          {a.days.map((d) => (
            <span key={d.day} className={cn("flex min-w-11 flex-col items-center rounded-xl border px-2 py-1.5 text-sm font-semibold tabular-nums", d.paidCount === d.count && "text-muted-foreground")}>
              {String(d.day).padStart(2, "0")}
              <span className="flex gap-0.5">
                {Array.from({ length: Math.min(d.count, 4) }, (_, i) => (
                  <span key={i} className={cn("size-1 rounded-full", i < d.paidCount ? "bg-success" : "bg-muted-foreground/50")} />
                ))}
              </span>
            </span>
          ))}
        </div>
      )}
      <ul className="flex flex-col">
        {pending.map((it) => (
          <li key={`${it.kind}-${it.refId}`} className="flex items-center gap-3 border-b py-2 last:border-b-0">
            <PaidCircle
              paid={false}
              label={`Pagar ${it.name}`}
              onClick={() => {
                haptic();
                setPaying(it);
              }}
            />
            <span className="min-w-0 flex-1">
              <span className="block truncate text-sm font-medium">{it.name}</span>
              <span className="block text-xs text-muted-foreground">vence {formatDayMonth(it.dueOn)}</span>
            </span>
            <Money cents={it.amountCents} className="text-sm font-medium" />
          </li>
        ))}
      </ul>
      {a && pending.length === 0 && <p className="text-sm text-muted-foreground">{a.count ? "Tudo pago neste mês." : "Nada vence neste mês."}</p>}
      <AgendaPayDialog item={paying} month={month} onClose={() => setPaying(null)} />
    </>
  );
}

/** The main card with the invoice of the month, the couple part and the free limit (section 5.3). */
function CardsCard({ month }: { month: string }) {
  const cards = useCards();
  const main = (cards.data ?? []).find((c) => c.active);
  const invoice = useInvoice(main?.id, month);
  const [y, m] = month.split("-").map(Number);
  if (!main) {
    return (
      <>
        <Header title="Cartões" icon={<CreditCard />} to="/cards" />
        <p className="text-sm text-muted-foreground">Cadastre um cartão para ver a fatura aqui.</p>
      </>
    );
  }
  const inv = invoice.data;
  return (
    <>
      <Header title="Cartões" subtitle={`Fatura de ${monthName(y!, m!).toLowerCase()}`} icon={<CreditCard />} to="/cards" />
      <div className="flex items-end justify-between gap-3">
        <MoneyFlow cents={inv?.totalCents ?? 0} className="text-3xl font-semibold tracking-tight" />
        {inv && inv.thirdPartyCents > 0 && (
          <span className="text-right text-xs text-muted-foreground">
            parte do casal
            <Money cents={inv.netCents} className="block text-sm font-medium text-foreground" />
          </span>
        )}
      </div>
      <CardVisual
        name={main.name}
        personName={main.personName}
        lastDigits={main.lastDigits}
        color={main.color}
        finish={main.finish}
        dueLabel={inv ? formatDayMonth(inv.dueOn) : undefined}
        limitFreeCents={main.limitFreeCents}
        status={inv?.status}
      />
    </>
  );
}

/** Share of the income that goes to the house, with the reference ruler (section 5.3). */
function HousingCard({ ov }: { ov: Overview }) {
  const [scope, setScope] = useState<"person" | "couple">("couple");
  const h = ov.housing;
  const bp = scope === "couple" ? h.coupleBp : h.personBp;
  // The ruler shows twice the reference, so the mark sits in the middle.
  const scale = h.referenceBp * 2;
  const within = bp <= h.referenceBp;
  return (
    <>
      <Header title="Moradia" subtitle="Quanto da renda vai para a casa." icon={<Home />} />
      <div className="flex items-end gap-3">
        <MoneyFlow cents={h.housingCents} className="text-3xl font-semibold tracking-tight" />
      </div>
      <p className={cn("flex items-center gap-1.5 text-sm font-medium", within ? "text-success" : "text-warning")}>
        {within ? <Building2 className="size-4" /> : <AlertTriangle className="size-4" />}
        {percent(bp)} {scope === "couple" ? "da renda do casal" : "do seu salário"} · referência até {percent(h.referenceBp)}
      </p>
      <Segmented<"person" | "couple">
        ariaLabel="Base da moradia"
        value={scope}
        options={[
          { value: "person", label: "Só meu salário" },
          { value: "couple", label: "Salário do casal" },
        ]}
        onChange={setScope}
        className="w-full"
      />
      <div className="relative pt-5">
        <span className="absolute top-0 -translate-x-1/2 text-[0.65rem] text-muted-foreground" style={{ left: "50%" }}>
          {percent(h.referenceBp)}
        </span>
        <div className="relative h-3 overflow-hidden rounded-full bg-muted" role="meter" aria-label="Moradia sobre a renda" aria-valuemin={0} aria-valuemax={scale} aria-valuenow={bp}>
          <motion.div
            className={cn("h-full rounded-full", within ? "bg-gradient-to-r from-success-strong to-success-soft" : "bg-warning")}
            initial={{ width: 0 }}
            animate={{ width: `${Math.min(100, (bp / scale) * 100)}%` }}
            transition={spring}
          />
        </div>
        <span className="absolute -bottom-1 h-4 w-0.5 -translate-x-1/2 rounded bg-foreground" style={{ left: "50%" }} aria-hidden />
      </div>
    </>
  );
}

/** What other people owe, with the people who owe the most (section 5.3). */
function ReceivablesCard({ ov }: { ov: Overview }) {
  const people = useReceivables();
  const owing = (people.data ?? []).filter((p) => p.pendingCents > 0).slice(0, 3);
  return (
    <>
      <Header title="Valores a receber" icon={<HandCoins />} to="/receivables" />
      <MoneyFlow cents={ov.closing.receivablePendingCents} className="text-3xl font-semibold tracking-tight" />
      <p className="-mt-3 text-xs text-muted-foreground">
        recebido neste mês <Money cents={ov.closing.receivedCents} className="font-medium text-success" />
      </p>
      <ul className="flex flex-col">
        {owing.map((p) => (
          <li key={p.partyId} className="flex items-center justify-between gap-3 border-b py-2 text-sm last:border-b-0">
            <span className="truncate">{p.name}</span>
            <Money cents={p.pendingCents} className="font-medium" />
          </li>
        ))}
      </ul>
      {people.data && owing.length === 0 && <p className="text-sm text-muted-foreground">Ninguém deve nada agora.</p>}
    </>
  );
}

/** Progress of each savings box and the deposits waiting for confirmation (section 5.3). */
function SavingsCard({ month }: { month: string }) {
  const view = useSavingsMonth(month);
  const boxes = view.data?.boxes ?? [];
  const pending = boxes.filter((b) => b.suggestedCents > 0).length;
  return (
    <>
      <Header title="Caixinhas" subtitle={pending ? `${pending} ${pending === 1 ? "aporte aguardando" : "aportes aguardando"} confirmação` : "Aportes do mês em dia"} icon={<PiggyBank />} to="/savings" />
      <ul className="flex flex-col gap-3">
        {boxes.slice(0, 4).map((b) => (
          <li key={b.id} className="flex flex-col gap-1">
            <span className="flex items-center justify-between gap-2 text-sm">
              <span className="truncate font-medium">{b.name}</span>
              <span className="text-xs text-muted-foreground">
                <Money cents={b.balanceCents} /> · {percent(b.progressBp)}
              </span>
            </span>
            <div className="h-2 overflow-hidden rounded-full bg-muted">
              <motion.div className="h-full rounded-full bg-gradient-to-r from-success-strong to-success-soft" initial={{ width: 0 }} animate={{ width: `${b.progressBp / 100}%` }} transition={spring} />
            </div>
          </li>
        ))}
      </ul>
      {view.data && boxes.length === 0 && <p className="text-sm text-muted-foreground">Nenhuma caixinha criada.</p>}
    </>
  );
}

/** Remaining installments and balance of each financing (section 5.3). */
function FinancingsCard() {
  const fins = useFinancings();
  const list = (fins.data ?? []).filter((f) => !f.done);
  return (
    <>
      <Header title="Financiamentos" icon={<Building2 />} to="/bills" />
      <ul className="flex flex-col">
        {list.map((f) => (
          <li key={f.id} className="flex items-center justify-between gap-3 border-b py-2 last:border-b-0">
            <span className="min-w-0">
              <span className="block truncate text-sm font-medium">{f.name}</span>
              <span className="block text-xs text-muted-foreground">faltam {f.remainingInstallments} de {f.totalInstallments}</span>
            </span>
            <span className="text-right">
              <span className="block text-[0.7rem] text-muted-foreground">saldo devedor</span>
              <Money cents={f.balanceCents} className="text-sm font-semibold" />
            </span>
          </li>
        ))}
      </ul>
      {fins.data && list.length === 0 && <p className="text-sm text-muted-foreground">Nenhum financiamento em aberto.</p>}
    </>
  );
}

/** Categories close to or over the budget, with icon and label besides the color (RF-52). */
function BudgetCard({ month }: { month: string }) {
  const budgets = useBudgets(month);
  const alert = (budgets.data ?? []).filter((b) => b.level !== "ok");
  return (
    <>
      <Header title="Orçamento" subtitle={budgets.data?.length ? `${alert.length} perto ou acima do limite` : undefined} icon={<CircleAlert />} to="/settings/budgets" />
      <ul className="flex flex-col gap-3">
        {alert.map((b) => (
          <li key={b.categoryId} className="flex flex-col gap-1">
            <span className="flex items-center justify-between gap-2 text-sm">
              <span className="flex min-w-0 items-center gap-1.5 font-medium">
                {b.level === "over" ? <CircleAlert className="size-4 shrink-0 text-destructive" /> : <AlertTriangle className="size-4 shrink-0 text-warning" />}
                <span className="truncate">{b.name}</span>
              </span>
              <span className={cn("text-xs font-medium", b.level === "over" ? "text-destructive" : "text-warning")}>
                {percent(b.usedBp)} {b.level === "over" ? "estourado" : "usado"}
              </span>
            </span>
            <div className="h-2 overflow-hidden rounded-full bg-muted">
              <motion.div className={cn("h-full rounded-full", b.level === "over" ? "bg-destructive" : "bg-warning")} initial={{ width: 0 }} animate={{ width: `${Math.min(100, b.usedBp / 100)}%` }} transition={spring} />
            </div>
            <span className="text-xs text-muted-foreground">
              <Money cents={b.spentCents} /> de <Money cents={b.budgetCents} />
            </span>
          </li>
        ))}
      </ul>
      {budgets.data && budgets.data.length === 0 && (
        <p className="text-sm text-muted-foreground">
          Defina orçamentos em <Link to="/settings/budgets" className="text-primary">Ajustes</Link>.
        </p>
      )}
      {budgets.data && budgets.data.length > 0 && alert.length === 0 && <p className="text-sm text-muted-foreground">Todas as categorias dentro do orçamento.</p>}
    </>
  );
}
