import { zodResolver } from "@hookform/resolvers/zod";
import { MoveToSpace, SpaceField, useSpaceChoice } from "@/app/space";
import { Check, Trash2 } from "lucide-react";
import { Controller, useForm } from "react-hook-form";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Dialog, DialogActions } from "@/components/ui/dialog";
import { Field, FieldError } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Money } from "@/components/ui/money";
import { MoneyInput } from "@/components/ui/money-input";
import { NativeSelect } from "@/components/ui/native-select";
import { useAccounts } from "@/features/accounts/api";
import type { AgendaItem } from "@/features/agenda/api";
import { useCreateBill, useDeleteBill, usePayBill, useSetBillActual, useUpdateBill, type Bill } from "@/features/bills/api";
import { usePayInvoice } from "@/features/cards/api";
import { CategorySelect } from "@/features/categories/CategorySelect";
import { useCorrectFinancing, useCreateFinancing, useDeleteFinancing, usePayFinancing, useUpdateFinancing, type Financing } from "@/features/financings/api";
import { formatDate, todayISO } from "@/lib/format";

const monthField = z.string().regex(/^\d{4}-\d{2}$/, "Informe o mês.");
const optionalMonth = z.string().regex(/^(\d{4}-\d{2})?$/, "Informe o mês.");
const dayField = z.number({ message: "Informe o dia." }).int().min(1, "De 1 a 31.").max(31, "De 1 a 31.");

/** Active accounts as options of a native select. */
function AccountOptions({ keepId }: { keepId?: number }) {
  const accounts = useAccounts();
  return (
    <>
      <option value={0}>Escolha</option>
      {(accounts.data ?? [])
        .filter((a) => a.active || a.id === keepId)
        .map((a) => (
          <option key={a.id} value={a.id}>
            {a.name}
          </option>
        ))}
    </>
  );
}

const billSchema = z.object({
  name: z.string().trim().min(1, "Informe o nome.").max(60, "Use no máximo 60 caracteres."),
  categoryId: z.number().int().positive("Escolha a categoria."),
  accountId: z.number().int().positive("Escolha a conta de pagamento."),
  dueDay: dayField,
  expectedCents: z.number().int().positive("Informe o valor previsto."),
  startMonth: monthField,
  endMonth: optionalMonth,
});

/** Creates or edits a fixed bill (RF-35). */
export function BillFormDialog({ open, bill, onClose }: { open: boolean; bill?: Bill; onClose: () => void }) {
  const create = useCreateBill();
  const update = useUpdateBill();
  const remove = useDeleteBill();
  const mutation = bill ? update : create;
  const where = useSpaceChoice();
  const form = useForm<z.infer<typeof billSchema>>({
    resolver: zodResolver(billSchema),
    resetOptions: { keepDirtyValues: true },
    values: {
      name: bill?.name ?? "",
      categoryId: bill?.categoryId ?? 0,
      accountId: bill?.accountId ?? 0,
      dueDay: bill?.dueDay ?? 10,
      expectedCents: bill?.expectedCents ?? 0,
      startMonth: bill?.startMonth ?? todayISO().slice(0, 7),
      endMonth: bill?.endMonth ?? "",
    },
  });
  const errors = form.formState.errors;

  /** Clears the mutation state and closes. */
  const close = () => {
    create.reset();
    update.reset();
    remove.reset();
    form.reset();
    onClose();
  };

  /** Validates and sends the form. */
  const submit = form.handleSubmit((v) => {
    const body = { ...v, endMonth: v.endMonth || undefined };
    if (bill) update.mutate({ id: bill.id, body }, { onSuccess: close });
    else create.mutate({ ...body, space: where.target }, { onSuccess: close });
  });

  return (
    <Dialog open={open} onOpenChange={(o) => !o && close()} title={bill ? "Editar conta fixa" : "Nova conta fixa"} description="Conta paga direto da conta bancária, como energia, água e aluguel.">
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <Field label="Nome" htmlFor="bill-name" error={errors.name?.message}>
          <Input id="bill-name" autoComplete="off" {...form.register("name")} />
        </Field>
        <Field label="Categoria" htmlFor="bill-category" error={errors.categoryId?.message}>
          <CategorySelect id="bill-category" {...form.register("categoryId", { valueAsNumber: true })} />
        </Field>
        <div className="grid grid-cols-3 gap-3">
          <div className="col-span-2">
            <Field label="Conta de pagamento" htmlFor="bill-account" error={errors.accountId?.message}>
              <NativeSelect id="bill-account" {...form.register("accountId", { valueAsNumber: true })}>
                <AccountOptions keepId={bill?.accountId} />
              </NativeSelect>
            </Field>
          </div>
          <Field label="Vence dia" htmlFor="bill-day" error={errors.dueDay?.message}>
            <Input id="bill-day" type="number" inputMode="numeric" min={1} max={31} {...form.register("dueDay", { valueAsNumber: true })} />
          </Field>
        </div>
        <Field label="Valor previsto" htmlFor="bill-expected" error={errors.expectedCents?.message}>
          <Controller control={form.control} name="expectedCents" render={({ field }) => <MoneyInput id="bill-expected" value={field.value} onChange={field.onChange} invalid={!!errors.expectedCents} />} />
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Primeiro mês" htmlFor="bill-start" error={errors.startMonth?.message}>
            <Input id="bill-start" type="month" {...form.register("startMonth")} />
          </Field>
          <Field label="Último mês (opcional)" htmlFor="bill-end" error={errors.endMonth?.message}>
            <Input id="bill-end" type="month" {...form.register("endMonth")} />
          </Field>
        </div>
        {bill ? <MoveToSpace kind="bill" itemId={bill.id} onMoved={close} /> : <SpaceField value={where.space} onChange={where.setSpace} />}
        <FieldError message={mutation.error?.message ?? remove.error?.message} />
        <DialogActions className="sm:justify-between">
          {bill ? (
            <Button variant="ghost" className="text-destructive" disabled={remove.isPending} onClick={() => remove.mutate(bill.id, { onSuccess: close })}>
              <Trash2 />
              Excluir
            </Button>
          ) : (
            <span />
          )}
          <span className="flex flex-col-reverse gap-2 sm:flex-row">
            <Button variant="ghost" onClick={close}>
              Cancelar
            </Button>
            <Button type="submit" disabled={mutation.isPending}>
              Salvar
            </Button>
          </span>
        </DialogActions>
      </form>
    </Dialog>
  );
}

/** Informs the real amount of a bill in the month; empty goes back to the expected one (RF-36, RN-14). */
export function ActualDialog({ item, month, onClose }: { item: AgendaItem | null; month: string; onClose: () => void }) {
  const save = useSetBillActual();
  const form = useForm<{ actualCents: number }>({ resetOptions: { keepDirtyValues: true }, values: { actualCents: item?.actualCents ?? 0 } });

  /** Clears the mutation state and closes. */
  const close = () => {
    form.reset();
    save.reset();
    onClose();
  };

  /** Sends the real amount, zero clearing it. */
  const submit = form.handleSubmit((v) => item && save.mutate({ id: item.refId, month, actualCents: v.actualCents }, { onSuccess: close }));

  return (
    <Dialog open={item !== null} onOpenChange={(o) => !o && close()} title={`Valor real de ${item?.name ?? ""}`} description="Até você informar, vale o previsto. Deixe zerado para voltar ao previsto.">
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        {item?.expectedCents !== undefined && (
          <p className="text-sm">
            Previsto: <Money cents={item.expectedCents} className="font-medium" />
          </p>
        )}
        <Field label="Valor real" htmlFor="bill-actual">
          <Controller control={form.control} name="actualCents" render={({ field }) => <MoneyInput id="bill-actual" value={field.value} onChange={field.onChange} autoFocus />} />
        </Field>
        <FieldError message={save.error?.message} />
        <DialogActions>
          <Button variant="ghost" onClick={close}>
            Cancelar
          </Button>
          <Button type="submit" disabled={save.isPending}>
            Salvar
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}

const paySchema = z.object({
  amountCents: z.number().int().positive("Informe o valor pago."),
  accountId: z.number().int().positive("Escolha a conta."),
  date: z.string().regex(/^\d{4}-\d{2}-\d{2}$/, "Informe a data."),
  balanceAfterCents: z.number().int().min(0),
});

/** Pays an agenda item: a bill, a financing installment with its new balance (RN-16) or a card invoice (RF-21). */
export function AgendaPayDialog({ item, month, onClose }: { item: AgendaItem | null; month: string; onClose: () => void }) {
  const payBill = usePayBill();
  const payFinancing = usePayFinancing();
  const payInvoice = usePayInvoice();
  const mutation = item?.kind === "financing" ? payFinancing : item?.kind === "invoice" ? payInvoice : payBill;
  const form = useForm<z.infer<typeof paySchema>>({
    resolver: zodResolver(paySchema),
    resetOptions: { keepDirtyValues: true },
    values: {
      amountCents: item?.amountCents ?? 0,
      accountId: item?.accountId ?? 0,
      date: todayISO(),
      balanceAfterCents: item?.suggestedBalanceCents ?? 0,
    },
  });
  const errors = form.formState.errors;

  /** Clears the mutation state and closes. */
  const close = () => {
    form.reset();
    payBill.reset();
    payFinancing.reset();
    payInvoice.reset();
    onClose();
  };

  /** Sends the payment to the service of the item kind. */
  const submit = form.handleSubmit((v) => {
    if (!item) return;
    const done = { onSuccess: close };
    if (item.kind === "bill") payBill.mutate({ id: item.refId, month, body: { amountCents: v.amountCents, accountId: v.accountId, date: v.date } }, done);
    else if (item.kind === "financing") payFinancing.mutate({ id: item.refId, month, body: v }, done);
    else payInvoice.mutate({ cardId: item.refId, month, body: { accountId: v.accountId, date: v.date } }, done);
  });

  const title = item?.kind === "financing" ? `Pagar parcela ${item.number} de ${item.total}` : item?.kind === "invoice" ? `Pagar fatura ${item.name}` : `Pagar ${item?.name ?? ""}`;
  return (
    <Dialog open={item !== null} onOpenChange={(o) => !o && close()} title={title} description={item ? `Vence em ${formatDate(item.dueOn)}.` : undefined}>
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        {item?.kind === "invoice" ? (
          <div className="rounded-xl bg-muted p-3">
            <p className="text-sm text-muted-foreground">Valor total da fatura</p>
            <Money cents={item.amountCents} className="text-2xl font-semibold" />
          </div>
        ) : (
          <Field label="Valor pago" htmlFor="pay-amount" error={errors.amountCents?.message}>
            <Controller control={form.control} name="amountCents" render={({ field }) => <MoneyInput id="pay-amount" value={field.value} onChange={field.onChange} />} />
          </Field>
        )}
        {item?.kind === "financing" && (
          <Field label="Saldo devedor depois do pagamento" htmlFor="pay-balance" error={errors.balanceAfterCents?.message}>
            <Controller control={form.control} name="balanceAfterCents" render={({ field }) => <MoneyInput id="pay-balance" value={field.value} onChange={field.onChange} />} />
            <span className="text-xs text-muted-foreground">Sugerido: saldo atual menos a parcela. Corrija com o valor que o banco mostra.</span>
          </Field>
        )}
        <div className="grid grid-cols-2 gap-3">
          <Field label="Sai da conta" htmlFor="pay-account" error={errors.accountId?.message}>
            <NativeSelect id="pay-account" {...form.register("accountId", { valueAsNumber: true })}>
              <AccountOptions />
            </NativeSelect>
          </Field>
          <Field label="Data" htmlFor="pay-date" error={errors.date?.message}>
            <Input id="pay-date" type="date" {...form.register("date")} />
          </Field>
        </div>
        <FieldError message={mutation.error?.message} />
        <DialogActions>
          <Button variant="ghost" onClick={close}>
            Cancelar
          </Button>
          <Button type="submit" disabled={mutation.isPending}>
            <Check />
            Pagar
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}

const financingSchema = z.object({
  name: z.string().trim().min(1, "Informe o nome.").max(60, "Use no máximo 60 caracteres."),
  categoryId: z.number().int().positive("Escolha a categoria."),
  accountId: z.number().int().positive("Escolha a conta."),
  dueDay: dayField,
  installmentCents: z.number().int().positive("Informe a parcela."),
  totalInstallments: z.number({ message: "Informe o total." }).int().min(1, "De 1 a 600.").max(600, "De 1 a 600."),
  paidInstallments: z.number({ message: "Informe as pagas." }).int().min(0),
  balanceCents: z.number().int().min(0),
  nextMonth: monthField,
});

/** Registers a financing with its state today, or edits its descriptive fields (RF-39). */
export function FinancingFormDialog({ open, financing, onClose }: { open: boolean; financing?: Financing; onClose: () => void }) {
  const create = useCreateFinancing();
  const update = useUpdateFinancing();
  const remove = useDeleteFinancing();
  const mutation = financing ? update : create;
  const where = useSpaceChoice();
  const form = useForm<z.infer<typeof financingSchema>>({
    resolver: zodResolver(financingSchema),
    resetOptions: { keepDirtyValues: true },
    values: {
      name: financing?.name ?? "",
      categoryId: financing?.categoryId ?? 0,
      accountId: financing?.accountId ?? 0,
      dueDay: financing?.dueDay ?? 10,
      installmentCents: financing?.installmentCents ?? 0,
      totalInstallments: financing?.totalInstallments ?? 1,
      paidInstallments: financing?.paidInstallments ?? 0,
      balanceCents: financing?.balanceCents ?? 0,
      nextMonth: financing?.nextMonth ?? todayISO().slice(0, 7),
    },
  });
  const errors = form.formState.errors;

  /** Clears the mutation state and closes. */
  const close = () => {
    create.reset();
    update.reset();
    remove.reset();
    form.reset();
    onClose();
  };

  /** Validates and sends the form; editing keeps the amounts, which change through a correction. */
  const submit = form.handleSubmit((v) => {
    if (financing) update.mutate({ id: financing.id, body: { name: v.name, categoryId: v.categoryId, accountId: v.accountId, dueDay: v.dueDay } }, { onSuccess: close });
    else create.mutate({ ...v, space: where.target }, { onSuccess: close });
  });

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => !o && close()}
      title={financing ? "Editar financiamento" : "Novo financiamento"}
      description={financing ? "Para mudar saldo ou parcela, use Corrigir saldo." : "Informe a situação de hoje, como o banco mostra."}
    >
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <Field label="Nome" htmlFor="fin-name" error={errors.name?.message}>
          <Input id="fin-name" autoComplete="off" placeholder="Casa, lote, carro..." {...form.register("name")} />
        </Field>
        <Field label="Categoria" htmlFor="fin-category" error={errors.categoryId?.message}>
          <CategorySelect id="fin-category" {...form.register("categoryId", { valueAsNumber: true })} />
        </Field>
        <div className="grid grid-cols-3 gap-3">
          <div className="col-span-2">
            <Field label="Conta de pagamento" htmlFor="fin-account" error={errors.accountId?.message}>
              <NativeSelect id="fin-account" {...form.register("accountId", { valueAsNumber: true })}>
                <AccountOptions keepId={financing?.accountId} />
              </NativeSelect>
            </Field>
          </div>
          <Field label="Vence dia" htmlFor="fin-day" error={errors.dueDay?.message}>
            <Input id="fin-day" type="number" inputMode="numeric" min={1} max={31} {...form.register("dueDay", { valueAsNumber: true })} />
          </Field>
        </div>
        {!financing && (
          <>
            <div className="grid grid-cols-2 gap-3">
              <Field label="Valor da parcela" htmlFor="fin-installment" error={errors.installmentCents?.message}>
                <Controller control={form.control} name="installmentCents" render={({ field }) => <MoneyInput id="fin-installment" value={field.value} onChange={field.onChange} />} />
              </Field>
              <Field label="Saldo devedor hoje" htmlFor="fin-balance" error={errors.balanceCents?.message}>
                <Controller control={form.control} name="balanceCents" render={({ field }) => <MoneyInput id="fin-balance" value={field.value} onChange={field.onChange} />} />
              </Field>
            </div>
            <div className="grid grid-cols-3 gap-3">
              <Field label="Total de parcelas" htmlFor="fin-total" error={errors.totalInstallments?.message}>
                <Input id="fin-total" type="number" inputMode="numeric" min={1} {...form.register("totalInstallments", { valueAsNumber: true })} />
              </Field>
              <Field label="Já pagas" htmlFor="fin-paid" error={errors.paidInstallments?.message}>
                <Input id="fin-paid" type="number" inputMode="numeric" min={0} {...form.register("paidInstallments", { valueAsNumber: true })} />
              </Field>
              <Field label="Próxima em" htmlFor="fin-next" error={errors.nextMonth?.message}>
                <Input id="fin-next" type="month" {...form.register("nextMonth")} />
              </Field>
            </div>
          </>
        )}
        {financing ? <MoveToSpace kind="financing" itemId={financing.id} onMoved={close} /> : <SpaceField value={where.space} onChange={where.setSpace} />}
        <FieldError message={mutation.error?.message ?? remove.error?.message} />
        <DialogActions className="sm:justify-between">
          {financing ? (
            <Button variant="ghost" className="text-destructive" disabled={remove.isPending} onClick={() => remove.mutate(financing.id, { onSuccess: close })}>
              <Trash2 />
              Excluir
            </Button>
          ) : (
            <span />
          )}
          <span className="flex flex-col-reverse gap-2 sm:flex-row">
            <Button variant="ghost" onClick={close}>
              Cancelar
            </Button>
            <Button type="submit" disabled={mutation.isPending}>
              Salvar
            </Button>
          </span>
        </DialogActions>
      </form>
    </Dialog>
  );
}

const correctionSchema = z.object({
  balanceCents: z.number().int().min(0),
  installmentCents: z.number().int().positive("Informe a parcela."),
});

/** Sets the balance and the installment by hand, for interest, inflation or prepayment (RF-42). */
export function CorrectionDialog({ financing, onClose }: { financing: Financing | null; onClose: () => void }) {
  const correct = useCorrectFinancing();
  const form = useForm<z.infer<typeof correctionSchema>>({
    resolver: zodResolver(correctionSchema),
    resetOptions: { keepDirtyValues: true },
    values: { balanceCents: financing?.balanceCents ?? 0, installmentCents: financing?.installmentCents ?? 0 },
  });
  const errors = form.formState.errors;

  /** Clears the mutation state and closes. */
  const close = () => {
    form.reset();
    correct.reset();
    onClose();
  };

  /** Validates and sends the correction. */
  const submit = form.handleSubmit((v) => financing && correct.mutate({ id: financing.id, body: v }, { onSuccess: close }));

  return (
    <Dialog open={financing !== null} onOpenChange={(o) => !o && close()} title={`Corrigir ${financing?.name ?? ""}`} description="Use os valores que o banco mostra hoje.">
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <Field label="Saldo devedor" htmlFor="fix-balance" error={errors.balanceCents?.message}>
          <Controller control={form.control} name="balanceCents" render={({ field }) => <MoneyInput id="fix-balance" value={field.value} onChange={field.onChange} />} />
        </Field>
        <Field label="Valor da parcela" htmlFor="fix-installment" error={errors.installmentCents?.message}>
          <Controller control={form.control} name="installmentCents" render={({ field }) => <MoneyInput id="fix-installment" value={field.value} onChange={field.onChange} />} />
        </Field>
        <FieldError message={correct.error?.message} />
        <DialogActions>
          <Button variant="ghost" onClick={close}>
            Cancelar
          </Button>
          <Button type="submit" disabled={correct.isPending}>
            Salvar
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}
