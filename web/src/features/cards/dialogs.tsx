import { zodResolver } from "@hookform/resolvers/zod";
import { motion } from "motion/react";
import { Check, Minus, Plus } from "lucide-react";
import { Controller, useForm, useWatch } from "react-hook-form";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Dialog, DialogActions } from "@/components/ui/dialog";
import { Field, FieldError } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Money } from "@/components/ui/money";
import { MoneyInput } from "@/components/ui/money-input";
import { NativeSelect } from "@/components/ui/native-select";
import { Segmented } from "@/components/ui/segmented";
import { Switch } from "@/components/ui/switch";
import { useAccounts, usePersons } from "@/features/accounts/api";
import {
  useAddCredit,
  useAddPurchase,
  useAddRecurrence,
  useCreateCard,
  usePayInvoice,
  useUpdateCard,
  useUpdatePurchase,
  useUpdateRecurrence,
  type Card,
  type CardColor,
  type CardFinish,
  type Invoice,
  type InvoiceItem,
  type Recurrence,
} from "@/features/cards/api";
import { CardVisual } from "@/features/cards/CardVisual";
import { CategorySelect } from "@/features/categories/CategorySelect";
import { haptic } from "@/lib/haptics";
import { formatDate, monthName, todayISO } from "@/lib/format";
import { spring } from "@/lib/motion";
import { cn } from "@/lib/utils";

/** Colors offered for the card drawing, with the swatch shown in the picker. */
const COLORS: { value: CardColor; label: string; swatch: string }[] = [
  { value: "graphite", label: "Grafite", swatch: "#26282c" },
  { value: "purple", label: "Roxo", swatch: "#7a2fd0" },
  { value: "blue", label: "Azul", swatch: "#2a5ef0" },
  { value: "green", label: "Verde", swatch: "#1c8c56" },
  { value: "red", label: "Vermelho", swatch: "#d23a40" },
  { value: "orange", label: "Laranja", swatch: "#ff7a1a" },
  { value: "gold", label: "Dourado", swatch: "#d9b456" },
  { value: "silver", label: "Prata", swatch: "#c9ccd2" },
];

const dateField = z.string().regex(/^\d{4}-\d{2}-\d{2}$/, "Informe a data.");
const descriptionField = z.string().trim().max(120, "Use no máximo 120 caracteres.");

const cardSchema = z.object({
  name: z.string().trim().min(1, "Informe o nome do cartão.").max(40, "Use no máximo 40 caracteres."),
  personId: z.number().int().positive("Escolha o titular."),
  limitCents: z.number().int().min(0),
  closingDay: z.number({ message: "Informe o dia." }).int().min(1, "De 1 a 31.").max(31, "De 1 a 31."),
  dueDay: z.number({ message: "Informe o dia." }).int().min(1, "De 1 a 31.").max(31, "De 1 a 31."),
  lastDigits: z.string().regex(/^(\d{4})?$/, "Informe os 4 últimos dígitos."),
  color: z.enum(["graphite", "purple", "blue", "green", "red", "orange", "gold", "silver"]),
  finish: z.enum(["metal", "matte"]),
});
type CardValues = z.infer<typeof cardSchema>;

/** Creates or edits a card with a live preview of its drawing (RF-14, section 5.4). */
export function CardFormDialog({ open, card, onClose }: { open: boolean; card?: Card; onClose: () => void }) {
  const persons = usePersons();
  const create = useCreateCard();
  const update = useUpdateCard();
  const mutation = card ? update : create;
  const form = useForm<CardValues>({
    resolver: zodResolver(cardSchema),
    values: {
      name: card?.name ?? "",
      personId: card?.personId ?? 0,
      limitCents: card?.limitCents ?? 0,
      closingDay: card?.closingDay ?? 1,
      dueDay: card?.dueDay ?? 8,
      lastDigits: card?.lastDigits ?? "",
      color: card?.color ?? "graphite",
      finish: card?.finish ?? "metal",
    },
  });
  const v = useWatch({ control: form.control });
  const errors = form.formState.errors;
  const personName = persons.data?.find((p) => p.id === v.personId)?.name ?? "Titular";

  /** Clears the mutation state and closes. */
  const close = () => {
    mutation.reset();
    form.reset();
    onClose();
  };

  /** Validates and sends the form. */
  const submit = form.handleSubmit((values) => {
    const body = { ...values, lastDigits: values.lastDigits || undefined };
    if (card) update.mutate({ id: card.id, body: { ...body, active: card.active } }, { onSuccess: close });
    else create.mutate(body, { onSuccess: close });
  });

  return (
    <Dialog open={open} onOpenChange={(o) => !o && close()} title={card ? "Editar cartão" : "Novo cartão"}>
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <CardVisual
          name={v.name || "Cartão"}
          personName={personName}
          lastDigits={v.lastDigits || undefined}
          color={v.color ?? "graphite"}
          finish={v.finish ?? "metal"}
          dueLabel={`Dia ${v.dueDay || ""}`}
          limitFreeCents={v.limitCents ?? 0}
          className="mx-auto max-w-xs"
        />
        <div className="grid grid-cols-2 gap-3">
          <Field label="Nome" htmlFor="card-name" error={errors.name?.message}>
            <Input id="card-name" autoComplete="off" placeholder="Nubank, Inter..." {...form.register("name")} />
          </Field>
          <Field label="Titular" htmlFor="card-person" error={errors.personId?.message}>
            <NativeSelect id="card-person" {...form.register("personId", { valueAsNumber: true })}>
              <option value={0}>Escolha</option>
              {(persons.data ?? []).map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </NativeSelect>
          </Field>
        </div>
        <Field label="Limite total" htmlFor="card-limit">
          <Controller control={form.control} name="limitCents" render={({ field }) => <MoneyInput id="card-limit" value={field.value} onChange={field.onChange} />} />
        </Field>
        <div className="grid grid-cols-3 gap-3">
          <Field label="Fecha dia" htmlFor="card-closing" error={errors.closingDay?.message}>
            <Input id="card-closing" type="number" inputMode="numeric" min={1} max={31} {...form.register("closingDay", { valueAsNumber: true })} />
          </Field>
          <Field label="Vence dia" htmlFor="card-due" error={errors.dueDay?.message}>
            <Input id="card-due" type="number" inputMode="numeric" min={1} max={31} {...form.register("dueDay", { valueAsNumber: true })} />
          </Field>
          <Field label="Final" htmlFor="card-digits" error={errors.lastDigits?.message}>
            <Input id="card-digits" inputMode="numeric" maxLength={4} placeholder="1234" {...form.register("lastDigits")} />
          </Field>
        </div>
        <div className="flex flex-col gap-1.5">
          <span className="text-sm font-medium">Cor</span>
          <div role="radiogroup" aria-label="Cor" className="flex flex-wrap gap-2">
            {COLORS.map((c) => (
              <button
                key={c.value}
                type="button"
                role="radio"
                aria-checked={v.color === c.value}
                aria-label={c.label}
                onClick={() => {
                  haptic();
                  form.setValue("color", c.value);
                }}
                className="relative grid size-9 place-items-center rounded-full border"
                style={{ background: c.swatch }}
              >
                {v.color === c.value && (
                  <motion.span layoutId="card-color" transition={spring} className="absolute -inset-1 rounded-full border-2 border-primary" />
                )}
              </button>
            ))}
          </div>
        </div>
        <Segmented<CardFinish>
          ariaLabel="Acabamento"
          value={v.finish ?? "metal"}
          options={[
            { value: "metal", label: "Metálico" },
            { value: "matte", label: "Fosco" },
          ]}
          onChange={(value) => form.setValue("finish", value)}
        />
        <FieldError message={mutation.error?.message} />
        <DialogActions>
          <Button variant="ghost" onClick={close}>
            Cancelar
          </Button>
          <Button type="submit" disabled={mutation.isPending}>
            {mutation.isPending ? "Salvando..." : "Salvar"}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}

const purchaseSchema = z.object({
  merchant: z.string().trim().min(1, "Informe o estabelecimento.").max(80, "Use no máximo 80 caracteres."),
  amountCents: z.number().int().positive("Informe um valor maior que zero."),
  installments: z.number().int().min(1).max(48),
  perInstallment: z.boolean(),
  categoryId: z.number().int().positive("Escolha a categoria."),
  date: dateField,
  buyerPersonId: z.number().int(),
  description: descriptionField,
});
type PurchaseValues = z.infer<typeof purchaseSchema>;

/** Registers or edits a purchase in one or more installments (RF-15, RF-16). */
export function PurchaseDialog({
  open,
  card,
  item,
  onClose,
}: {
  open: boolean;
  card: Card;
  /** Invoice line of the purchase being edited. */
  item?: InvoiceItem;
  onClose: () => void;
}) {
  const persons = usePersons();
  const add = useAddPurchase();
  const update = useUpdatePurchase();
  const mutation = item ? update : add;
  const form = useForm<PurchaseValues>({
    resolver: zodResolver(purchaseSchema),
    values: {
      merchant: item?.merchant ?? "",
      amountCents: item?.purchaseTotalCents ?? 0,
      installments: item?.count || 1,
      perInstallment: false,
      categoryId: item?.categoryId ?? 0,
      date: item?.date ?? todayISO(),
      buyerPersonId: item?.buyerPersonId ?? 0,
      description: item?.description ?? "",
    },
  });
  const installments = useWatch({ control: form.control, name: "installments" });
  const perInstallment = useWatch({ control: form.control, name: "perInstallment" });
  const errors = form.formState.errors;

  /** Clears the mutation state and closes. */
  const close = () => {
    mutation.reset();
    form.reset();
    onClose();
  };

  /** Changes the number of installments by delta, within 1 and 48. */
  const step = (delta: number) => {
    haptic();
    form.setValue("installments", Math.min(48, Math.max(1, installments + delta)));
  };

  /** Validates and sends the form. */
  const submit = form.handleSubmit((v) => {
    const body = { ...v, buyerPersonId: v.buyerPersonId || undefined };
    if (item?.purchaseId) update.mutate({ id: item.purchaseId, body }, { onSuccess: close });
    else add.mutate({ cardId: card.id, body }, { onSuccess: close });
  });

  return (
    <Dialog open={open} onOpenChange={(o) => !o && close()} title={item ? "Editar compra" : `Compra no ${card.name}`} description="A fatura é escolhida pela data e pelo dia de fechamento do cartão.">
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <Field label="Estabelecimento" htmlFor="purchase-merchant" error={errors.merchant?.message}>
          <Input id="purchase-merchant" autoComplete="off" {...form.register("merchant")} />
        </Field>
        <Field label={perInstallment ? "Valor da parcela" : "Valor total"} htmlFor="purchase-amount" error={errors.amountCents?.message}>
          <Controller control={form.control} name="amountCents" render={({ field }) => <MoneyInput id="purchase-amount" value={field.value} onChange={field.onChange} invalid={!!errors.amountCents} />} />
        </Field>
        <div className="flex items-center justify-between gap-3">
          <span className="text-sm font-medium" id="purchase-installments-label">
            Parcelas
          </span>
          <div className="flex items-center gap-2" role="group" aria-labelledby="purchase-installments-label">
            <Button variant="outline" size="icon-sm" aria-label="Menos parcelas" onClick={() => step(-1)} disabled={installments <= 1}>
              <Minus />
            </Button>
            <span className="w-10 text-center text-lg font-semibold tabular-nums" aria-live="polite">
              {installments}x
            </span>
            <Button variant="outline" size="icon-sm" aria-label="Mais parcelas" onClick={() => step(1)} disabled={installments >= 48}>
              <Plus />
            </Button>
          </div>
        </div>
        {installments > 1 && (
          <label className="flex items-center justify-between gap-3 text-sm">
            Informar o valor da parcela
            <Controller control={form.control} name="perInstallment" render={({ field }) => <Switch checked={field.value} onCheckedChange={field.onChange} />} />
          </label>
        )}
        <Field label="Categoria" htmlFor="purchase-category" error={errors.categoryId?.message}>
          <CategorySelect id="purchase-category" {...form.register("categoryId", { valueAsNumber: true })} />
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Data da compra" htmlFor="purchase-date" error={errors.date?.message}>
            <Input id="purchase-date" type="date" {...form.register("date")} />
          </Field>
          <Field label="Quem comprou" htmlFor="purchase-buyer">
            <NativeSelect id="purchase-buyer" {...form.register("buyerPersonId", { valueAsNumber: true })}>
              <option value={0}>Não informar</option>
              {(persons.data ?? []).map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </NativeSelect>
          </Field>
        </div>
        <Field label="Descrição" htmlFor="purchase-desc" error={errors.description?.message}>
          <Input id="purchase-desc" autoComplete="off" placeholder="Opcional" {...form.register("description")} />
        </Field>
        <FieldError message={mutation.error?.message} />
        <DialogActions>
          <Button variant="ghost" onClick={close}>
            Cancelar
          </Button>
          <Button type="submit" disabled={mutation.isPending}>
            <Check />
            {item ? "Salvar" : "Lançar"}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}

const creditSchema = z.object({
  merchant: z.string().trim().min(1, "Informe a origem do crédito.").max(80, "Use no máximo 80 caracteres."),
  amountCents: z.number().int().positive("Informe um valor maior que zero."),
  categoryId: z.number().int(),
  date: dateField,
  description: descriptionField,
});

/** Registers a refund or another credit on the invoice of its date (RF-22). */
export function CreditDialog({ open, card, onClose }: { open: boolean; card: Card; onClose: () => void }) {
  const add = useAddCredit();
  const form = useForm<z.infer<typeof creditSchema>>({
    resolver: zodResolver(creditSchema),
    values: { merchant: "", amountCents: 0, categoryId: 0, date: todayISO(), description: "" },
  });
  const errors = form.formState.errors;

  /** Clears the mutation state and closes. */
  const close = () => {
    add.reset();
    form.reset();
    onClose();
  };

  /** Validates and sends the form. */
  const submit = form.handleSubmit((v) => add.mutate({ cardId: card.id, body: { ...v, categoryId: v.categoryId || undefined } }, { onSuccess: close }));

  return (
    <Dialog open={open} onOpenChange={(o) => !o && close()} title="Estorno ou crédito" description="Entra negativo na fatura da data. Com categoria, abate o gasto dela.">
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <Field label="Origem" htmlFor="credit-merchant" error={errors.merchant?.message}>
          <Input id="credit-merchant" autoComplete="off" placeholder="Estorno da loja, crédito de anuidade..." {...form.register("merchant")} />
        </Field>
        <Field label="Valor" htmlFor="credit-amount" error={errors.amountCents?.message}>
          <Controller control={form.control} name="amountCents" render={({ field }) => <MoneyInput id="credit-amount" value={field.value} onChange={field.onChange} invalid={!!errors.amountCents} />} />
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Data" htmlFor="credit-date" error={errors.date?.message}>
            <Input id="credit-date" type="date" {...form.register("date")} />
          </Field>
          <Field label="Categoria" htmlFor="credit-category">
            <CategorySelect id="credit-category" placeholder="Sem categoria" {...form.register("categoryId", { valueAsNumber: true })} />
          </Field>
        </div>
        <FieldError message={add.error?.message} />
        <DialogActions>
          <Button variant="ghost" onClick={close}>
            Cancelar
          </Button>
          <Button type="submit" disabled={add.isPending}>
            Lançar
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}

const recurrenceSchema = z.object({
  description: z.string().trim().min(1, "Informe a descrição.").max(80, "Use no máximo 80 caracteres."),
  amountCents: z.number().int().positive("Informe um valor maior que zero."),
  categoryId: z.number().int().positive("Escolha a categoria."),
  startDate: z.string(),
  endMonth: z.string().regex(/^(\d{4}-\d{2})?$/, "Informe o mês."),
});

/** Registers or edits a charge repeated on every invoice, like a subscription (RF-17). */
export function RecurrenceDialog({ open, card, recurrence, onClose }: { open: boolean; card: Card; recurrence?: Recurrence; onClose: () => void }) {
  const add = useAddRecurrence();
  const update = useUpdateRecurrence();
  const mutation = recurrence ? update : add;
  const form = useForm<z.infer<typeof recurrenceSchema>>({
    resolver: zodResolver(recurrenceSchema),
    values: {
      description: recurrence?.description ?? "",
      amountCents: recurrence?.amountCents ?? 0,
      categoryId: recurrence?.categoryId ?? 0,
      startDate: todayISO(),
      endMonth: recurrence?.endMonth ?? "",
    },
  });
  const errors = form.formState.errors;

  /** Clears the mutation state and closes. */
  const close = () => {
    mutation.reset();
    form.reset();
    onClose();
  };

  /** Validates and sends the form. */
  const submit = form.handleSubmit((v) => {
    const endMonth = v.endMonth || undefined;
    if (recurrence) update.mutate({ id: recurrence.id, body: { description: v.description, amountCents: v.amountCents, categoryId: v.categoryId, endMonth } }, { onSuccess: close });
    else add.mutate({ cardId: card.id, body: { ...v, endMonth } }, { onSuccess: close });
  });

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => !o && close()}
      title={recurrence ? "Editar recorrência" : `Recorrência no ${card.name}`}
      description={recurrence ? "Um valor novo vale da fatura aberta em diante. Faturas fechadas não mudam." : "Lançada sozinha em toda fatura, como streaming, barbearia e internet."}
    >
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <Field label="Descrição" htmlFor="recurrence-desc" error={errors.description?.message}>
          <Input id="recurrence-desc" autoComplete="off" {...form.register("description")} />
        </Field>
        <Field label="Valor" htmlFor="recurrence-amount" error={errors.amountCents?.message}>
          <Controller control={form.control} name="amountCents" render={({ field }) => <MoneyInput id="recurrence-amount" value={field.value} onChange={field.onChange} invalid={!!errors.amountCents} />} />
        </Field>
        <Field label="Categoria" htmlFor="recurrence-category" error={errors.categoryId?.message}>
          <CategorySelect id="recurrence-category" {...form.register("categoryId", { valueAsNumber: true })} />
        </Field>
        <div className="grid grid-cols-2 gap-3">
          {!recurrence && (
            <Field label="Primeira cobrança" htmlFor="recurrence-start">
              <Input id="recurrence-start" type="date" {...form.register("startDate")} />
            </Field>
          )}
          <Field label="Última fatura (opcional)" htmlFor="recurrence-end" error={errors.endMonth?.message}>
            <Input id="recurrence-end" type="month" {...form.register("endMonth")} />
          </Field>
        </div>
        <FieldError message={mutation.error?.message} />
        <DialogActions>
          <Button variant="ghost" onClick={close}>
            Cancelar
          </Button>
          <Button type="submit" disabled={mutation.isPending}>
            Salvar
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}

const paySchema = z.object({ accountId: z.number().int().positive("Escolha a conta."), date: dateField });

/** Pays the whole invoice from an account; the payment is not spending (RF-21, RN-07). */
export function PayDialog({ open, card, invoice, onClose }: { open: boolean; card: Card; invoice: Invoice; onClose: () => void }) {
  const accounts = useAccounts();
  const pay = usePayInvoice();
  const form = useForm<z.infer<typeof paySchema>>({ resolver: zodResolver(paySchema), values: { accountId: 0, date: todayISO() } });
  const errors = form.formState.errors;
  const [y, m] = invoice.month.split("-").map(Number);

  /** Clears the mutation state and closes. */
  const close = () => {
    pay.reset();
    onClose();
  };

  /** Validates and sends the form. */
  const submit = form.handleSubmit((v) => pay.mutate({ cardId: card.id, month: invoice.month, body: v }, { onSuccess: close }));

  return (
    <Dialog open={open} onOpenChange={(o) => !o && close()} title={`Pagar fatura de ${monthName(y!, m!).toLowerCase()}`} description={`${card.name}, vence em ${formatDate(invoice.dueOn)}.`}>
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <div className="rounded-xl bg-muted p-3">
          <p className="text-sm text-muted-foreground">Valor total da fatura</p>
          <Money cents={invoice.totalCents} className="text-2xl font-semibold" />
        </div>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Sai da conta" htmlFor="pay-account" error={errors.accountId?.message}>
            <NativeSelect id="pay-account" {...form.register("accountId", { valueAsNumber: true })}>
              <option value={0}>Escolha</option>
              {(accounts.data ?? []).filter((a) => a.active).map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </NativeSelect>
          </Field>
          <Field label="Data" htmlFor="pay-date" error={errors.date?.message}>
            <Input id="pay-date" type="date" {...form.register("date")} />
          </Field>
        </div>
        <p className="text-xs text-muted-foreground">O pagamento só reduz o saldo da conta. As compras já foram contadas como gasto.</p>
        <FieldError message={pay.error?.message} />
        <DialogActions>
          <Button variant="ghost" onClick={close}>
            Cancelar
          </Button>
          <Button type="submit" disabled={pay.isPending} className={cn(pay.isPending && "opacity-70")}>
            <Check />
            Pagar
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}
