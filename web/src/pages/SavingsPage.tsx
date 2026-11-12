import { zodResolver } from "@hookform/resolvers/zod";
import { AnimatePresence, motion } from "motion/react";
import { ArrowDownToLine, ArrowUpFromLine, ChevronDown, Coins, Pencil, PiggyBank, Plus, Trash2, Undo2 } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Controller, useForm } from "react-hook-form";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Card, CardDescription, CardTitle } from "@/components/ui/card";
import { Confetti } from "@/components/ui/confetti";
import { Dialog, DialogActions } from "@/components/ui/dialog";
import { Field, FieldError } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Money } from "@/components/ui/money";
import { MoneyFlow } from "@/components/ui/money-flow";
import { MoneyInput } from "@/components/ui/money-input";
import { NativeSelect } from "@/components/ui/native-select";
import { useAccounts } from "@/features/accounts/api";
import {
  formatPercent,
  useCreateSavings,
  useDeleteSavings,
  useDepositSavings,
  useSavingsMonth,
  useUndoSavingsMove,
  useUpdateSavings,
  useWithdrawSavings,
  type SavingsBox,
} from "@/features/savings/api";
import { formatDayMonth, monthName, monthParam, todayISO } from "@/lib/format";
import { fade, spring } from "@/lib/motion";
import { useMonth } from "@/lib/month";
import { cn } from "@/lib/utils";

type DialogState = { kind: "form"; box?: SavingsBox } | { kind: "deposit"; box: SavingsBox } | { kind: "withdraw"; box: SavingsBox };

/** Savings boxes: base of the month, suggested deposits, progress, estimate and history (RF-43 to RF-49). */
export function SavingsPage() {
  const { year, month } = useMonth();
  const param = monthParam(year, month);
  const view = useSavingsMonth(param);
  const [dialog, setDialog] = useState<DialogState | null>(null);
  const [coinFor, setCoinFor] = useState<number | null>(null);
  const v = view.data;

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between gap-3">
        <h1 className="px-1 text-lg font-semibold md:hidden">Caixinhas</h1>
        <Button size="sm" className="ml-auto" onClick={() => setDialog({ kind: "form" })}>
          <Plus />
          Nova caixinha
        </Button>
      </div>

      {v && (
        <Card className="flex items-center justify-between gap-3">
          <div>
            <CardTitle>Entradas de {monthName(year, month).toLowerCase()}</CardTitle>
            <CardDescription>Base dos aportes sugeridos: salários, bolsa, 13º, férias e rendas avulsas que já caíram.</CardDescription>
          </div>
          <MoneyFlow cents={v.baseCents} className="shrink-0 text-2xl font-semibold" />
        </Card>
      )}

      {v && v.boxes.length === 0 && (
        <Card className="flex flex-col items-center gap-3 py-10 text-center">
          <span className="grid size-14 place-items-center rounded-2xl bg-success/10">
            <PiggyBank className="size-7 text-success" />
          </span>
          <p className="max-w-xs text-sm text-muted-foreground">Crie uma caixinha com o valor-alvo e o percentual das entradas de cada mês.</p>
        </Card>
      )}

      <div className="grid gap-4 lg:grid-cols-2">
        {(v?.boxes ?? []).map((b) => (
          <BoxCard
            key={b.id}
            box={b}
            coin={coinFor === b.id}
            onDeposit={() => setDialog({ kind: "deposit", box: b })}
            onWithdraw={() => setDialog({ kind: "withdraw", box: b })}
            onEdit={() => setDialog({ kind: "form", box: b })}
          />
        ))}
      </div>

      <BoxFormDialog state={dialog?.kind === "form" ? dialog : null} onClose={() => setDialog(null)} />
      <MoveDialog
        state={dialog?.kind === "deposit" || dialog?.kind === "withdraw" ? dialog : null}
        month={param}
        onClose={() => setDialog(null)}
        onDeposited={(id) => {
          setCoinFor(id);
          window.setTimeout(() => setCoinFor(null), 900);
        }}
      />
    </div>
  );
}

/** One box with balance, progress, suggestion and history; it throws confetti when it becomes done (section 8.6). */
function BoxCard({ box: b, coin, onDeposit, onWithdraw, onEdit }: { box: SavingsBox; coin: boolean; onDeposit: () => void; onWithdraw: () => void; onEdit: () => void }) {
  const undo = useUndoSavingsMove();
  const [history, setHistory] = useState(false);
  const wasDone = useRef(b.done);
  const [celebrate, setCelebrate] = useState(false);

  // Celebrates only the moment the box becomes done, not every render of a done box.
  useEffect(() => {
    if (b.done && !wasDone.current) {
      setCelebrate(true);
      const t = window.setTimeout(() => setCelebrate(false), 1400);
      wasDone.current = true;
      return () => window.clearTimeout(t);
    }
    wasDone.current = b.done;
  }, [b.done]);

  return (
    <Card className="relative flex flex-col gap-3">
      {celebrate && <Confetti />}
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <CardTitle className="truncate text-base">{b.name}</CardTitle>
            {b.done && <span className="rounded-full bg-success/10 px-2 py-0.5 text-[0.7rem] font-medium text-success">Concluída</span>}
          </div>
          <CardDescription>
            {formatPercent(b.percentBp)} das entradas · alvo <Money cents={b.targetCents} />
          </CardDescription>
        </div>
        <Button variant="ghost" size="icon-sm" aria-label={`Editar ${b.name}`} onClick={onEdit}>
          <Pencil />
        </Button>
      </div>

      <div className="relative flex items-end justify-between gap-3">
        <MoneyFlow cents={b.balanceCents} className="text-3xl font-semibold tracking-tight" />
        <span className="text-sm font-medium text-success tabular-nums">{formatPercent(b.progressBp)}</span>
        <AnimatePresence>
          {coin && (
            <motion.span
              className="pointer-events-none absolute left-1/2 top-0"
              initial={{ y: -40, opacity: 0, rotate: -30 }}
              animate={{ y: 30, opacity: [0, 1, 1, 0], rotate: 0 }}
              exit={{ opacity: 0 }}
              transition={{ duration: 0.8, ease: "easeIn" }}
              aria-hidden
            >
              <Coins className="size-6 text-warning" />
            </motion.span>
          )}
        </AnimatePresence>
      </div>
      <div className="h-2.5 overflow-hidden rounded-full bg-muted" role="meter" aria-label={`Progresso de ${b.name}`} aria-valuemin={0} aria-valuemax={10000} aria-valuenow={b.progressBp}>
        <motion.div
          className="h-full rounded-full bg-gradient-to-r from-success-strong to-success-soft"
          initial={{ width: 0 }}
          animate={{ width: `${b.progressBp / 100}%` }}
          transition={spring}
        />
      </div>
      <p className="text-xs text-muted-foreground">
        {b.done
          ? "Valor-alvo atingido. A caixinha não recebe mais sugestões, mas continua no histórico."
          : b.estimatedMonth
            ? `No ritmo dos últimos meses, conclui em ${monthLabel(b.estimatedMonth)}.`
            : "A data estimada aparece depois do primeiro mês com aportes."}
      </p>

      {!b.done && (
        <div className="flex items-center justify-between gap-3 rounded-xl bg-muted px-3 py-2 text-sm">
          {b.depositedCents > 0 ? (
            <span className="text-success">
              Aporte do mês confirmado: <Money cents={b.depositedCents} className="font-medium" />
            </span>
          ) : (
            <span>
              Aporte sugerido: <Money cents={b.suggestedCents} className="font-semibold" />
            </span>
          )}
          <Button size="sm" onClick={onDeposit}>
            <ArrowDownToLine />
            {b.depositedCents > 0 ? "Aportar mais" : "Confirmar"}
          </Button>
        </div>
      )}

      <div className="flex items-center justify-between gap-2">
        <button type="button" onClick={() => setHistory((h) => !h)} className="flex items-center gap-1 text-xs text-muted-foreground" aria-expanded={history}>
          <motion.span animate={{ rotate: history ? 180 : 0 }} transition={spring}>
            <ChevronDown className="size-3.5" />
          </motion.span>
          Histórico ({b.moves.length})
        </button>
        <Button variant="ghost" size="sm" onClick={onWithdraw} disabled={b.balanceCents <= 0}>
          <ArrowUpFromLine />
          Retirar
        </Button>
      </div>
      <AnimatePresence initial={false}>
        {history && (
          <motion.ul initial={{ height: 0, opacity: 0 }} animate={{ height: "auto", opacity: 1 }} exit={{ height: 0, opacity: 0 }} transition={{ height: spring, opacity: fade }} className="overflow-hidden">
            {b.moves.map((m) => (
              <li key={m.id} className="flex items-center gap-3 border-b py-2 text-sm last:border-b-0">
                <span className="w-10 shrink-0 text-xs text-muted-foreground">{formatDayMonth(m.date)}</span>
                <span className="min-w-0 flex-1 truncate">
                  {m.kind === "deposit" ? `Aporte de ${m.accountName}` : `Retirada para ${m.accountName}: ${m.reason}`}
                </span>
                <Money cents={m.kind === "deposit" ? m.amountCents : -m.amountCents} signed className={cn("text-sm", m.kind === "deposit" && "text-success")} />
                <Button variant="ghost" size="icon-sm" aria-label="Desfazer movimento" disabled={undo.isPending} onClick={() => undo.mutate(m.entryId)}>
                  <Undo2 />
                </Button>
              </li>
            ))}
            {b.moves.length === 0 && <li className="py-2 text-sm text-muted-foreground">Nenhum movimento ainda.</li>}
          </motion.ul>
        )}
      </AnimatePresence>
    </Card>
  );
}

/** Month name and year of a YYYY-MM month, like dezembro de 2027. */
function monthLabel(m: string) {
  const [y, mm] = m.split("-").map(Number);
  return `${monthName(y!, mm!).toLowerCase()} de ${y}`;
}

const boxSchema = z.object({
  name: z.string().trim().min(1, "Informe o nome.").max(40, "Use no máximo 40 caracteres."),
  targetCents: z.number().int().positive("Informe o valor-alvo."),
  percentBp: z.number().int().min(0).max(10000),
  initialBalanceCents: z.number().int().min(0),
});

/** Creates or edits a box, with the monthly percentage on a slider (RF-43, RF-44, section 8.6). */
function BoxFormDialog({ state, onClose }: { state: { box?: SavingsBox } | null; onClose: () => void }) {
  const create = useCreateSavings();
  const update = useUpdateSavings();
  const remove = useDeleteSavings();
  const box = state?.box;
  const mutation = box ? update : create;
  const form = useForm<z.infer<typeof boxSchema>>({
    resolver: zodResolver(boxSchema),
    resetOptions: { keepDirtyValues: true },
    values: { name: box?.name ?? "", targetCents: box?.targetCents ?? 0, percentBp: box?.percentBp ?? 1000, initialBalanceCents: 0 },
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

  /** Validates and sends the form; the starting balance only exists on creation. */
  const submit = form.handleSubmit(({ initialBalanceCents, ...v }) => {
    if (box) update.mutate({ id: box.id, body: v }, { onSuccess: close });
    else create.mutate({ ...v, initialBalanceCents }, { onSuccess: close });
  });

  return (
    <Dialog open={state !== null} onOpenChange={(o) => !o && close()} title={box ? "Editar caixinha" : "Nova caixinha"}>
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <Field label="Nome" htmlFor="box-name" error={errors.name?.message}>
          <Input id="box-name" autoComplete="off" placeholder="Reserva, viagem, carro..." {...form.register("name")} />
        </Field>
        <Field label="Valor-alvo" htmlFor="box-target" error={errors.targetCents?.message}>
          <Controller control={form.control} name="targetCents" render={({ field }) => <MoneyInput id="box-target" value={field.value} onChange={field.onChange} />} />
        </Field>
        <Controller
          control={form.control}
          name="percentBp"
          render={({ field }) => (
            <div className="flex flex-col gap-2">
              <div className="flex items-center justify-between text-sm">
                <label htmlFor="box-percent" className="font-medium">
                  Percentual das entradas do mês
                </label>
                <span className="font-semibold tabular-nums text-success">{formatPercent(field.value)}</span>
              </div>
              <input
                id="box-percent"
                type="range"
                min={0}
                max={10000}
                step={50}
                value={field.value}
                onChange={(e) => field.onChange(Number(e.target.value))}
                className="savings-slider"
                style={{ ["--fill" as string]: `${field.value / 100}%` }}
              />
            </div>
          )}
        />
        {!box && (
          <Field label="Já guardado hoje" htmlFor="box-initial">
            <Controller control={form.control} name="initialBalanceCents" render={({ field }) => <MoneyInput id="box-initial" value={field.value} onChange={field.onChange} />} />
          </Field>
        )}
        <FieldError message={mutation.error?.message ?? remove.error?.message} />
        <DialogActions className="sm:justify-between">
          {box ? (
            <Button variant="ghost" className="text-destructive" disabled={remove.isPending} onClick={() => remove.mutate(box.id, { onSuccess: close })}>
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

const moveSchema = z.object({
  amountCents: z.number().int().positive("Informe o valor."),
  accountId: z.number().int().positive("Escolha a conta."),
  date: z.string().regex(/^\d{4}-\d{2}-\d{2}$/, "Informe a data."),
  reason: z.string().trim().max(120, "Use no máximo 120 caracteres."),
});

/** Confirms a deposit with the real amount, or records a withdrawal with a reason (RF-46, RF-47). */
function MoveDialog({
  state,
  month,
  onClose,
  onDeposited,
}: {
  state: { kind: "deposit" | "withdraw"; box: SavingsBox } | null;
  month: string;
  onClose: () => void;
  onDeposited: (id: number) => void;
}) {
  const accounts = useAccounts();
  const deposit = useDepositSavings();
  const withdraw = useWithdrawSavings();
  const isDeposit = state?.kind === "deposit";
  const mutation = isDeposit ? deposit : withdraw;
  const form = useForm<z.infer<typeof moveSchema>>({
    resolver: zodResolver(moveSchema),
    resetOptions: { keepDirtyValues: true },
    values: { amountCents: isDeposit ? (state?.box.suggestedCents ?? 0) : 0, accountId: 0, date: todayISO(), reason: "" },
  });
  const errors = form.formState.errors;

  /** Clears the mutation state and closes. */
  const close = () => {
    deposit.reset();
    withdraw.reset();
    form.reset();
    onClose();
  };

  /** Validates and sends the move. */
  const submit = form.handleSubmit((v) => {
    if (!state) return;
    if (isDeposit)
      deposit.mutate(
        { id: state.box.id, body: { month, amountCents: v.amountCents, accountId: v.accountId, date: v.date } },
        {
          onSuccess: () => {
            onDeposited(state.box.id);
            close();
          },
        },
      );
    else if (!v.reason) form.setError("reason", { message: "Informe o motivo da retirada." });
    else withdraw.mutate({ id: state.box.id, body: { amountCents: v.amountCents, accountId: v.accountId, date: v.date, reason: v.reason } }, { onSuccess: close });
  });

  return (
    <Dialog
      open={state !== null}
      onOpenChange={(o) => !o && close()}
      title={isDeposit ? `Aporte em ${state?.box.name ?? ""}` : `Retirar de ${state?.box.name ?? ""}`}
      description={isDeposit ? "Confirme o valor que você realmente guardou. Não conta como gasto." : "O dinheiro volta para a conta. Não conta como renda."}
    >
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <Field label={isDeposit ? "Valor guardado" : "Valor retirado"} htmlFor="move-amount" error={errors.amountCents?.message}>
          <Controller control={form.control} name="amountCents" render={({ field }) => <MoneyInput id="move-amount" value={field.value} onChange={field.onChange} />} />
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label={isDeposit ? "Saiu da conta" : "Voltou para a conta"} htmlFor="move-account" error={errors.accountId?.message}>
            <NativeSelect id="move-account" {...form.register("accountId", { valueAsNumber: true })}>
              <option value={0}>Escolha</option>
              {(accounts.data ?? []).filter((a) => a.active).map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </NativeSelect>
          </Field>
          <Field label="Data" htmlFor="move-date" error={errors.date?.message}>
            <Input id="move-date" type="date" {...form.register("date")} />
          </Field>
        </div>
        {!isDeposit && (
          <Field label="Motivo" htmlFor="move-reason" error={errors.reason?.message}>
            <Input id="move-reason" autoComplete="off" placeholder="Conserto do carro, viagem..." {...form.register("reason")} />
          </Field>
        )}
        <FieldError message={mutation.error?.message} />
        <DialogActions>
          <Button variant="ghost" onClick={close}>
            Cancelar
          </Button>
          <Button type="submit" disabled={mutation.isPending}>
            {isDeposit ? "Confirmar aporte" : "Retirar"}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}
