import { zodResolver } from "@hookform/resolvers/zod";
import { AnimatePresence, motion } from "motion/react";
import { CalendarClock, Check, ChevronLeft, CircleDashed, Plus, Repeat, Sparkles, Trash2, Undo2 } from "lucide-react";
import { useState } from "react";
import { Controller, useForm, useWatch } from "react-hook-form";
import { Link } from "react-router";
import { z } from "zod";
import { ApiRequestError } from "@/api/client";
import { Button } from "@/components/ui/button";
import { Card, CardDescription, CardTitle } from "@/components/ui/card";
import { Dialog, DialogActions } from "@/components/ui/dialog";
import { Field, FieldError } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Money } from "@/components/ui/money";
import { MoneyInput } from "@/components/ui/money-input";
import { NativeSelect } from "@/components/ui/native-select";
import { Segmented } from "@/components/ui/segmented";
import { useAccounts, useDeleteEntry, usePersons } from "@/features/accounts/api";
import {
  useAddOneOffIncome,
  useConfirmIncome,
  useCreateIncome,
  useDeleteIncome,
  useIncomeMonth,
  useIncomes,
  useUpdateIncome,
  type Income,
  type IncomeOccurrence,
} from "@/features/incomes/api";
import { formatDate, formatDayMonth, monthName, monthParam, todayISO } from "@/lib/format";
import { fade, spring } from "@/lib/motion";
import { useMonth } from "@/lib/month";
import { cn } from "@/lib/utils";

/** Incomes screen: the month with what is expected and received, and the registered incomes (RF-09 to RF-13). */
export function IncomesPage() {
  const { year, month } = useMonth();
  const param = monthParam(year, month);
  const view = useIncomeMonth(param);
  const incomes = useIncomes();
  const undo = useDeleteEntry();
  const [editing, setEditing] = useState<Income | "new" | null>(null);
  const [confirming, setConfirming] = useState<IncomeOccurrence | null>(null);
  const [oneOff, setOneOff] = useState(false);
  const v = view.data;

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between gap-3">
        <div className="flex flex-col gap-0.5">
          <Link to="/settings" className="flex items-center gap-0.5 text-sm text-primary">
            <ChevronLeft className="size-4" />
            Ajustes
          </Link>
          <h1 className="px-1 text-lg font-semibold">Rendas</h1>
        </div>
        <Button size="sm" onClick={() => setEditing("new")}>
          <Plus />
          Nova renda
        </Button>
      </div>

      <Card className="flex flex-col gap-4">
        <div className="flex items-start justify-between gap-3">
          <div>
            <CardTitle>
              {monthName(year, month)} de {year}
            </CardTitle>
            <CardDescription>A renda prevista só entra no saldo da conta quando você confirma o recebimento.</CardDescription>
          </div>
          <Button size="sm" variant="outline" onClick={() => setOneOff(true)}>
            <Sparkles />
            Renda avulsa
          </Button>
        </div>
        {v && (
          <dl className="grid grid-cols-2 gap-3 rounded-xl bg-muted p-3 text-sm">
            <div>
              <dt className="text-muted-foreground">Recebido</dt>
              <dd className="text-lg font-semibold text-success">
                <Money cents={v.receivedCents} />
              </dd>
            </div>
            <div>
              <dt className="text-muted-foreground">A receber</dt>
              <dd className="text-lg font-semibold">
                <Money cents={v.pendingCents} />
              </dd>
            </div>
          </dl>
        )}
        <ul className="flex flex-col">
          <AnimatePresence initial={false}>
            {(v?.occurrences ?? []).map((o) => (
              <motion.li
                key={o.incomeId}
                layout
                initial={{ opacity: 0, y: -6 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0 }}
                transition={{ ...spring, opacity: fade }}
                className="flex items-center gap-3 border-b py-2.5 last:border-b-0"
              >
                <StatusIcon confirmed={o.confirmed} />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm font-medium">{o.name}</span>
                  <span className="block truncate text-xs text-muted-foreground">
                    {o.confirmed && o.receivedOn
                      ? `Recebido em ${formatDayMonth(o.receivedOn)} · ${o.receivedIn ?? o.accountName}`
                      : `Previsto para ${formatDayMonth(o.expectedOn)} · ${o.personName} · ${o.accountName}`}
                  </span>
                </span>
                <Money cents={o.confirmed ? (o.receivedCents ?? o.expectedCents) : o.expectedCents} className={cn("text-sm font-medium", o.confirmed ? "text-success" : "text-muted-foreground")} />
                {o.confirmed ? (
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={`Desfazer recebimento de ${o.name}`}
                    disabled={undo.isPending}
                    onClick={() => o.entryId && undo.mutate(o.entryId)}
                  >
                    <Undo2 />
                  </Button>
                ) : (
                  <Button size="sm" onClick={() => setConfirming(o)} aria-label={`Confirmar ${o.name}`}>
                    <Check />
                    Recebi
                  </Button>
                )}
              </motion.li>
            ))}
            {(v?.oneOffs ?? []).map((x) => (
              <motion.li
                key={`one-off-${x.entryId}`}
                layout
                initial={{ opacity: 0, y: -6 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0 }}
                transition={{ ...spring, opacity: fade }}
                className="flex items-center gap-3 border-b py-2.5 last:border-b-0"
              >
                <span className="grid size-9 shrink-0 place-items-center rounded-xl bg-success/10">
                  <Sparkles className="size-4 text-success" />
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm font-medium">{x.description || "Renda avulsa"}</span>
                  <span className="block truncate text-xs text-muted-foreground">
                    Avulsa · {formatDayMonth(x.date)} · {x.accountName}
                  </span>
                </span>
                <Money cents={x.amountCents} className="text-sm font-medium text-success" />
                <Button variant="ghost" size="icon-sm" aria-label={`Excluir ${x.description || "renda avulsa"}`} disabled={undo.isPending} onClick={() => undo.mutate(x.entryId)}>
                  <Trash2 />
                </Button>
              </motion.li>
            ))}
          </AnimatePresence>
        </ul>
        {v && v.occurrences.length === 0 && v.oneOffs.length === 0 && (
          <p className="py-2 text-center text-sm text-muted-foreground">Nenhuma renda neste mês.</p>
        )}
      </Card>

      <Card className="flex flex-col gap-2">
        <CardTitle>Cadastradas</CardTitle>
        {incomes.data?.length === 0 && <p className="text-sm text-muted-foreground">Cadastre o salário, a bolsa e as rendas eventuais, como 13º e férias.</p>}
        <ul className="flex flex-col">
          {(incomes.data ?? []).map((i) => (
            <li key={i.id}>
              <button type="button" onClick={() => setEditing(i)} className="-mx-2 flex w-[calc(100%+1rem)] items-center gap-3 rounded-xl px-2 py-2.5 text-left hover:bg-accent active:bg-accent">
                <span className="grid size-9 shrink-0 place-items-center rounded-xl bg-primary/10">
                  {i.kind === "recurring" ? <Repeat className="size-4 text-primary" /> : <CalendarClock className="size-4 text-primary" />}
                </span>
                <span className="min-w-0 flex-1">
                  <span className="flex items-center gap-2">
                    <span className="truncate text-sm font-medium">{i.name}</span>
                    {i.endingSoon && <span className="shrink-0 rounded-full bg-warning/15 px-2 py-0.5 text-[0.7rem] font-medium text-warning">Termina em breve</span>}
                    {i.ended && <span className="shrink-0 rounded-full bg-muted px-2 py-0.5 text-[0.7rem] text-muted-foreground">Encerrada</span>}
                  </span>
                  <span className="block truncate text-xs text-muted-foreground">{incomeSummary(i)}</span>
                </span>
                <Money cents={i.amountCents} className="text-sm font-medium" />
              </button>
            </li>
          ))}
        </ul>
      </Card>

      <IncomeFormDialog state={editing} onClose={() => setEditing(null)} />
      <ConfirmDialog occurrence={confirming} month={param} onClose={() => setConfirming(null)} />
      <OneOffDialog open={oneOff} onClose={() => setOneOff(false)} />
    </div>
  );
}

/** One line describing when and where an income arrives. */
function incomeSummary(i: Income) {
  if (i.kind === "eventual") return `${i.personName} · prevista para ${i.expectedOn ? formatDate(i.expectedOn) : ""} · ${i.accountName}`;
  const end = i.endOn ? ` · até ${formatDate(i.endOn)}` : "";
  return `${i.personName} · todo dia ${i.day} · ${i.accountName}${end}`;
}

/** Check for received incomes and a dashed circle for expected ones. */
function StatusIcon({ confirmed }: { confirmed: boolean }) {
  return (
    <span className={cn("grid size-9 shrink-0 place-items-center rounded-xl", confirmed ? "bg-success/10" : "bg-muted")}>
      <AnimatePresence mode="wait" initial={false}>
        <motion.span key={confirmed ? "on" : "off"} initial={{ scale: 0.5, opacity: 0 }} animate={{ scale: 1, opacity: 1 }} exit={{ scale: 0.5, opacity: 0 }} transition={spring}>
          {confirmed ? <Check className="size-4 text-success" /> : <CircleDashed className="size-4 text-muted-foreground" />}
        </motion.span>
      </AnimatePresence>
    </span>
  );
}

const dateField = z.string().regex(/^\d{4}-\d{2}-\d{2}$/, "Informe a data.");

const incomeSchema = z
  .object({
    kind: z.enum(["recurring", "eventual"]),
    name: z.string().trim().min(1, "Informe o nome da renda.").max(60, "Use no máximo 60 caracteres."),
    personId: z.number().int().positive("Escolha de quem é a renda."),
    accountId: z.number().int().positive("Escolha a conta que recebe."),
    amountCents: z.number().int().positive("Informe o valor líquido."),
    day: z.number({ message: "Informe o dia." }).int(),
    startMonth: z.string(),
    endOn: z.string(),
    expectedOn: z.string(),
  })
  .superRefine((v, ctx) => {
    if (v.kind === "recurring") {
      if (!(v.day >= 1 && v.day <= 31)) ctx.addIssue({ code: "custom", path: ["day"], message: "Informe um dia entre 1 e 31." });
      if (!/^\d{4}-\d{2}$/.test(v.startMonth)) ctx.addIssue({ code: "custom", path: ["startMonth"], message: "Informe o mês do primeiro recebimento." });
    } else if (!dateField.safeParse(v.expectedOn).success) {
      ctx.addIssue({ code: "custom", path: ["expectedOn"], message: "Informe a data prevista." });
    }
  });
type IncomeValues = z.infer<typeof incomeSchema>;

/** Creates or edits an income; recurring ones ask the day, the first month and an optional end date. */
function IncomeFormDialog({ state, onClose }: { state: Income | "new" | null; onClose: () => void }) {
  const persons = usePersons();
  const accounts = useAccounts();
  const create = useCreateIncome();
  const update = useUpdateIncome();
  const remove = useDeleteIncome();
  const editing = state !== null && state !== "new" ? state : undefined;
  const mutation = editing ? update : create;
  const thisMonth = todayISO().slice(0, 7);
  const form = useForm<IncomeValues>({
    resolver: zodResolver(incomeSchema),
    resetOptions: { keepDirtyValues: true },
    values: {
      kind: editing?.kind ?? "recurring",
      name: editing?.name ?? "",
      personId: editing?.personId ?? 0,
      accountId: editing?.accountId ?? 0,
      amountCents: editing?.amountCents ?? 0,
      day: editing?.day ?? 5,
      startMonth: editing?.startMonth ?? thisMonth,
      endOn: editing?.endOn ?? "",
      expectedOn: editing?.expectedOn ?? "",
    },
  });
  const kind = useWatch({ control: form.control, name: "kind" });
  const errors = form.formState.errors;
  const inUse = remove.error instanceof ApiRequestError && remove.error.code === "income_in_use";

  /** Clears the mutation state and closes. */
  const close = () => {
    create.reset();
    update.reset();
    remove.reset();
    form.reset();
    onClose();
  };

  /** Validates and sends the form, keeping only the fields of the chosen kind. */
  const submit = form.handleSubmit((v) => {
    const body =
      v.kind === "recurring"
        ? { kind: v.kind, name: v.name, personId: v.personId, accountId: v.accountId, amountCents: v.amountCents, day: v.day, startMonth: v.startMonth, endOn: v.endOn || undefined }
        : { kind: v.kind, name: v.name, personId: v.personId, accountId: v.accountId, amountCents: v.amountCents, expectedOn: v.expectedOn };
    if (editing) update.mutate({ id: editing.id, body }, { onSuccess: close });
    else create.mutate(body, { onSuccess: close });
  });

  return (
    <Dialog open={state !== null} onOpenChange={(o) => !o && close()} title={editing ? "Editar renda" : "Nova renda"}>
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        {!editing && (
          <Segmented<IncomeValues["kind"]>
            ariaLabel="Tipo de renda"
            value={kind}
            options={[
              { value: "recurring", label: "Todo mês", icon: <Repeat className="size-4" /> },
              { value: "eventual", label: "Eventual", icon: <CalendarClock className="size-4" /> },
            ]}
            onChange={(value) => form.setValue("kind", value)}
          />
        )}
        <Field label="Nome" htmlFor="income-name" error={errors.name?.message}>
          <Input id="income-name" autoComplete="off" placeholder={kind === "recurring" ? "Salário, bolsa..." : "13º salário, férias..."} {...form.register("name")} />
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="De quem" htmlFor="income-person" error={errors.personId?.message}>
            <NativeSelect id="income-person" {...form.register("personId", { valueAsNumber: true })}>
              <option value={0}>Escolha</option>
              {(persons.data ?? []).map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </NativeSelect>
          </Field>
          <Field label="Conta que recebe" htmlFor="income-account" error={errors.accountId?.message}>
            <NativeSelect id="income-account" {...form.register("accountId", { valueAsNumber: true })}>
              <option value={0}>Escolha</option>
              {(accounts.data ?? []).filter((a) => a.active || a.id === editing?.accountId).map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </NativeSelect>
          </Field>
        </div>
        <Field label={kind === "recurring" ? "Valor líquido" : "Valor estimado"} htmlFor="income-amount" error={errors.amountCents?.message}>
          <Controller control={form.control} name="amountCents" render={({ field }) => <MoneyInput id="income-amount" value={field.value} onChange={field.onChange} invalid={!!errors.amountCents} />} />
        </Field>
        {kind === "recurring" ? (
          <>
            <div className="grid grid-cols-2 gap-3">
              <Field label="Dia do recebimento" htmlFor="income-day" error={errors.day?.message}>
                <Input id="income-day" type="number" inputMode="numeric" min={1} max={31} {...form.register("day", { valueAsNumber: true })} />
              </Field>
              <Field label="Primeiro mês" htmlFor="income-start" error={errors.startMonth?.message}>
                <Input id="income-start" type="month" {...form.register("startMonth")} />
              </Field>
            </div>
            <Field label="Termina em (opcional)" htmlFor="income-end" error={errors.endOn?.message}>
              <Input id="income-end" type="date" {...form.register("endOn")} />
            </Field>
            <p className="-mt-2 text-xs text-muted-foreground">Use para bolsas e contratos com fim. O Radar avisa 30 dias antes.</p>
          </>
        ) : (
          <Field label="Data prevista" htmlFor="income-expected" error={errors.expectedOn?.message}>
            <Input id="income-expected" type="date" {...form.register("expectedOn")} />
          </Field>
        )}
        <FieldError message={mutation.error?.message ?? remove.error?.message} />
        <DialogActions className="sm:justify-between">
          {editing && !inUse ? (
            <Button variant="ghost" className="text-destructive" disabled={remove.isPending} onClick={() => remove.mutate(editing.id, { onSuccess: close })}>
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
              {mutation.isPending ? "Salvando..." : "Salvar"}
            </Button>
          </span>
        </DialogActions>
      </form>
    </Dialog>
  );
}

const confirmSchema = z.object({
  amountCents: z.number().int().positive("Informe o valor recebido."),
  date: dateField,
  accountId: z.number().int().positive("Escolha a conta."),
});

/** Confirms a receipt with the real amount, date and account (RF-10, RN-13). */
function ConfirmDialog({ occurrence: o, month, onClose }: { occurrence: IncomeOccurrence | null; month: string; onClose: () => void }) {
  const accounts = useAccounts();
  const confirm = useConfirmIncome();
  const form = useForm<z.infer<typeof confirmSchema>>({
    resolver: zodResolver(confirmSchema),
    resetOptions: { keepDirtyValues: true },
    values: { amountCents: o?.expectedCents ?? 0, date: o?.expectedOn ?? todayISO(), accountId: o?.accountId ?? 0 },
  });
  const errors = form.formState.errors;

  /** Clears the mutation state and closes. */
  const close = () => {
    form.reset();
    confirm.reset();
    onClose();
  };

  /** Validates and sends the form. */
  const submit = form.handleSubmit((v) => o && confirm.mutate({ id: o.incomeId, body: { month, ...v } }, { onSuccess: close }));

  return (
    <Dialog open={o !== null} onOpenChange={(open) => !open && close()} title={`Recebi ${o?.name ?? ""}`} description="Confira o valor que caiu na conta.">
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <Field label="Valor recebido" htmlFor="confirm-amount" error={errors.amountCents?.message}>
          <Controller control={form.control} name="amountCents" render={({ field }) => <MoneyInput id="confirm-amount" value={field.value} onChange={field.onChange} invalid={!!errors.amountCents} />} />
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Data" htmlFor="confirm-date" error={errors.date?.message}>
            <Input id="confirm-date" type="date" {...form.register("date")} />
          </Field>
          <Field label="Conta" htmlFor="confirm-account" error={errors.accountId?.message}>
            <NativeSelect id="confirm-account" {...form.register("accountId", { valueAsNumber: true })}>
              {(accounts.data ?? []).filter((a) => a.active).map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </NativeSelect>
          </Field>
        </div>
        <FieldError message={confirm.error?.message} />
        <DialogActions>
          <Button variant="ghost" onClick={close}>
            Cancelar
          </Button>
          <Button type="submit" disabled={confirm.isPending}>
            <Check />
            Confirmar
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}

const oneOffSchema = z.object({
  accountId: z.number().int().positive("Escolha a conta."),
  amountCents: z.number().int().positive("Informe um valor maior que zero."),
  date: dateField,
  description: z.string().trim().max(120, "Use no máximo 120 caracteres."),
});

/** Records an income that was not registered, like a sale or a freelance job (RF-13). */
function OneOffDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const accounts = useAccounts();
  const add = useAddOneOffIncome();
  const form = useForm<z.infer<typeof oneOffSchema>>({
    resolver: zodResolver(oneOffSchema),
    resetOptions: { keepDirtyValues: true },
    values: { accountId: 0, amountCents: 0, date: todayISO(), description: "" },
  });
  const errors = form.formState.errors;

  /** Clears the mutation state and closes. */
  const close = () => {
    add.reset();
    form.reset();
    onClose();
  };

  /** Validates and sends the form. */
  const submit = form.handleSubmit(({ accountId, ...body }) => add.mutate({ accountId, body }, { onSuccess: close }));

  return (
    <Dialog open={open} onOpenChange={(o) => !o && close()} title="Renda avulsa" description="Dinheiro que entrou fora das rendas cadastradas.">
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <Field label="Descrição" htmlFor="oneoff-desc" error={errors.description?.message}>
          <Input id="oneoff-desc" autoComplete="off" placeholder="Venda, freela, reembolso..." {...form.register("description")} />
        </Field>
        <Field label="Valor" htmlFor="oneoff-amount" error={errors.amountCents?.message}>
          <Controller control={form.control} name="amountCents" render={({ field }) => <MoneyInput id="oneoff-amount" value={field.value} onChange={field.onChange} invalid={!!errors.amountCents} />} />
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Conta" htmlFor="oneoff-account" error={errors.accountId?.message}>
            <NativeSelect id="oneoff-account" {...form.register("accountId", { valueAsNumber: true })}>
              <option value={0}>Escolha</option>
              {(accounts.data ?? []).filter((a) => a.active).map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </NativeSelect>
          </Field>
          <Field label="Data" htmlFor="oneoff-date" error={errors.date?.message}>
            <Input id="oneoff-date" type="date" {...form.register("date")} />
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
