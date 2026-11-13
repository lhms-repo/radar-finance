import { AnimatePresence, motion } from "motion/react";
import { ArrowRight, Scale, Undo2 } from "lucide-react";
import { useState } from "react";
import { useMe } from "@/app/session";
import { Button } from "@/components/ui/button";
import { Card, CardDescription, CardTitle } from "@/components/ui/card";
import { Dialog, DialogActions } from "@/components/ui/dialog";
import { Field, FieldError } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Money } from "@/components/ui/money";
import { MoneyInput } from "@/components/ui/money-input";
import { NativeSelect } from "@/components/ui/native-select";
import { useAccounts } from "@/features/accounts/api";
import { useDeleteSettlementPayment, useSettlement, useSettlementPayment, type SettlementSuggestion } from "@/features/settle/api";
import { formatDate, formatDayMonth, monthName, monthParam, todayISO } from "@/lib/format";
import { useMonth } from "@/lib/month";
import { fade, spring } from "@/lib/motion";
import { cn } from "@/lib/utils";

/** What the payment dialog starts with: a suggestion, or nothing for a free payment. */
type Paying = SettlementSuggestion | "free" | null;

/** Settlement between the members: balances, who pays whom, payments made and the expenses of the month (RF-84, RN-29). */
export function SettlementPage() {
  const me = useMe();
  const { year, month } = useMonth();
  const personal = me?.group.personal ?? false;
  const settlement = useSettlement(monthParam(year, month), !personal);
  const undo = useDeleteSettlementPayment();
  const [paying, setPaying] = useState<Paying>(null);
  const st = settlement.data;

  if (personal) {
    return (
      <Card className="flex flex-col items-center gap-3 py-10 text-center">
        <Scale className="size-8 text-primary" />
        <p className="max-w-xs text-sm text-muted-foreground">O acerto é entre os membros de um grupo. Troque para um grupo para ver quem paga quem.</p>
      </Card>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <h1 className="px-1 text-lg font-semibold md:hidden">Acerto</h1>

      <Card className="flex flex-col gap-3">
        <div className="flex items-start justify-between gap-3">
          <div>
            <CardTitle>Saldos</CardTitle>
            <CardDescription>{st?.from ? `Desde ${formatDate(st.from)}. Positivo é quanto a pessoa tem a receber.` : "Positivo é quanto a pessoa tem a receber."}</CardDescription>
          </div>
          <Button size="sm" variant="outline" onClick={() => setPaying("free")}>
            Registrar pagamento
          </Button>
        </div>
        <ul className="flex flex-col">
          {(st?.members ?? []).map((m) => (
            <li key={m.personId} className="flex items-center gap-3 border-b py-2.5 last:border-b-0">
              <span className="min-w-0 flex-1">
                <span className="block truncate font-medium">{m.name}</span>
                <span className="block text-xs text-muted-foreground">
                  pagou <Money cents={m.paidCents} />, cota <Money cents={m.shareCents} />
                  {(m.sentCents > 0 || m.receivedCents > 0) && (
                    <>
                      , acertos <Money cents={m.sentCents - m.receivedCents} signed />
                    </>
                  )}
                </span>
              </span>
              <Money
                cents={m.balanceCents}
                signed
                className={cn("text-lg font-semibold", m.balanceCents > 0 && "text-success", m.balanceCents < 0 && "text-destructive")}
              />
            </li>
          ))}
        </ul>
      </Card>

      <Card className="flex flex-col gap-3">
        <CardTitle>Quem paga quem</CardTitle>
        {st && st.suggestions.length === 0 && <p className="text-sm text-muted-foreground">Tudo acertado. Ninguém deve nada.</p>}
        <ul className="flex flex-col gap-2">
          <AnimatePresence initial={false}>
            {(st?.suggestions ?? []).map((s) => (
              <motion.li
                key={`${s.fromPersonId}-${s.toPersonId}`}
                layout
                initial={{ opacity: 0, y: -6 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0 }}
                transition={{ ...spring, opacity: fade }}
                className="flex items-center gap-3 rounded-xl bg-muted px-3 py-2.5"
              >
                <span className="flex min-w-0 flex-1 items-center gap-2 text-sm">
                  <span className="truncate font-medium">{s.fromName}</span>
                  <ArrowRight className="size-4 shrink-0 text-muted-foreground" />
                  <span className="truncate font-medium">{s.toName}</span>
                </span>
                <Money cents={s.amountCents} className="font-semibold" />
                <Button size="sm" onClick={() => setPaying(s)} aria-label={`Registrar ${s.fromName} paga ${s.toName}`}>
                  Registrar
                </Button>
              </motion.li>
            ))}
          </AnimatePresence>
        </ul>
      </Card>

      <Card className="flex flex-col gap-3">
        <CardTitle>
          Gastos do grupo em {monthName(year, month).toLowerCase()} de {year}
        </CardTitle>
        {st && st.lines.length === 0 && <p className="text-sm text-muted-foreground">Nenhum gasto do grupo neste mês.</p>}
        <ul className="flex flex-col">
          {(st?.lines ?? []).map((l, i) => (
            <li key={`${l.date}-${i}`} className="flex flex-col gap-1 border-b py-2.5 last:border-b-0">
              <span className="flex items-center gap-3">
                <span className="w-10 shrink-0 text-xs text-muted-foreground">{formatDayMonth(l.date)}</span>
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm font-medium">{l.description}</span>
                  <span className="block truncate text-xs text-muted-foreground">
                    {l.payerName} pagou, {l.source}
                  </span>
                </span>
                <Money cents={l.amountCents} className="text-sm font-medium" />
              </span>
              <span className="pl-13 text-xs text-muted-foreground">
                {l.shares.map((s) => (
                  <span key={s.personId} className="mr-3 inline-block">
                    {s.name} <Money cents={s.cents} />
                  </span>
                ))}
              </span>
            </li>
          ))}
        </ul>
      </Card>

      {st && st.payments.length > 0 && (
        <Card className="flex flex-col gap-3">
          <CardTitle>Pagamentos de acerto</CardTitle>
          <ul className="flex flex-col">
            {st.payments.map((p) => (
              <li key={p.id} className="flex items-center gap-3 border-b py-2.5 last:border-b-0">
                <span className="w-10 shrink-0 text-xs text-muted-foreground">{formatDayMonth(p.date)}</span>
                <span className="min-w-0 flex-1 truncate text-sm">
                  {p.fromName} pagou {p.toName}
                </span>
                <Money cents={p.amountCents} className="text-sm font-medium" />
                <Button variant="ghost" size="icon-sm" aria-label={`Desfazer pagamento de ${p.fromName}`} disabled={undo.isPending} onClick={() => undo.mutate(p.id)}>
                  <Undo2 />
                </Button>
              </li>
            ))}
          </ul>
        </Card>
      )}

      <PaymentDialog key={paying === null ? "closed" : paying === "free" ? "free" : `${paying.fromPersonId}-${paying.toPersonId}`} paying={paying} onClose={() => setPaying(null)} />
    </div>
  );
}

/** Records a payment between members, from an account of one to an account of the other (RN-30). */
function PaymentDialog({ paying, onClose }: { paying: Paying; onClose: () => void }) {
  const accounts = useAccounts();
  const pay = useSettlementPayment();
  const suggestion = paying && paying !== "free" ? paying : undefined;
  const active = (accounts.data ?? []).filter((a) => a.active);
  const first = (personId?: number) => active.find((a) => a.personId === personId)?.id ?? 0;
  const [from, setFrom] = useState(first(suggestion?.fromPersonId));
  const [to, setTo] = useState(first(suggestion?.toPersonId));
  const [cents, setCents] = useState(suggestion?.amountCents ?? 0);
  const [date, setDate] = useState(todayISO());

  /** Clears the mutation state and closes. */
  const close = () => {
    pay.reset();
    onClose();
  };

  /** Lists the accounts, of one person when the suggestion names her. */
  const options = (personId?: number) =>
    active
      .filter((a) => personId === undefined || a.personId === personId)
      .map((a) => (
        <option key={a.id} value={a.id}>
          {a.name} ({a.personName})
        </option>
      ));

  return (
    <Dialog open={paying !== null} onOpenChange={(o) => !o && close()} title="Pagamento de acerto" description="Uma transferência entre contas de membros diferentes. Não conta como gasto nem como renda.">
      <form
        onSubmit={(e) => {
          e.preventDefault();
          pay.mutate({ fromAccountId: from, toAccountId: to, amountCents: cents, date }, { onSuccess: close });
        }}
        className="flex flex-col gap-4"
        noValidate
      >
        <Field label="Sai da conta" htmlFor="settle-from">
          <NativeSelect id="settle-from" value={from} onChange={(e) => setFrom(Number(e.target.value))}>
            <option value={0}>Escolha a conta</option>
            {options(suggestion?.fromPersonId)}
          </NativeSelect>
        </Field>
        <Field label="Entra na conta" htmlFor="settle-to">
          <NativeSelect id="settle-to" value={to} onChange={(e) => setTo(Number(e.target.value))}>
            <option value={0}>Escolha a conta</option>
            {options(suggestion?.toPersonId)}
          </NativeSelect>
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Valor" htmlFor="settle-amount">
            <MoneyInput id="settle-amount" value={cents} onChange={setCents} />
          </Field>
          <Field label="Data" htmlFor="settle-date">
            <Input id="settle-date" type="date" value={date} onChange={(e) => setDate(e.target.value)} />
          </Field>
        </div>
        <FieldError message={pay.error?.message} />
        <DialogActions>
          <Button variant="ghost" onClick={close}>
            Cancelar
          </Button>
          <Button type="submit" disabled={pay.isPending || !from || !to || cents <= 0}>
            Registrar
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}
