import { zodResolver } from "@hookform/resolvers/zod";
import NumberFlow from "@number-flow/react";
import { AnimatePresence, motion } from "motion/react";
import { Check, ChevronDown, Copy, HandCoins, MessageCircle, Undo2 } from "lucide-react";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { toast } from "sonner";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Card, CardDescription, CardTitle } from "@/components/ui/card";
import { Dialog, DialogActions } from "@/components/ui/dialog";
import { Field, FieldError } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Money } from "@/components/ui/money";
import { MoneyFlow } from "@/components/ui/money-flow";
import { NativeSelect } from "@/components/ui/native-select";
import { useAccounts } from "@/features/accounts/api";
import { fetchChargeMessage, useMarkReceived, useReceivables, useUndoReceipt, type ReceivablePerson } from "@/features/receivables/api";
import { formatDayMonth, todayISO } from "@/lib/format";
import { haptic } from "@/lib/haptics";
import { fade, spring } from "@/lib/motion";
import { cn } from "@/lib/utils";

/** Receivables screen: what each person owes, marking payments and the charge message (RF-26, RF-28 to RF-30). */
export function ReceivablesPage() {
  const receivables = useReceivables();
  const people = receivables.data ?? [];
  const pending = people.filter((p) => p.pendingCents > 0).length;
  return (
    <div className="flex flex-col gap-4">
      <h1 className="px-1 text-lg font-semibold md:hidden">A receber</h1>
      {receivables.isSuccess && people.length === 0 && (
        <Card className="flex flex-col items-center gap-3 py-10 text-center">
          <span className="grid size-14 place-items-center rounded-2xl bg-primary/10">
            <HandCoins className="size-7 text-primary" />
          </span>
          <p className="max-w-xs text-sm text-muted-foreground">Ao lançar uma compra no cartão, ligue "Dividir com outras pessoas". As cotas aparecem aqui, por pessoa.</p>
        </Card>
      )}
      {people.length > 0 && (
        <p className="px-1 text-sm text-muted-foreground">
          <NumberFlow value={pending} className="font-semibold text-foreground" /> {pending === 1 ? "pessoa com pendências" : "pessoas com pendências"}
        </p>
      )}
      <div className="grid gap-4 lg:grid-cols-2">
        {people.map((p) => (
          <PersonCard key={p.partyId} person={p} />
        ))}
      </div>
    </div>
  );
}

/** One person: pending total, installments to pick, received ones and the charge button. */
function PersonCard({ person }: { person: ReceivablePerson }) {
  const [selected, setSelected] = useState<number[]>([]);
  const [paying, setPaying] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [showReceived, setShowReceived] = useState(false);
  const undo = useUndoReceipt();
  const pending = person.items.filter((it) => !it.received);
  const received = person.items.filter((it) => it.received);
  const settled = person.pendingCents === 0;

  /** Toggles an installment in the selection. */
  const toggle = (id: number) => {
    haptic();
    setSelected((s) => (s.includes(id) ? s.filter((x) => x !== id) : [...s, id]));
  };

  /** Loads the charge message and opens it. */
  const charge = async () => {
    try {
      setMessage(await fetchChargeMessage(person.partyId));
    } catch (err) {
      toast.error((err as Error).message);
    }
  };

  return (
    <Card className="flex flex-col gap-3">
      <div className="flex items-start justify-between gap-3">
        <div>
          <CardTitle className={cn("text-base transition-colors", settled && "text-muted-foreground line-through")}>{person.name}</CardTitle>
          <CardDescription>{settled ? "Tudo pago" : `${pending.length} ${pending.length === 1 ? "parcela pendente" : "parcelas pendentes"}`}</CardDescription>
        </div>
        <MoneyFlow cents={person.pendingCents} className={cn("text-2xl font-semibold", settled && "text-success")} />
      </div>

      <ul className="flex flex-col">
        <AnimatePresence initial={false}>
          {pending.map((it) => {
            const checked = selected.includes(it.id);
            return (
              <motion.li key={it.id} layout initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0, height: 0 }} transition={{ ...spring, opacity: fade }}>
                <label className="flex cursor-pointer items-center gap-3 border-b py-2 last:border-b-0">
                  <input type="checkbox" className="size-4 accent-primary" checked={checked} onChange={() => toggle(it.id)} aria-label={`${it.merchant} ${it.number} de ${it.count}`} />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-medium">{it.merchant}</span>
                    <span className="block truncate text-xs text-muted-foreground">
                      {it.count > 1 ? `parcela ${it.number} de ${it.count} · ` : ""}fatura vence {formatDayMonth(it.dueOn)} · {it.cardName}
                    </span>
                  </span>
                  <Money cents={it.amountCents} className="text-sm font-medium" />
                </label>
              </motion.li>
            );
          })}
        </AnimatePresence>
      </ul>

      {pending.length > 0 && (
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" size="sm" onClick={() => setSelected(selected.length === pending.length ? [] : pending.map((it) => it.id))}>
            {selected.length === pending.length ? "Desmarcar todas" : "Marcar todas"}
          </Button>
          <Button size="sm" disabled={selected.length === 0} onClick={() => setPaying(true)}>
            <Check />
            Recebi
          </Button>
          <Button size="sm" variant="outline" className="ml-auto" onClick={charge}>
            <MessageCircle />
            Cobrar
          </Button>
        </div>
      )}

      {received.length > 0 && (
        <div>
          <button type="button" onClick={() => setShowReceived((s) => !s)} className="flex items-center gap-1 text-xs text-muted-foreground" aria-expanded={showReceived}>
            <motion.span animate={{ rotate: showReceived ? 180 : 0 }} transition={spring}>
              <ChevronDown className="size-3.5" />
            </motion.span>
            {received.length} {received.length === 1 ? "recebida" : "recebidas"}
          </button>
          <AnimatePresence initial={false}>
            {showReceived && (
              <motion.ul initial={{ height: 0, opacity: 0 }} animate={{ height: "auto", opacity: 1 }} exit={{ height: 0, opacity: 0 }} transition={{ height: spring, opacity: fade }} className="overflow-hidden">
                {received.map((it) => (
                  <li key={it.id} className="flex items-center gap-3 py-1.5 text-sm text-muted-foreground">
                    <Check className="size-4 shrink-0 text-success" />
                    <span className="min-w-0 flex-1 truncate line-through">
                      {it.merchant}
                      {it.count > 1 ? ` ${it.number}/${it.count}` : ""}
                    </span>
                    <span className="text-xs">{it.receivedOn ? formatDayMonth(it.receivedOn) : ""}</span>
                    <Money cents={it.amountCents} className="text-xs" />
                    <Button variant="ghost" size="icon-sm" aria-label={`Desfazer recebimento de ${it.merchant}`} disabled={undo.isPending} onClick={() => it.entryId && undo.mutate(it.entryId)}>
                      <Undo2 />
                    </Button>
                  </li>
                ))}
              </motion.ul>
            )}
          </AnimatePresence>
        </div>
      )}

      <ReceiptDialog open={paying} person={person} itemIds={selected} onClose={() => setPaying(false)} onDone={() => setSelected([])} />
      <MessageDialog message={message} name={person.name} onClose={() => setMessage(null)} />
    </Card>
  );
}

const receiptSchema = z.object({
  accountId: z.number().int().positive("Escolha a conta."),
  date: z.string().regex(/^\d{4}-\d{2}-\d{2}$/, "Informe a data."),
});

/** Records the chosen shares as paid into an account; it is not income (RF-30, RN-06). */
function ReceiptDialog({ open, person, itemIds, onClose, onDone }: { open: boolean; person: ReceivablePerson; itemIds: number[]; onClose: () => void; onDone: () => void }) {
  const accounts = useAccounts();
  const mark = useMarkReceived();
  const form = useForm<z.infer<typeof receiptSchema>>({ resolver: zodResolver(receiptSchema), resetOptions: { keepDirtyValues: true }, values: { accountId: 0, date: todayISO() } });
  const errors = form.formState.errors;

  /** Clears the mutation state and closes. */
  const close = () => {
    form.reset();
    mark.reset();
    onClose();
  };

  /** Validates and sends the receipt. */
  const submit = form.handleSubmit((v) =>
    mark.mutate(
      { itemIds, ...v },
      {
        onSuccess: () => {
          onDone();
          close();
        },
      },
    ),
  );

  return (
    <Dialog open={open} onOpenChange={(o) => !o && close()} title={`Recebi de ${person.name}`} description={`${itemIds.length} ${itemIds.length === 1 ? "parcela" : "parcelas"} numa entrada só na conta.`}>
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Caiu na conta" htmlFor="receipt-account" error={errors.accountId?.message}>
            <NativeSelect id="receipt-account" {...form.register("accountId", { valueAsNumber: true })}>
              <option value={0}>Escolha</option>
              {(accounts.data ?? []).filter((a) => a.active).map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </NativeSelect>
          </Field>
          <Field label="Data" htmlFor="receipt-date" error={errors.date?.message}>
            <Input id="receipt-date" type="date" {...form.register("date")} />
          </Field>
        </div>
        <p className="text-xs text-muted-foreground">Entra no saldo da conta, mas não conta como renda do mês.</p>
        <FieldError message={mark.error?.message} />
        <DialogActions>
          <Button variant="ghost" onClick={close}>
            Cancelar
          </Button>
          <Button type="submit" disabled={mark.isPending}>
            <Check />
            Registrar
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}

/** Shows the charge message with copy and WhatsApp buttons (RF-29). */
function MessageDialog({ message, name, onClose }: { message: string | null; name: string; onClose: () => void }) {
  /** Copies the message to the clipboard. */
  const copy = async () => {
    if (!message) return;
    try {
      await navigator.clipboard.writeText(message);
      haptic();
      toast.success("Mensagem copiada");
    } catch {
      toast.error("Não foi possível copiar. Selecione o texto e copie.");
    }
  };

  return (
    <Dialog open={message !== null} onOpenChange={(o) => !o && onClose()} title={`Cobrar ${name}`} description="Confira e envie pelo WhatsApp.">
      <pre className="max-h-72 overflow-auto whitespace-pre-wrap rounded-xl bg-muted p-3 font-sans text-sm select-text">{message}</pre>
      <DialogActions>
        <Button variant="outline" onClick={copy}>
          <Copy />
          Copiar
        </Button>
        <a
          href={`https://wa.me/?text=${encodeURIComponent(message ?? "")}`}
          target="_blank"
          rel="noreferrer"
          className="inline-flex h-10 items-center justify-center gap-2 rounded-xl bg-success px-4 text-sm font-medium text-white shadow-sm transition-transform active:scale-[0.97] [&_svg]:size-4"
        >
          <MessageCircle />
          Abrir no WhatsApp
        </a>
      </DialogActions>
    </Dialog>
  );
}
