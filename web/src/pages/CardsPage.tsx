import { AnimatePresence, motion } from "motion/react";
import { DropdownMenu } from "radix-ui";
import { AlertTriangle, Check, CreditCard, Eye, EyeOff, MinusCircle, MoreHorizontal, Pencil, Plus, Repeat, Trash2, Undo2 } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { ApiRequestError } from "@/api/client";
import { Button } from "@/components/ui/button";
import { Card as Surface, CardDescription, CardTitle } from "@/components/ui/card";
import { Dialog, DialogActions } from "@/components/ui/dialog";
import { Money } from "@/components/ui/money";
import { MoneyFlow } from "@/components/ui/money-flow";
import {
  useCards,
  useDeleteCard,
  useDeletePurchase,
  useDeleteRecurrence,
  useForecast,
  useInvoice,
  useRecurrences,
  useUndoPayment,
  useUpdateCard,
  type Card,
  type Invoice,
  type InvoiceItem,
  type InvoiceStatus,
  type InvoiceSummary,
  type Recurrence,
} from "@/features/cards/api";
import { CardVisual } from "@/features/cards/CardVisual";
import { CardFormDialog, CreditDialog, PayDialog, PurchaseDialog, RecurrenceDialog } from "@/features/cards/dialogs";
import { CategoryIcon } from "@/features/categories/icons";
import { formatDate, formatDayMonth, formatMoney, monthName, monthParam } from "@/lib/format";
import { haptic } from "@/lib/haptics";
import { fade, spring } from "@/lib/motion";
import { useMonth } from "@/lib/month";
import { cn, prefersReducedMotion } from "@/lib/utils";

/** Labels of the invoice states. */
const STATUS_LABEL: Record<InvoiceStatus, string> = { open: "Aberta", closed: "Fechada", overdue: "Vencida", paid: "Paga" };

/** Pill colors of the invoice states. */
const STATUS_CLASS: Record<InvoiceStatus, string> = {
  open: "bg-primary/10 text-primary",
  closed: "bg-warning/15 text-warning",
  overdue: "bg-destructive/10 text-destructive",
  paid: "bg-success/10 text-success",
};

type DialogState =
  | { kind: "card"; card?: Card }
  | { kind: "purchase"; item?: InvoiceItem }
  | { kind: "credit" }
  | { kind: "recurrence"; recurrence?: Recurrence }
  | { kind: "pay" }
  | { kind: "delete-card" }
  | { kind: "delete-item"; item: InvoiceItem }
  | { kind: "delete-recurrence"; recurrence: Recurrence };

/** Cards screen: carousel, limit, invoice of the selected month, next invoices and recurrences (RF-14 to RF-23). */
export function CardsPage() {
  const cards = useCards();
  const list = cards.data ?? [];
  const [selectedId, setSelectedId] = useState<number | undefined>();
  const selected = list.find((c) => c.id === selectedId) ?? list[0];
  const { year, month } = useMonth();
  const param = monthParam(year, month);
  const invoice = useInvoice(selected?.id, param);
  const [dialog, setDialog] = useState<DialogState | null>(null);
  const toggle = useUpdateCard();

  if (cards.isPending) return <p className="px-1 text-sm text-muted-foreground">Carregando...</p>;
  if (cards.isError) return <p className="px-1 text-sm text-destructive">Não foi possível carregar os cartões.</p>;

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between gap-3">
        <h1 className="px-1 text-lg font-semibold md:hidden">Cartões</h1>
        <Button size="sm" className="ml-auto" onClick={() => setDialog({ kind: "card" })}>
          <Plus />
          Novo cartão
        </Button>
      </div>

      {list.length === 0 ? (
        <Surface className="flex flex-col items-center gap-3 py-10 text-center">
          <span className="grid size-14 place-items-center rounded-2xl bg-primary/10">
            <CreditCard className="size-7 text-primary" />
          </span>
          <p className="max-w-xs text-sm text-muted-foreground">Cadastre os cartões com os dias de fechamento e vencimento. As compras caem na fatura certa sozinhas.</p>
          <Button onClick={() => setDialog({ kind: "card" })}>
            <Plus />
            Cadastrar o primeiro cartão
          </Button>
        </Surface>
      ) : (
        selected && (
          <>
            <Carousel cards={list} selected={selected} status={invoice.data?.status} onSelect={(c) => setSelectedId(c.id)} />
            <LimitBar card={selected} onEdit={() => setDialog({ kind: "card", card: selected })} onToggle={() =>
              toggle.mutate(
                { id: selected.id, body: { ...cardInput(selected), active: !selected.active } },
                { onError: (err) => toast.error(err.message) },
              )
            } onDelete={() => setDialog({ kind: "delete-card" })} />
            <div className="grid gap-4 lg:grid-cols-5">
              <div className="flex flex-col gap-4 lg:col-span-3">
                {invoice.data && (
                  <InvoiceCard
                    card={selected}
                    invoice={invoice.data}
                    onPay={() => setDialog({ kind: "pay" })}
                    onAdd={(kind) => setDialog({ kind })}
                    onEdit={(item) => setDialog({ kind: "purchase", item })}
                    onDelete={(item) => setDialog({ kind: "delete-item", item })}
                  />
                )}
              </div>
              <div className="flex flex-col gap-4 lg:col-span-2">
                <ForecastCard card={selected} selectedMonth={param} />
                <RecurrencesCard
                  card={selected}
                  onAdd={() => setDialog({ kind: "recurrence" })}
                  onEdit={(recurrence) => setDialog({ kind: "recurrence", recurrence })}
                  onDelete={(recurrence) => setDialog({ kind: "delete-recurrence", recurrence })}
                />
              </div>
            </div>
          </>
        )
      )}

      <CardFormDialog open={dialog?.kind === "card"} card={dialog?.kind === "card" ? dialog.card : undefined} onClose={() => setDialog(null)} />
      {selected && (
        <>
          <PurchaseDialog open={dialog?.kind === "purchase"} card={selected} item={dialog?.kind === "purchase" ? dialog.item : undefined} onClose={() => setDialog(null)} />
          <CreditDialog open={dialog?.kind === "credit"} card={selected} onClose={() => setDialog(null)} />
          <RecurrenceDialog
            open={dialog?.kind === "recurrence"}
            card={selected}
            recurrence={dialog?.kind === "recurrence" ? dialog.recurrence : undefined}
            onClose={() => setDialog(null)}
          />
          {invoice.data && <PayDialog open={dialog?.kind === "pay"} card={selected} invoice={invoice.data} onClose={() => setDialog(null)} />}
          <DeleteCardDialog open={dialog?.kind === "delete-card"} card={selected} onClose={() => setDialog(null)} />
        </>
      )}
      <DeleteItemDialog item={dialog?.kind === "delete-item" ? dialog.item : null} onClose={() => setDialog(null)} />
      <DeleteRecurrenceDialog recurrence={dialog?.kind === "delete-recurrence" ? dialog.recurrence : null} onClose={() => setDialog(null)} />
    </div>
  );
}

/** Editable fields of a card, for updates that change only one of them. */
function cardInput(c: Card) {
  return {
    name: c.name,
    personId: c.personId,
    limitCents: c.limitCents,
    closingDay: c.closingDay,
    dueDay: c.dueDay,
    lastDigits: c.lastDigits,
    color: c.color,
    finish: c.finish,
  };
}

/** Horizontal carousel of card drawings; the next card peeks at the edge, as in the reference (section 5.4). */
function Carousel({ cards, selected, status, onSelect }: { cards: Card[]; selected: Card; status?: InvoiceStatus; onSelect: (c: Card) => void }) {
  const ref = useRef<HTMLDivElement>(null);

  // Scrolls only the carousel, never the page, when the selected card is out of view.
  // scrollIntoView fights scroll snapping in WebKit and keeps the page moving.
  useEffect(() => {
    const box = ref.current;
    const el = box?.querySelector<HTMLElement>(`[data-card="${selected.id}"]`);
    if (!box || !el) return;
    const left = el.offsetLeft - box.offsetLeft;
    if (left >= box.scrollLeft && left + el.offsetWidth <= box.scrollLeft + box.clientWidth) return;
    box.scrollTo({ left, behavior: prefersReducedMotion() ? "auto" : "smooth" });
  }, [selected.id]);

  return (
    <div
      ref={ref}
      role="listbox"
      aria-label="Cartões"
      className="-mx-4 flex snap-x snap-mandatory gap-3 overflow-x-auto px-4 pb-2 [scrollbar-width:none] md:mx-0 md:px-0"
    >
      {cards.map((c) => {
        const active = c.id === selected.id;
        return (
          <motion.button
            key={c.id}
            type="button"
            role="option"
            aria-selected={active}
            aria-label={c.name}
            data-card={c.id}
            onClick={() => {
              haptic();
              onSelect(c);
            }}
            animate={{ scale: active ? 1 : 0.94, opacity: active ? 1 : 0.7 }}
            transition={spring}
            className={cn("w-[82%] shrink-0 snap-start sm:w-80", !c.active && "grayscale")}
          >
            <CardVisual
              name={c.name}
              personName={c.personName}
              lastDigits={c.lastDigits}
              color={c.color}
              finish={c.finish}
              dueLabel={formatDayMonth(c.currentInvoiceDueOn)}
              limitFreeCents={c.limitFreeCents}
              status={active ? status : undefined}
            />
          </motion.button>
        );
      })}
    </div>
  );
}

/** Used and free limit of the card, counting future installments (RF-20, RN-17). */
function LimitBar({ card, onEdit, onToggle, onDelete }: { card: Card; onEdit: () => void; onToggle: () => void; onDelete: () => void }) {
  const used = card.limitCents > 0 ? Math.min(100, (card.limitUsedCents / card.limitCents) * 100) : 0;
  return (
    <Surface className="flex flex-col gap-3">
      <div className="flex items-start justify-between gap-3">
        <div>
          <CardTitle>
            {card.name} · {card.personName}
            {!card.active && " · Desativado"}
          </CardTitle>
          <CardDescription>
            Fecha dia {card.closingDay}, vence dia {card.dueDay}
          </CardDescription>
        </div>
        <CardMenu card={card} onEdit={onEdit} onToggle={onToggle} onDelete={onDelete} />
      </div>
      <div className="h-2.5 overflow-hidden rounded-full bg-muted" role="meter" aria-label="Limite usado" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(used)}>
        <motion.div
          className={cn("h-full rounded-full", used >= 90 ? "bg-destructive" : used >= 70 ? "bg-warning" : "bg-primary")}
          initial={{ width: 0 }}
          animate={{ width: `${used}%` }}
          transition={spring}
        />
      </div>
      <div className="flex justify-between gap-3 text-sm">
        <span>
          <span className="block text-xs text-muted-foreground">Usado, com parcelas futuras</span>
          <Money cents={card.limitUsedCents} className="font-medium" />
        </span>
        <span className="text-right">
          <span className="block text-xs text-muted-foreground">Livre de {<Money cents={card.limitCents} />}</span>
          <Money cents={card.limitFreeCents} className={cn("font-semibold", card.limitFreeCents < 0 ? "text-destructive" : "text-success")} />
        </span>
      </div>
    </Surface>
  );
}

/** Menu with edit, deactivate and delete for the card. */
function CardMenu({ card, onEdit, onToggle, onDelete }: { card: Card; onEdit: () => void; onToggle: () => void; onDelete: () => void }) {
  const item = "flex cursor-pointer items-center gap-2 rounded-lg px-3 py-2 text-sm outline-none data-[highlighted]:bg-accent";
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label="Ações do cartão">
          <MoreHorizontal />
        </Button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="end" sideOffset={4} className="z-50 min-w-44 rounded-xl border bg-card p-1 shadow-lg data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95 motion-reduce:animate-none">
          <DropdownMenu.Item className={item} onSelect={onEdit}>
            <Pencil className="size-4" />
            Editar
          </DropdownMenu.Item>
          <DropdownMenu.Item className={item} onSelect={onToggle}>
            {card.active ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
            {card.active ? "Desativar" : "Reativar"}
          </DropdownMenu.Item>
          <DropdownMenu.Item className={cn(item, "text-destructive")} onSelect={onDelete}>
            <Trash2 className="size-4" />
            Excluir
          </DropdownMenu.Item>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

/** Invoice of the selected month with totals, payment and every line (RF-19, RF-21). */
function InvoiceCard({
  card,
  invoice,
  onPay,
  onAdd,
  onEdit,
  onDelete,
}: {
  card: Card;
  invoice: Invoice;
  onPay: () => void;
  onAdd: (kind: "purchase" | "credit") => void;
  onEdit: (item: InvoiceItem) => void;
  onDelete: (item: InvoiceItem) => void;
}) {
  const undo = useUndoPayment();
  const [y, m] = invoice.month.split("-").map(Number);
  const canPay = !invoice.payment && invoice.status !== "open" && invoice.totalCents > 0;
  return (
    <Surface className="flex flex-col gap-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <p className="text-sm text-muted-foreground">Fatura de {monthName(y!, m!).toLowerCase()}</p>
          <MoneyFlow cents={invoice.totalCents} className="text-3xl font-semibold tracking-tight" />
          <p className="text-xs text-muted-foreground">
            Fecha {formatDate(invoice.closingOn)} · vence {formatDate(invoice.dueOn)}
          </p>
        </div>
        <span className={cn("rounded-full px-2.5 py-1 text-xs font-medium", STATUS_CLASS[invoice.status])}>{STATUS_LABEL[invoice.status]}</span>
      </div>

      <dl className="grid grid-cols-3 gap-2 rounded-xl bg-muted p-3 text-sm">
        <div>
          <dt className="text-xs text-muted-foreground">Total real</dt>
          <dd className="font-medium">
            <Money cents={invoice.totalCents} />
          </dd>
        </div>
        <div>
          <dt className="text-xs text-muted-foreground">Terceiros</dt>
          <dd className="font-medium">
            <Money cents={invoice.thirdPartyCents} />
          </dd>
        </div>
        <div>
          <dt className="text-xs text-muted-foreground">Parte do casal</dt>
          <dd className="font-semibold">
            <Money cents={invoice.netCents} />
          </dd>
        </div>
      </dl>

      {invoice.payment ? (
        <div className="flex items-center justify-between gap-3 rounded-xl bg-success/10 px-3 py-2 text-sm">
          <span className="flex items-center gap-2 text-success">
            <Check className="size-4 shrink-0" />
            Paga em {formatDate(invoice.payment.paidOn)} com {invoice.payment.accountName}
          </span>
          <Button variant="ghost" size="sm" onClick={() => undo.mutate(invoice.payment!.entryId)} disabled={undo.isPending}>
            <Undo2 />
            Desfazer
          </Button>
        </div>
      ) : (
        canPay && (
          <Button onClick={onPay}>
            <Check />
            Pagar fatura
          </Button>
        )
      )}
      {invoice.paymentOutdated && invoice.payment && (
        <p className="flex items-start gap-2 rounded-xl bg-warning/15 px-3 py-2 text-sm text-warning">
          <AlertTriangle className="mt-0.5 size-4 shrink-0" />
          <span>
            O total mudou depois do pagamento de <Money cents={invoice.payment.amountCents} />. Desfaça e pague de novo para acertar a conta.
          </span>
        </p>
      )}

      <div className="flex items-center justify-between gap-2">
        <CardTitle>Lançamentos</CardTitle>
        {card.active && (
          <div className="flex gap-2">
            <Button size="sm" variant="outline" onClick={() => onAdd("credit")}>
              <MinusCircle />
              Crédito
            </Button>
            <Button size="sm" onClick={() => onAdd("purchase")}>
              <Plus />
              Compra
            </Button>
          </div>
        )}
      </div>
      <ul className="flex flex-col">
        <AnimatePresence initial={false}>
          {invoice.items.map((it) => (
            <motion.li
              key={`${it.kind}-${it.purchaseId ?? it.recurrenceId}`}
              layout
              initial={{ opacity: 0, y: -6 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, height: 0 }}
              transition={{ ...spring, opacity: fade }}
              className="flex items-center gap-3 border-b py-2.5 last:border-b-0"
            >
              <span className="w-10 shrink-0 text-xs text-muted-foreground">{it.date ? formatDayMonth(it.date) : ""}</span>
              <span className="grid size-9 shrink-0 place-items-center rounded-xl bg-muted">
                {it.kind === "recurrence" ? <Repeat className="size-4 text-primary" /> : <CategoryIcon name={it.categoryIcon} className="size-4 text-primary" />}
              </span>
              <button
                type="button"
                className="min-w-0 flex-1 text-left"
                onClick={() => it.kind === "purchase" && onEdit(it)}
                disabled={it.kind !== "purchase"}
                aria-label={it.kind === "purchase" ? `Editar ${it.merchant}` : undefined}
              >
                <span className="block truncate text-sm font-medium">{it.merchant}</span>
                <span className="block truncate text-xs text-muted-foreground">{itemSubtitle(it)}</span>
              </button>
              <span className="flex flex-col items-end">
                <Money cents={it.amountCents} className={cn("text-sm font-medium", it.amountCents < 0 && "text-success")} />
                {it.count > 1 && <span className="text-[0.7rem] text-muted-foreground">{`${it.number}/${it.count}`}</span>}
              </span>
              {it.kind !== "recurrence" ? (
                <Button variant="ghost" size="icon-sm" aria-label={`Excluir ${it.merchant}`} onClick={() => onDelete(it)} className="text-muted-foreground">
                  <Trash2 />
                </Button>
              ) : (
                <span className="size-8" />
              )}
            </motion.li>
          ))}
        </AnimatePresence>
      </ul>
      {invoice.items.length === 0 && <p className="py-2 text-center text-sm text-muted-foreground">Nenhum lançamento nesta fatura.</p>}
    </Surface>
  );
}

/** Second line of an invoice item: category, buyer, kind and the shares of other people (RN-05). */
function itemSubtitle(it: InvoiceItem) {
  const shares = (it.shares ?? []).map((s) => `${s.partyName} ${formatMoney(s.amountCents)}${s.received ? " ✓" : ""}`).join(", ");
  const parts = [it.categoryName, it.buyerName && `comprou ${it.buyerName}`, shares && `dividida: ${shares}`];
  if (it.kind === "recurrence") parts.unshift("Recorrência");
  if (it.kind === "credit") parts.unshift("Crédito");
  return parts.filter(Boolean).join(" · ");
}

/** Next invoices as tiles joined by a dashed line; tapping one opens that month (RF-23). */
function ForecastCard({ card, selectedMonth }: { card: Card; selectedMonth: string }) {
  const forecast = useForecast(card.id, 6);
  const { goTo } = useMonth();
  const items = forecast.data ?? [];
  const max = Math.max(1, ...items.map((i) => i.totalCents));
  return (
    <Surface className="flex flex-col gap-3">
      <CardTitle className="text-xs tracking-wide text-muted-foreground uppercase">Próximas faturas</CardTitle>
      <div className="relative grid grid-cols-3 gap-2">
        <ForecastLine items={items} max={max} />
        {items.map((s) => {
          const [y, m] = s.month.split("-").map(Number);
          const active = s.month === selectedMonth;
          return (
            <button
              key={s.month}
              type="button"
              onClick={() => {
                haptic();
                goTo(y!, m!);
              }}
              aria-pressed={active}
              className={cn(
                "relative flex h-24 flex-col justify-between rounded-xl border p-2 text-left transition-colors",
                active ? "border-primary bg-primary/5" : "border-transparent bg-muted hover:bg-accent",
              )}
            >
              <span className="text-xs font-medium">
                {monthName(y!, m!).slice(0, 3)} {String(y).slice(2)}
              </span>
              <Money cents={s.totalCents} className={cn("text-xs font-semibold", active && "text-primary")} />
            </button>
          );
        })}
      </div>
    </Surface>
  );
}

/** Dashed line behind the forecast tiles, higher for bigger invoices. */
function ForecastLine({ items, max }: { items: InvoiceSummary[]; max: number }) {
  if (items.length < 2) return null;
  // Tiles flow in rows of three; draw one line per row.
  const rows = [items.slice(0, 3), items.slice(3, 6)].filter((r) => r.length > 1);
  return (
    <>
      {rows.map((row, r) => (
        <svg
          key={r}
          className="pointer-events-none absolute inset-x-0 z-10 h-24"
          style={{ top: `calc(${r} * (6rem + 0.5rem))` }}
          viewBox="0 0 300 96"
          preserveAspectRatio="none"
          aria-hidden
        >
          <polyline
            points={row.map((s, i) => `${50 + i * 100},${68 - (s.totalCents / max) * 32}`).join(" ")}
            fill="none"
            stroke="currentColor"
            strokeDasharray="3 4"
            strokeWidth="1.5"
            className="text-muted-foreground/60"
            vectorEffect="non-scaling-stroke"
          />
        </svg>
      ))}
    </>
  );
}

/** Recurrences of the card with their current price (RF-17). */
function RecurrencesCard({
  card,
  onAdd,
  onEdit,
  onDelete,
}: {
  card: Card;
  onAdd: () => void;
  onEdit: (r: Recurrence) => void;
  onDelete: (r: Recurrence) => void;
}) {
  const recurrences = useRecurrences(card.id);
  return (
    <Surface className="flex flex-col gap-2">
      <div className="flex items-center justify-between gap-2">
        <CardTitle>Recorrências</CardTitle>
        {card.active && (
          <Button size="sm" variant="outline" onClick={onAdd}>
            <Plus />
            Nova
          </Button>
        )}
      </div>
      <ul className="flex flex-col">
        {(recurrences.data ?? []).map((r) => (
          <li key={r.id} className="flex items-center gap-3 border-b py-2 last:border-b-0">
            <span className="grid size-8 shrink-0 place-items-center rounded-lg bg-muted">
              <CategoryIcon name={r.categoryIcon} className="size-4 text-primary" />
            </span>
            <button type="button" className="min-w-0 flex-1 text-left" onClick={() => onEdit(r)} aria-label={`Editar ${r.description}`}>
              <span className="block truncate text-sm font-medium">{r.description}</span>
              <span className="block truncate text-xs text-muted-foreground">
                {r.categoryName}
                {r.endMonth ? ` · até ${r.endMonth.split("-").reverse().join("/")}` : ""}
              </span>
            </button>
            <Money cents={r.amountCents} className="text-sm font-medium" />
            <Button variant="ghost" size="icon-sm" aria-label={`Excluir ${r.description}`} onClick={() => onDelete(r)} className="text-muted-foreground">
              <Trash2 />
            </Button>
          </li>
        ))}
      </ul>
      {recurrences.data?.length === 0 && <p className="text-sm text-muted-foreground">Streaming, barbearia, internet: o que cai todo mês neste cartão.</p>}
    </Surface>
  );
}

/** Confirms deleting the card and offers to deactivate it when it has history. */
function DeleteCardDialog({ open, card, onClose }: { open: boolean; card: Card; onClose: () => void }) {
  const remove = useDeleteCard();
  const deactivate = useUpdateCard("Cartão desativado");
  const inUse = remove.error instanceof ApiRequestError && remove.error.code === "card_in_use";

  /** Clears the mutation state and closes. */
  const close = () => {
    remove.reset();
    onClose();
  };

  return (
    <Dialog open={open} onOpenChange={(o) => !o && close()} title={`Excluir ${card.name}?`}>
      <p className="text-sm text-muted-foreground">{remove.error ? remove.error.message : "O cartão some de vez. Isso só é possível enquanto ele não tem lançamentos."}</p>
      <DialogActions>
        <Button variant="ghost" onClick={close}>
          Cancelar
        </Button>
        {inUse && card.active ? (
          <Button onClick={() => deactivate.mutate({ id: card.id, body: { ...cardInput(card), active: false } }, { onSuccess: close })}>
            <EyeOff />
            Desativar
          </Button>
        ) : (
          <Button variant="destructive" disabled={remove.isPending || !!remove.error} onClick={() => remove.mutate(card.id, { onSuccess: close })}>
            <Trash2 />
            Excluir
          </Button>
        )}
      </DialogActions>
    </Dialog>
  );
}

/** Confirms deleting a purchase with all its installments, or a credit. */
function DeleteItemDialog({ item, onClose }: { item: InvoiceItem | null; onClose: () => void }) {
  const remove = useDeletePurchase();

  /** Clears the mutation state and closes. */
  const close = () => {
    remove.reset();
    onClose();
  };

  return (
    <Dialog open={item !== null} onOpenChange={(o) => !o && close()} title={`Excluir ${item?.merchant ?? ""}?`}>
      <p className="text-sm text-muted-foreground">
        {item && item.count > 1 ? `As ${item.count} parcelas saem de todas as faturas.` : "O lançamento sai da fatura."}
      </p>
      {remove.error && <p className="text-sm text-destructive">{remove.error.message}</p>}
      <DialogActions>
        <Button variant="ghost" onClick={close}>
          Cancelar
        </Button>
        <Button variant="destructive" disabled={remove.isPending} onClick={() => item?.purchaseId && remove.mutate(item.purchaseId, { onSuccess: close })}>
          <Trash2 />
          Excluir
        </Button>
      </DialogActions>
    </Dialog>
  );
}

/** Confirms deleting a recurrence; one already on paid invoices must be ended instead. */
function DeleteRecurrenceDialog({ recurrence, onClose }: { recurrence: Recurrence | null; onClose: () => void }) {
  const remove = useDeleteRecurrence();

  /** Clears the mutation state and closes. */
  const close = () => {
    remove.reset();
    onClose();
  };

  return (
    <Dialog open={recurrence !== null} onOpenChange={(o) => !o && close()} title={`Excluir ${recurrence?.description ?? ""}?`}>
      <p className="text-sm text-muted-foreground">{remove.error ? remove.error.message : "A recorrência sai de todas as faturas. Para parar de cobrar, prefira informar a última fatura."}</p>
      <DialogActions>
        <Button variant="ghost" onClick={close}>
          Cancelar
        </Button>
        <Button variant="destructive" disabled={remove.isPending || !!remove.error} onClick={() => recurrence && remove.mutate(recurrence.id, { onSuccess: close })}>
          <Trash2 />
          Excluir
        </Button>
      </DialogActions>
    </Dialog>
  );
}
