import { CheckCircle2 } from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardDescription, CardTitle } from "@/components/ui/card";
import { Dialog, DialogActions } from "@/components/ui/dialog";
import { Field, FieldError } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Money } from "@/components/ui/money";
import { MoneyInput } from "@/components/ui/money-input";
import { NativeSelect } from "@/components/ui/native-select";
import { Segmented } from "@/components/ui/segmented";
import { useAccounts } from "@/features/accounts/api";
import { useAleloCards } from "@/features/alelo/api";
import { useCards } from "@/features/cards/api";
import { useFinishList, type ShoppingListDetail } from "@/features/market/api";
import { formatDate, todayISO } from "@/lib/format";

/** Ways to pay, as shown. */
const METHODS = [
  { value: "account", label: "Conta" },
  { value: "card", label: "Cartão" },
  { value: "alelo", label: "Alelo" },
] as const;

/** Labels of the payment ways in the summary. */
const METHOD_LABEL = { account: "na conta", card: "no cartão", alelo: "no Alelo" } as const;

/** Real total of the purchase, how it was paid and the description of the expense (RF-77, RN-26). */
export function FinishDialog({ open, list, suggestion, onClose }: { open: boolean; list: ShoppingListDetail; suggestion: string; onClose: () => void }) {
  const finish = useFinishList();
  const accounts = useAccounts();
  const cards = useCards();
  const alelos = useAleloCards();
  const [method, setMethod] = useState<"account" | "card" | "alelo">("card");
  const [total, setTotal] = useState(0);
  const [date, setDate] = useState(todayISO());
  const [description, setDescription] = useState<string | null>(null);
  const [sourceId, setSourceId] = useState(0);
  const [installments, setInstallments] = useState(1);
  const options =
    method === "account"
      ? (accounts.data ?? []).filter((a) => a.active).map((a) => ({ id: a.id, name: a.name }))
      : method === "card"
        ? (cards.data ?? []).filter((c) => c.active).map((c) => ({ id: c.id, name: c.name }))
        : (alelos.data ?? []).filter((a) => a.active).map((a) => ({ id: a.id, name: a.name }));
  const chosen = options.some((o) => o.id === sourceId) ? sourceId : (options[0]?.id ?? 0);
  const desc = description ?? suggestion;

  /** Clears the mutation state and closes. */
  const close = () => {
    finish.reset();
    onClose();
  };

  return (
    <Dialog open={open} onOpenChange={(o) => !o && close()} title="Finalizar compra" description="O gasto entra em Alimentação, Compras para casa.">
      <form
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault();
          finish.mutate(
            {
              id: list.id,
              body: {
                method,
                totalCents: total,
                date,
                description: desc,
                ...(method === "account" ? { accountId: chosen } : method === "card" ? { cardId: chosen, installments } : { aleloId: chosen }),
              },
            },
            { onSuccess: close },
          );
        }}
      >
        <Field label="Total pago" htmlFor="finish-total">
          <MoneyInput id="finish-total" value={total} onChange={setTotal} autoFocus />
        </Field>
        <Segmented ariaLabel="Como pagou" value={method} options={[...METHODS]} onChange={(v) => setMethod(v)} />
        <div className="grid grid-cols-2 gap-3">
          <Field label={method === "account" ? "Conta" : method === "card" ? "Cartão" : "Cartão Alelo"} htmlFor="finish-source">
            <NativeSelect id="finish-source" value={chosen} onChange={(e) => setSourceId(Number(e.target.value))}>
              {options.length === 0 && <option value={0}>Nenhum cadastrado</option>}
              {options.map((o) => (
                <option key={o.id} value={o.id}>
                  {o.name}
                </option>
              ))}
            </NativeSelect>
          </Field>
          {method === "card" ? (
            <Field label="Parcelas" htmlFor="finish-installments">
              <Input id="finish-installments" type="number" inputMode="numeric" min={1} max={48} value={installments} onChange={(e) => setInstallments(Number(e.target.value))} />
            </Field>
          ) : (
            <Field label="Data" htmlFor="finish-date">
              <Input id="finish-date" type="date" value={date} onChange={(e) => setDate(e.target.value)} />
            </Field>
          )}
        </div>
        {method === "card" && (
          <Field label="Data" htmlFor="finish-date">
            <Input id="finish-date" type="date" value={date} onChange={(e) => setDate(e.target.value)} />
          </Field>
        )}
        <Field label="Descrição" htmlFor="finish-description">
          <Input id="finish-description" autoComplete="off" value={desc} onChange={(e) => setDescription(e.target.value)} />
        </Field>
        <FieldError message={finish.error?.message} />
        <DialogActions>
          <Button variant="ghost" onClick={close}>
            Cancelar
          </Button>
          <Button type="submit" disabled={finish.isPending || !chosen}>
            Finalizar
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}

/** Summary of a finished purchase. */
export function PaidSummary({ list }: { list: ShoppingListDetail }) {
  const paid = list.paid;
  if (!paid) return null;
  return (
    <Card className="flex items-center gap-3">
      <CheckCircle2 className="size-6 shrink-0 text-success" />
      <div className="min-w-0 flex-1">
        <CardTitle>Compra finalizada</CardTitle>
        <CardDescription>
          {paid.description}, {METHOD_LABEL[paid.method]} em {formatDate(paid.date)}
        </CardDescription>
      </div>
      <Money cents={paid.totalCents} className="text-lg font-semibold" />
    </Card>
  );
}
