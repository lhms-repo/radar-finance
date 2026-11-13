import { AnimatePresence, motion } from "motion/react";
import { toast } from "sonner";
import { DropdownMenu } from "radix-ui";
import { CalendarClock, CreditCard, MoreHorizontal, Pencil, Plus, Receipt, Scale } from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardDescription, CardTitle } from "@/components/ui/card";
import { Money } from "@/components/ui/money";
import { MoneyFlow } from "@/components/ui/money-flow";
import { PaidCircle } from "@/components/ui/paid-circle";
import { useAgenda, useUndoAgendaPayment, type AgendaItem } from "@/features/agenda/api";
import { useBills, type Bill } from "@/features/bills/api";
import { ActualDialog, AgendaPayDialog, BillFormDialog, CorrectionDialog, FinancingFormDialog } from "@/features/bills/dialogs";
import { CategoryIcon } from "@/features/categories/icons";
import { useFinancings, type Financing } from "@/features/financings/api";
import { formatDayMonth, monthName, monthParam } from "@/lib/format";
import { haptic } from "@/lib/haptics";
import { fade, spring } from "@/lib/motion";
import { useMonth } from "@/lib/month";
import { cn } from "@/lib/utils";

type DialogState =
  | { kind: "pay"; item: AgendaItem }
  | { kind: "actual"; item: AgendaItem }
  | { kind: "bill"; bill?: Bill }
  | { kind: "financing"; financing?: Financing }
  | { kind: "correct"; financing: Financing };

/** Fixed bills and financings: what is due in the month, the financings and the registered bills (RF-35 to RF-42). */
export function BillsPage() {
  const { year, month } = useMonth();
  const param = monthParam(year, month);
  const agenda = useAgenda(param);
  const [day, setDay] = useState<number | null>(null);
  const [dialog, setDialog] = useState<DialogState | null>(null);
  const undo = useUndoAgendaPayment();
  const a = agenda.data;
  const items = (a?.items ?? []).filter((it) => day === null || Number(it.dueOn.slice(8)) === day);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between gap-3">
        <h1 className="px-1 text-lg font-semibold md:hidden">Contas</h1>
        <div className="ml-auto flex gap-2">
          <Button size="sm" variant="outline" onClick={() => setDialog({ kind: "financing" })}>
            <Plus />
            Financiamento
          </Button>
          <Button size="sm" onClick={() => setDialog({ kind: "bill" })}>
            <Plus />
            Conta fixa
          </Button>
        </div>
      </div>

      <div className="grid gap-4 lg:grid-cols-5">
        <Card className="flex flex-col gap-4 lg:col-span-3">
          <div className="flex items-start justify-between gap-3">
            <div>
              <CardTitle>
                Contas a pagar · {monthName(year, month).toLowerCase()}
              </CardTitle>
              <MoneyFlow cents={a?.totalCents ?? 0} className="text-3xl font-semibold tracking-tight" />
              <CardDescription>
                {a ? `${a.paidCount} de ${a.count} pagas` : "Carregando..."}
              </CardDescription>
            </div>
            <span className="grid size-12 shrink-0 place-items-center rounded-2xl bg-primary/10">
              <CalendarClock className="size-6 text-primary" />
            </span>
          </div>

          {a && a.days.length > 0 && (
            <div role="radiogroup" aria-label="Dias de vencimento" className="flex gap-2 overflow-x-auto pb-1 [scrollbar-width:none]">
              {a.days.map((d) => {
                const active = d.day === day;
                const allPaid = d.paidCount === d.count;
                return (
                  <button
                    key={d.day}
                    type="button"
                    role="radio"
                    aria-checked={active}
                    aria-label={`Dia ${d.day}, ${d.count} ${d.count === 1 ? "conta" : "contas"}`}
                    onClick={() => {
                      haptic();
                      setDay(active ? null : d.day);
                    }}
                    className={cn(
                      "relative flex min-w-14 flex-col items-center gap-1 rounded-xl border px-3 py-2 text-sm font-semibold transition-colors",
                      active ? "border-primary text-primary-foreground" : allPaid ? "text-muted-foreground" : "bg-card",
                    )}
                  >
                    {active && <motion.span layoutId="agenda-day" transition={spring} className="absolute inset-0 rounded-xl bg-primary" />}
                    <span className="relative tabular-nums">{String(d.day).padStart(2, "0")}</span>
                    <span className="relative flex gap-0.5">
                      {Array.from({ length: Math.min(d.count, 5) }, (_, i) => (
                        <span key={i} className={cn("size-1 rounded-full", active ? "bg-primary-foreground" : i < d.paidCount ? "bg-success" : "bg-muted-foreground/50")} />
                      ))}
                    </span>
                  </button>
                );
              })}
            </div>
          )}

          <ul className="flex flex-col">
            <AnimatePresence initial={false}>
              {items.map((it) => (
                <motion.li
                  key={`${it.kind}-${it.refId}`}
                  layout
                  initial={{ opacity: 0, y: -6 }}
                  animate={{ opacity: it.paid ? 0.6 : 1, y: 0 }}
                  exit={{ opacity: 0 }}
                  transition={{ layout: spring, opacity: fade }}
                  className="flex items-center gap-3 border-b py-2.5 last:border-b-0"
                >
                  <PaidCircle
                    paid={it.paid}
                    label={it.paid ? `Desfazer pagamento de ${it.name}` : `Pagar ${it.name}`}
                    onClick={() => {
                      haptic();
                      // A group reads its part of an invoice; only the owner pays it, in her personal space (RF-83).
                      if (it.kind === "invoice" && it.payable === false) toast.info("A fatura é paga pela dona do cartão, no espaço Pessoal dela.");
                      else if (it.paid && it.entryId) undo.mutate(it.entryId);
                      else if (!it.paid) setDialog({ kind: "pay", item: it });
                    }}
                  />
                  <span className="min-w-0 flex-1">
                    <span className={cn("block truncate text-sm font-medium", it.paid && "line-through decoration-muted-foreground/60")}>{it.name}</span>
                    <span className="block truncate text-xs text-muted-foreground">{itemSubtitle(it)}</span>
                  </span>
                  <span className="flex flex-col items-end">
                    <Money cents={it.amountCents} className="text-sm font-medium" />
                    {it.kind === "bill" && it.actualCents === undefined && !it.paid && <span className="text-[0.7rem] text-muted-foreground">previsto</span>}
                  </span>
                  {it.kind === "bill" && !it.paid ? (
                    <Button variant="ghost" size="icon-sm" aria-label={`Informar valor real de ${it.name}`} onClick={() => setDialog({ kind: "actual", item: it })} className="text-muted-foreground">
                      <Receipt />
                    </Button>
                  ) : (
                    <span className="size-8 shrink-0" />
                  )}
                </motion.li>
              ))}
            </AnimatePresence>
          </ul>
          {a && a.items.length === 0 && <p className="py-2 text-center text-sm text-muted-foreground">Nada vence neste mês.</p>}
        </Card>

        <div className="flex flex-col gap-4 lg:col-span-2">
          <FinancingsCard onEdit={(financing) => setDialog({ kind: "financing", financing })} onCorrect={(financing) => setDialog({ kind: "correct", financing })} />
          <BillsCard onEdit={(bill) => setDialog({ kind: "bill", bill })} />
        </div>
      </div>

      <AgendaPayDialog item={dialog?.kind === "pay" ? dialog.item : null} month={param} onClose={() => setDialog(null)} />
      <ActualDialog item={dialog?.kind === "actual" ? dialog.item : null} month={param} onClose={() => setDialog(null)} />
      <BillFormDialog open={dialog?.kind === "bill"} bill={dialog?.kind === "bill" ? dialog.bill : undefined} onClose={() => setDialog(null)} />
      <FinancingFormDialog open={dialog?.kind === "financing"} financing={dialog?.kind === "financing" ? dialog.financing : undefined} onClose={() => setDialog(null)} />
      <CorrectionDialog financing={dialog?.kind === "correct" ? dialog.financing : null} onClose={() => setDialog(null)} />
    </div>
  );
}

/** Second line of an agenda item, by kind. */
function itemSubtitle(it: AgendaItem) {
  const due = `vence ${formatDayMonth(it.dueOn)}`;
  if (it.paid && it.paidOn) return `pago em ${formatDayMonth(it.paidOn)}${it.accountName ? ` · ${it.accountName}` : ""}`;
  if (it.kind === "financing") return `${due} · parcela ${it.number} de ${it.total}`;
  if (it.kind === "invoice") return `${due} · fatura do cartão`;
  return `${due} · ${it.accountName ?? ""}`;
}

/** Financings with paid and remaining installments and the outstanding balance (RF-41). */
function FinancingsCard({ onEdit, onCorrect }: { onEdit: (f: Financing) => void; onCorrect: (f: Financing) => void }) {
  const financings = useFinancings();
  const item = "flex cursor-pointer items-center gap-2 rounded-lg px-3 py-2 text-sm outline-none data-[highlighted]:bg-accent";
  return (
    <Card className="flex flex-col gap-3">
      <CardTitle>Financiamentos</CardTitle>
      {financings.data?.length === 0 && <p className="text-sm text-muted-foreground">Casa, lote, carro, energia solar: cadastre com a situação de hoje.</p>}
      {(financings.data ?? []).map((f) => {
        const progress = (f.paidInstallments / f.totalInstallments) * 100;
        return (
          <div key={f.id} className="flex flex-col gap-2 rounded-xl bg-muted p-3">
            <div className="flex items-start justify-between gap-2">
              <div className="flex min-w-0 items-center gap-2">
                <CategoryIcon name={f.categoryIcon} className="size-4 shrink-0 text-primary" />
                <span className="truncate text-sm font-medium">{f.name}</span>
                {f.done && <span className="rounded-full bg-success/10 px-2 py-0.5 text-[0.7rem] font-medium text-success">Quitado</span>}
              </div>
              <DropdownMenu.Root>
                <DropdownMenu.Trigger asChild>
                  <Button variant="ghost" size="icon-sm" aria-label={`Ações de ${f.name}`} className="-my-1">
                    <MoreHorizontal />
                  </Button>
                </DropdownMenu.Trigger>
                <DropdownMenu.Portal>
                  <DropdownMenu.Content align="end" sideOffset={4} className="z-50 min-w-48 rounded-xl border bg-card p-1 shadow-lg data-[state=open]:animate-in data-[state=open]:fade-in-0 motion-reduce:animate-none">
                    <DropdownMenu.Item className={item} onSelect={() => onCorrect(f)}>
                      <Scale className="size-4" />
                      Corrigir saldo e parcela
                    </DropdownMenu.Item>
                    <DropdownMenu.Item className={item} onSelect={() => onEdit(f)}>
                      <Pencil className="size-4" />
                      Editar
                    </DropdownMenu.Item>
                  </DropdownMenu.Content>
                </DropdownMenu.Portal>
              </DropdownMenu.Root>
            </div>
            <div className="h-2 overflow-hidden rounded-full bg-background" role="meter" aria-label={`Parcelas pagas de ${f.name}`} aria-valuemin={0} aria-valuemax={f.totalInstallments} aria-valuenow={f.paidInstallments}>
              <motion.div className="h-full rounded-full bg-gradient-to-r from-success-strong to-success-soft" initial={{ width: 0 }} animate={{ width: `${progress}%` }} transition={spring} />
            </div>
            <div className="flex justify-between gap-2 text-xs">
              <span>
                <span className="block text-muted-foreground">Parcelas</span>
                <span className="font-medium tabular-nums">
                  {f.paidInstallments} de {f.totalInstallments} · faltam {f.remainingInstallments}
                </span>
              </span>
              <span className="text-right">
                <span className="block text-muted-foreground">Saldo devedor</span>
                <Money cents={f.balanceCents} className="text-sm font-semibold" />
              </span>
            </div>
          </div>
        );
      })}
    </Card>
  );
}

/** Registered fixed bills with due day, expected amount and account (RF-35). */
function BillsCard({ onEdit }: { onEdit: (b: Bill) => void }) {
  const bills = useBills();
  return (
    <Card className="flex flex-col gap-2">
      <CardTitle>Contas fixas</CardTitle>
      {bills.data?.length === 0 && <p className="text-sm text-muted-foreground">Energia, água, internet, aluguel: o que sai da conta todo mês.</p>}
      <ul className="flex flex-col">
        {(bills.data ?? []).map((b) => (
          <li key={b.id}>
            <button type="button" onClick={() => onEdit(b)} className="-mx-2 flex w-[calc(100%+1rem)] items-center gap-3 rounded-xl px-2 py-2 text-left hover:bg-accent" aria-label={`Editar ${b.name}`}>
              <span className="grid size-8 shrink-0 place-items-center rounded-lg bg-muted text-xs font-semibold tabular-nums">{String(b.dueDay).padStart(2, "0")}</span>
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm font-medium">{b.name}</span>
                <span className="block truncate text-xs text-muted-foreground">
                  {b.categoryName} · {b.accountName}
                  {b.endMonth ? ` · até ${b.endMonth.split("-").reverse().join("/")}` : ""}
                </span>
              </span>
              <Money cents={b.expectedCents} className="text-sm" />
            </button>
          </li>
        ))}
      </ul>
      <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
        <CreditCard className="size-3.5" />
        As faturas dos cartões entram na lista do mês sozinhas.
      </p>
    </Card>
  );
}
