import { zodResolver } from "@hookform/resolvers/zod";
import { motion } from "motion/react";
import { ChevronLeft, ChevronRight, Landmark, Plus } from "lucide-react";
import { useState } from "react";
import { Controller, useForm } from "react-hook-form";
import { Link } from "react-router";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Dialog, DialogActions } from "@/components/ui/dialog";
import { Field, FieldError } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Money } from "@/components/ui/money";
import { MoneyInput } from "@/components/ui/money-input";
import { NativeSelect } from "@/components/ui/native-select";
import { useAccounts, useCreateAccount, usePersons, useUpdateAccount, type Account } from "@/features/accounts/api";
import { spring } from "@/lib/motion";
import { cn } from "@/lib/utils";

/** List of bank accounts with their balances, in Settings (RF-04, RF-05). */
export function AccountsPage() {
  const accounts = useAccounts();
  const [creating, setCreating] = useState(false);
  const list = accounts.data ?? [];
  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between gap-3">
        <div className="flex flex-col gap-0.5">
          <Link to="/settings" className="flex items-center gap-0.5 text-sm text-primary">
            <ChevronLeft className="size-4" />
            Ajustes
          </Link>
          <h1 className="px-1 text-lg font-semibold">Contas bancárias</h1>
        </div>
        <Button size="sm" onClick={() => setCreating(true)}>
          <Plus />
          Nova conta
        </Button>
      </div>

      {accounts.isPending && <p className="px-1 text-sm text-muted-foreground">Carregando...</p>}
      {accounts.isError && <p className="px-1 text-sm text-destructive">Não foi possível carregar as contas.</p>}
      {accounts.isSuccess && list.length === 0 && (
        <Card className="flex flex-col items-center gap-3 py-10 text-center">
          <span className="grid size-14 place-items-center rounded-2xl bg-primary/10">
            <Landmark className="size-7 text-primary" />
          </span>
          <p className="max-w-xs text-sm text-muted-foreground">Cadastre as contas de cada um com o saldo de hoje. O Radar acompanha o saldo a partir daí.</p>
          <Button onClick={() => setCreating(true)}>
            <Plus />
            Cadastrar a primeira conta
          </Button>
        </Card>
      )}

      <div className="grid gap-3 sm:grid-cols-2">
        {list.map((a, i) => (
          <motion.div key={a.id} initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ ...spring, delay: i * 0.03 }}>
            <Link to={`/settings/accounts/${a.id}`} className="block">
              <Card className={cn("flex items-center gap-3 transition-transform active:scale-[0.98]", !a.active && "opacity-60")}>
                <span className="grid size-11 shrink-0 place-items-center rounded-xl bg-primary/10">
                  <Landmark className="size-5 text-primary" />
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-medium">{a.name}</span>
                  <span className="block text-xs text-muted-foreground">
                    {a.personName}
                    {!a.active && " · Desativada"}
                  </span>
                </span>
                <Money cents={a.balanceCents} className={cn("text-lg font-semibold", a.balanceCents < 0 && "text-destructive")} />
                <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
              </Card>
            </Link>
          </motion.div>
        ))}
      </div>

      <AccountFormDialog open={creating} onClose={() => setCreating(false)} />
    </div>
  );
}

const accountSchema = z.object({
  name: z.string().trim().min(1, "Informe o nome da conta.").max(40, "Use no máximo 40 caracteres."),
  personId: z.number({ message: "Escolha o titular." }).int().positive("Escolha o titular."),
  initialBalanceCents: z.number().int(),
});
type AccountValues = z.infer<typeof accountSchema>;

/** Creates an account or edits name, holder and initial balance of one (RF-04). */
export function AccountFormDialog({ open, onClose, account }: { open: boolean; onClose: () => void; account?: Account }) {
  const persons = usePersons();
  const create = useCreateAccount();
  const update = useUpdateAccount();
  const mutation = account ? update : create;
  const form = useForm<AccountValues>({
    resolver: zodResolver(accountSchema),
    resetOptions: { keepDirtyValues: true },
    values: {
      name: account?.name ?? "",
      personId: account?.personId ?? 0,
      initialBalanceCents: account?.initialBalanceCents ?? 0,
    },
  });
  const errors = form.formState.errors;

  /** Clears the mutation state and closes. */
  const close = () => {
    mutation.reset();
    form.reset();
    onClose();
  };

  /** Sends the form. */
  const submit = form.handleSubmit((v) => {
    const done = { onSuccess: close };
    if (account) update.mutate({ id: account.id, body: v }, done);
    else create.mutate(v, done);
  });

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => !o && close()}
      title={account ? "Editar conta" : "Nova conta"}
      description={account ? undefined : "Use o saldo que o app do banco mostra hoje."}
    >
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <Field label="Nome" htmlFor="account-name" error={errors.name?.message}>
          <Input id="account-name" autoComplete="off" placeholder="Nubank, Inter, Caixa..." aria-invalid={!!errors.name} {...form.register("name")} />
        </Field>
        <Field label="Titular" htmlFor="account-person" error={errors.personId?.message}>
          <NativeSelect id="account-person" aria-invalid={!!errors.personId} {...form.register("personId", { valueAsNumber: true })}>
            <option value={0}>Escolha</option>
            {(persons.data ?? []).map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </NativeSelect>
        </Field>
        <Field label="Saldo inicial" htmlFor="account-initial">
          <Controller
            control={form.control}
            name="initialBalanceCents"
            render={({ field }) => <MoneyInput id="account-initial" value={field.value} onChange={field.onChange} allowNegative />}
          />
        </Field>
        {account && (
          <p className="text-xs text-muted-foreground">Mudar o saldo inicial recalcula o saldo atual. Para acertar o saldo de hoje, use Ajustar saldo.</p>
        )}
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
