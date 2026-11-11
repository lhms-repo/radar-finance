import { zodResolver } from "@hookform/resolvers/zod";
import { AnimatePresence, motion } from "motion/react";
import { DropdownMenu } from "radix-ui";
import { ArrowDownLeft, ArrowLeftRight, ArrowUpRight, ChevronLeft, CreditCard, Eye, EyeOff, MinusCircle, MoreHorizontal, Pencil, Scale, Trash2 } from "lucide-react";
import { useState } from "react";
import { Controller, useForm } from "react-hook-form";
import { Link, useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { z } from "zod";
import { ApiRequestError } from "@/api/client";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { Dialog, DialogActions } from "@/components/ui/dialog";
import { Field, FieldError } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Money } from "@/components/ui/money";
import { MoneyInput } from "@/components/ui/money-input";
import { NativeSelect } from "@/components/ui/native-select";
import {
  useAccount,
  useAccounts,
  useAddExpense,
  useAdjustBalance,
  useDeleteAccount,
  useDeleteEntry,
  useStatement,
  useTransfer,
  useUpdateAccount,
  type Account,
  type Entry,
} from "@/features/accounts/api";
import { CategorySelect } from "@/features/categories/CategorySelect";
import { CategoryIcon } from "@/features/categories/icons";
import { formatDate, formatDayMonth, monthName, monthParam, todayISO } from "@/lib/format";
import { fade, spring } from "@/lib/motion";
import { useMonth } from "@/lib/month";
import { cn } from "@/lib/utils";
import { AccountFormDialog } from "@/pages/AccountsPage";

type DialogKind = "adjust" | "transfer" | "expense" | "edit" | "delete";

/** One bank account: balance, actions and the statement of the selected month (RF-05 to RF-07). */
export function AccountDetailPage() {
  const id = Number(useParams().id);
  const account = useAccount(id);
  const { year, month } = useMonth();
  const statement = useStatement(id, monthParam(year, month));
  const [dialog, setDialog] = useState<DialogKind | null>(null);
  const [removing, setRemoving] = useState<Entry | null>(null);
  const toggle = useUpdateAccount();
  const a = account.data;

  if (account.isError) {
    return (
      <div className="flex flex-col gap-3">
        <BackLink />
        <p className="px-1 text-sm text-destructive">{account.error.message}</p>
      </div>
    );
  }
  if (!a) return <p className="px-1 text-sm text-muted-foreground">Carregando...</p>;

  const st = statement.data;
  return (
    <div className="flex flex-col gap-4">
      <BackLink />
      <Card className="flex flex-col gap-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <h1 className="truncate text-lg font-semibold">{a.name}</h1>
            <p className="text-sm text-muted-foreground">
              {a.personName}
              {!a.active && " · Desativada"}
            </p>
          </div>
          <AccountMenu
            account={a}
            onEdit={() => setDialog("edit")}
            onToggle={() =>
              toggle.mutate({ id: a.id, body: { active: !a.active } }, { onError: (err) => toast.error(err.message) })
            }
            onDelete={() => setDialog("delete")}
          />
        </div>
        <div>
          <p className="text-sm text-muted-foreground">Saldo atual</p>
          <Money cents={a.balanceCents} className={cn("text-3xl font-semibold tracking-tight", a.balanceCents < 0 && "text-destructive")} />
        </div>
        {a.active && (
          <div className="grid grid-cols-3 gap-2">
            <ActionButton icon={<MinusCircle />} label="Saída" onClick={() => setDialog("expense")} />
            <ActionButton icon={<ArrowLeftRight />} label="Transferir" onClick={() => setDialog("transfer")} />
            <ActionButton icon={<Scale />} label="Ajustar saldo" onClick={() => setDialog("adjust")} />
          </div>
        )}
      </Card>

      <Card className="flex flex-col gap-3">
        <div className="flex items-baseline justify-between gap-3">
          <CardTitle>
            {monthName(year, month)} de {year}
          </CardTitle>
          {statement.isFetching && <span className="text-xs text-muted-foreground">Atualizando...</span>}
        </div>
        {st && (
          <dl className="grid grid-cols-2 gap-3 rounded-xl bg-muted p-3 text-sm">
            <div>
              <dt className="text-muted-foreground">Início do mês</dt>
              <dd className="font-medium">
                <Money cents={st.openingCents} />
              </dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Fim do mês</dt>
              <dd className="font-medium">
                <Money cents={st.closingCents} />
              </dd>
            </div>
          </dl>
        )}
        <ul className="flex flex-col">
          <AnimatePresence initial={false}>
            {(st?.entries ?? []).map((e) => (
              <EntryRow key={e.id} entry={e} onDelete={() => setRemoving(e)} />
            ))}
          </AnimatePresence>
        </ul>
        {st?.entries.length === 0 && <p className="py-4 text-center text-sm text-muted-foreground">Nenhum lançamento neste mês.</p>}
      </Card>

      <AdjustDialog open={dialog === "adjust"} account={a} onClose={() => setDialog(null)} />
      <TransferDialog open={dialog === "transfer"} account={a} onClose={() => setDialog(null)} />
      <ExpenseDialog open={dialog === "expense"} account={a} onClose={() => setDialog(null)} />
      <AccountFormDialog open={dialog === "edit"} account={a} onClose={() => setDialog(null)} />
      <DeleteAccountDialog open={dialog === "delete"} account={a} onClose={() => setDialog(null)} />
      <DeleteEntryDialog entry={removing} onClose={() => setRemoving(null)} />
    </div>
  );
}

/** Link back to the account list. */
function BackLink() {
  return (
    <Link to="/settings/accounts" className="flex items-center gap-0.5 text-sm text-primary">
      <ChevronLeft className="size-4" />
      Contas bancárias
    </Link>
  );
}

/** Square button with an icon over a short label. */
function ActionButton({ icon, label, onClick }: { icon: React.ReactNode; label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex flex-col items-center gap-1.5 rounded-xl bg-muted px-2 py-3 text-xs font-medium transition-transform active:scale-[0.96] [&_svg]:size-5 [&_svg]:text-primary"
    >
      {icon}
      {label}
    </button>
  );
}

/** Menu with edit, deactivate and delete for the account. */
function AccountMenu({ account, onEdit, onToggle, onDelete }: { account: Account; onEdit: () => void; onToggle: () => void; onDelete: () => void }) {
  const item = "flex cursor-pointer items-center gap-2 rounded-lg px-3 py-2 text-sm outline-none data-[highlighted]:bg-accent";
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label="Ações da conta">
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
            {account.active ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
            {account.active ? "Desativar" : "Reativar"}
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

/** Title shown for an entry in the statement. */
function entryTitle(e: Entry) {
  switch (e.kind) {
    case "expense":
      return e.categoryName ?? "Saída";
    case "income":
      return e.description || "Renda";
    case "adjustment":
      return "Ajuste de saldo";
    case "invoice_payment":
      return "Pagamento de fatura";
    case "transfer":
      return e.amountCents < 0 ? `Transferência para ${e.counterpartAccountName ?? "outra conta"}` : `Transferência de ${e.counterpartAccountName ?? "outra conta"}`;
  }
}

/** One statement line with icon, title, author and signed amount. */
function EntryRow({ entry: e, onDelete }: { entry: Entry; onDelete: () => void }) {
  const subtitle = [
    e.kind === "expense" && e.parentCategoryName,
    e.kind !== "income" && e.description,
    `por ${e.createdByName}`,
  ]
    .filter(Boolean)
    .join(" · ");
  return (
    <motion.li
      layout
      initial={{ opacity: 0, y: -6 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, height: 0 }}
      transition={{ ...spring, opacity: fade }}
      className="group flex items-center gap-3 border-b py-2.5 last:border-b-0"
    >
      <span className="w-10 shrink-0 text-xs text-muted-foreground" title={formatDate(e.date)}>
        {formatDayMonth(e.date)}
      </span>
      <span className="grid size-9 shrink-0 place-items-center rounded-xl bg-muted">
        <EntryIcon entry={e} />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-medium">{entryTitle(e)}</span>
        <span className="block truncate text-xs text-muted-foreground">{subtitle}</span>
      </span>
      <span className="flex flex-col items-end">
        <Money cents={e.amountCents} signed className={cn("text-sm font-medium", e.amountCents > 0 && "text-success")} />
        {!e.confirmed && <span className="text-[0.7rem] text-warning">Previsto</span>}
      </span>
      <Button variant="ghost" size="icon-sm" aria-label={`Excluir ${entryTitle(e)}`} onClick={onDelete} className="text-muted-foreground">
        <Trash2 />
      </Button>
    </motion.li>
  );
}

/** Icon that tells the kind of an entry. */
function EntryIcon({ entry: e }: { entry: Entry }) {
  const cls = "size-4 text-primary";
  if (e.kind === "expense") return <CategoryIcon name={e.categoryIcon} className={cls} />;
  if (e.kind === "transfer") return <ArrowLeftRight className={cls} />;
  if (e.kind === "adjustment") return <Scale className={cls} />;
  if (e.kind === "invoice_payment") return <CreditCard className={cls} />;
  return e.amountCents > 0 ? <ArrowDownLeft className="size-4 text-success" /> : <ArrowUpRight className={cls} />;
}

const dateField = z.string().regex(/^\d{4}-\d{2}-\d{2}$/, "Informe a data.");
const descriptionField = z.string().trim().max(120, "Use no máximo 120 caracteres.");

const adjustSchema = z.object({ balanceCents: z.number().int(), date: dateField, description: descriptionField });

/** Sets the balance to what the bank shows; the difference becomes an adjustment (RF-06). */
function AdjustDialog({ open, account, onClose }: { open: boolean; account: Account; onClose: () => void }) {
  const adjust = useAdjustBalance();
  const form = useForm<z.infer<typeof adjustSchema>>({
    resolver: zodResolver(adjustSchema),
    values: { balanceCents: account.balanceCents, date: todayISO(), description: "" },
  });
  /** Clears the mutation state and closes. */
  const close = () => {
    adjust.reset();
    onClose();
  };
  /** Validates and sends the form. */
  const submit = form.handleSubmit((v) => adjust.mutate({ id: account.id, body: v }, { onSuccess: close }));
  return (
    <Dialog open={open} onOpenChange={(o) => !o && close()} title="Ajustar saldo" description="Informe o saldo que o app do banco mostra. A diferença vira um lançamento de ajuste.">
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <p className="text-sm">
          Saldo no Radar: <Money cents={account.balanceCents} className="font-medium" />
        </p>
        <Field label="Saldo real" htmlFor="adjust-balance">
          <Controller control={form.control} name="balanceCents" render={({ field }) => <MoneyInput id="adjust-balance" value={field.value} onChange={field.onChange} allowNegative autoFocus />} />
        </Field>
        <Field label="Data" htmlFor="adjust-date" error={form.formState.errors.date?.message}>
          <Input id="adjust-date" type="date" {...form.register("date")} />
        </Field>
        <Field label="Observação" htmlFor="adjust-desc" error={form.formState.errors.description?.message}>
          <Input id="adjust-desc" autoComplete="off" placeholder="Opcional" {...form.register("description")} />
        </Field>
        <FieldError message={adjust.error?.message} />
        <DialogActions>
          <Button variant="ghost" onClick={close}>
            Cancelar
          </Button>
          <Button type="submit" disabled={adjust.isPending}>
            Ajustar
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}

const transferSchema = z.object({
  toAccountId: z.number().int().positive("Escolha a conta de destino."),
  amountCents: z.number().int().positive("Informe um valor maior que zero."),
  date: dateField,
  description: descriptionField,
});

/** Moves money from this account to another one (RF-07, RN-08). */
function TransferDialog({ open, account, onClose }: { open: boolean; account: Account; onClose: () => void }) {
  const accounts = useAccounts();
  const transfer = useTransfer();
  const form = useForm<z.infer<typeof transferSchema>>({
    resolver: zodResolver(transferSchema),
    values: { toAccountId: 0, amountCents: 0, date: todayISO(), description: "" },
  });
  const errors = form.formState.errors;
  const targets = (accounts.data ?? []).filter((x) => x.active && x.id !== account.id);
  /** Clears the mutation state and closes. */
  const close = () => {
    transfer.reset();
    form.reset();
    onClose();
  };
  /** Validates and sends the form. */
  const submit = form.handleSubmit((v) => transfer.mutate({ fromAccountId: account.id, ...v }, { onSuccess: close }));
  return (
    <Dialog open={open} onOpenChange={(o) => !o && close()} title={`Transferir de ${account.name}`} description="Transferência não conta como gasto nem como receita.">
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <Field label="Para" htmlFor="transfer-to" error={errors.toAccountId?.message}>
          <NativeSelect id="transfer-to" aria-invalid={!!errors.toAccountId} {...form.register("toAccountId", { valueAsNumber: true })}>
            <option value={0}>Escolha a conta</option>
            {targets.map((t) => (
              <option key={t.id} value={t.id}>
                {t.name} ({t.personName})
              </option>
            ))}
          </NativeSelect>
        </Field>
        <Field label="Valor" htmlFor="transfer-amount" error={errors.amountCents?.message}>
          <Controller control={form.control} name="amountCents" render={({ field }) => <MoneyInput id="transfer-amount" value={field.value} onChange={field.onChange} invalid={!!errors.amountCents} />} />
        </Field>
        <Field label="Data" htmlFor="transfer-date" error={errors.date?.message}>
          <Input id="transfer-date" type="date" {...form.register("date")} />
        </Field>
        <Field label="Descrição" htmlFor="transfer-desc" error={errors.description?.message}>
          <Input id="transfer-desc" autoComplete="off" placeholder="Opcional" {...form.register("description")} />
        </Field>
        <FieldError message={transfer.error?.message} />
        <DialogActions>
          <Button variant="ghost" onClick={close}>
            Cancelar
          </Button>
          <Button type="submit" disabled={transfer.isPending}>
            Transferir
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}

const expenseSchema = z.object({
  amountCents: z.number().int().positive("Informe um valor maior que zero."),
  categoryId: z.number().int().positive("Escolha a categoria."),
  date: dateField,
  description: descriptionField,
});

/** One off expense paid from this account, until the quick entry of step 2.5 arrives. */
function ExpenseDialog({ open, account, onClose }: { open: boolean; account: Account; onClose: () => void }) {
  const add = useAddExpense();
  const form = useForm<z.infer<typeof expenseSchema>>({
    resolver: zodResolver(expenseSchema),
    values: { amountCents: 0, categoryId: 0, date: todayISO(), description: "" },
  });
  const errors = form.formState.errors;
  /** Clears the mutation state and closes. */
  const close = () => {
    add.reset();
    form.reset();
    onClose();
  };
  /** Validates and sends the form. */
  const submit = form.handleSubmit((v) => add.mutate({ id: account.id, body: v }, { onSuccess: close }));
  return (
    <Dialog open={open} onOpenChange={(o) => !o && close()} title={`Saída de ${account.name}`} description="Pix, débito, boleto ou dinheiro que sai direto da conta.">
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <Field label="Valor" htmlFor="expense-amount" error={errors.amountCents?.message}>
          <Controller control={form.control} name="amountCents" render={({ field }) => <MoneyInput id="expense-amount" value={field.value} onChange={field.onChange} invalid={!!errors.amountCents} autoFocus />} />
        </Field>
        <Field label="Categoria" htmlFor="expense-category" error={errors.categoryId?.message}>
          <CategorySelect id="expense-category" aria-invalid={!!errors.categoryId} {...form.register("categoryId", { valueAsNumber: true })} />
        </Field>
        <Field label="Data" htmlFor="expense-date" error={errors.date?.message}>
          <Input id="expense-date" type="date" {...form.register("date")} />
        </Field>
        <Field label="Descrição" htmlFor="expense-desc" error={errors.description?.message}>
          <Input id="expense-desc" autoComplete="off" placeholder="Opcional" {...form.register("description")} />
        </Field>
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

/** Confirms deleting the account and offers to deactivate it when it has entries. */
function DeleteAccountDialog({ open, account, onClose }: { open: boolean; account: Account; onClose: () => void }) {
  const remove = useDeleteAccount();
  const deactivate = useUpdateAccount("Conta desativada");
  const navigate = useNavigate();
  const inUse = remove.error instanceof ApiRequestError && remove.error.code === "account_in_use";
  /** Clears the mutation state and closes. */
  const close = () => {
    remove.reset();
    onClose();
  };
  return (
    <Dialog open={open} onOpenChange={(o) => !o && close()} title={`Excluir ${account.name}?`}>
      <p className="text-sm text-muted-foreground">{remove.error ? remove.error.message : "A conta some de vez. Isso só é possível enquanto ela não tem lançamentos."}</p>
      <DialogActions>
        <Button variant="ghost" onClick={close}>
          Cancelar
        </Button>
        {inUse && account.active ? (
          <Button onClick={() => deactivate.mutate({ id: account.id, body: { active: false } }, { onSuccess: close })}>
            <EyeOff />
            Desativar
          </Button>
        ) : (
          <Button
            variant="destructive"
            disabled={remove.isPending || !!remove.error}
            onClick={() => remove.mutate(account.id, { onSuccess: () => navigate("/settings/accounts", { replace: true }) })}
          >
            <Trash2 />
            Excluir
          </Button>
        )}
      </DialogActions>
    </Dialog>
  );
}

/** Confirms deleting an entry; a transfer goes away on both accounts. */
function DeleteEntryDialog({ entry, onClose }: { entry: Entry | null; onClose: () => void }) {
  const remove = useDeleteEntry();
  /** Clears the mutation state and closes. */
  const close = () => {
    remove.reset();
    onClose();
  };
  return (
    <Dialog open={entry !== null} onOpenChange={(o) => !o && close()} title="Excluir lançamento?">
      {entry && (
        <p className="text-sm text-muted-foreground">
          {entryTitle(entry)} de {formatDate(entry.date)}, <Money cents={entry.amountCents} />.
          {entry.kind === "transfer" && " A transferência sai das duas contas."}
        </p>
      )}
      <FieldError message={remove.error?.message} />
      <DialogActions>
        <Button variant="ghost" onClick={close}>
          Cancelar
        </Button>
        <Button variant="destructive" disabled={remove.isPending} onClick={() => entry && remove.mutate(entry.id, { onSuccess: close })}>
          <Trash2 />
          Excluir
        </Button>
      </DialogActions>
    </Dialog>
  );
}
