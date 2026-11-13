import { zodResolver } from "@hookform/resolvers/zod";
import { MemberSplitField, useSplitState } from "@/features/settle/MemberSplitField";
import { AnimatePresence, motion } from "motion/react";
import { ArrowRightLeft, Check, Pencil, Plus, ShoppingBasket, Trash2, Undo2, Utensils } from "lucide-react";
import { useMe } from "@/app/session";
import { MoveToSpace, SpaceField, useSpaceChoice } from "@/app/space";
import { useState } from "react";
import { Controller, useForm } from "react-hook-form";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Card, CardDescription, CardTitle } from "@/components/ui/card";
import { Dialog, DialogActions } from "@/components/ui/dialog";
import { Field, FieldError } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Money } from "@/components/ui/money";
import { MoneyFlow } from "@/components/ui/money-flow";
import { MoneyInput } from "@/components/ui/money-input";
import { NativeSelect } from "@/components/ui/native-select";
import { Switch } from "@/components/ui/switch";
import { Segmented } from "@/components/ui/segmented";
import { usePersons } from "@/features/accounts/api";
import { AleloTudoVisual } from "@/features/alelo/AleloVisual";
import {
  useAddAleloPurchase,
  useAleloCards,
  useAleloMonth,
  useConfirmAleloCredit,
  useCreateAlelo,
  useDeleteAlelo,
  useDeleteAleloPurchase,
  useUndoAleloCredit,
  useUpdateAlelo,
  type AleloCard,
  type AleloMonth,
} from "@/features/alelo/api";
import { formatDate, formatDayMonth, monthName, monthParam, todayISO } from "@/lib/format";
import { haptic } from "@/lib/haptics";
import { fade, spring } from "@/lib/motion";
import { useMonth } from "@/lib/month";
import { cn } from "@/lib/utils";

/** Alelo screen: balance carried over, monthly credit and purchases of each card (RF-31 to RF-34). The balance goes
 * only to the holder; the full month, with credit and every space, stays in the personal space of the holder (RF-83). */
export function AleloPage() {
  const cards = useAleloCards();
  const list = cards.data ?? [];
  const [selectedId, setSelectedId] = useState<number | undefined>();
  const selected = list.find((c) => c.id === selectedId) ?? list[0];
  const [form, setForm] = useState<AleloCard | "new" | null>(null);
  const [buying, setBuying] = useState(false);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between gap-3">
        <h1 className="px-1 text-lg font-semibold md:hidden">Alelo</h1>
        <Button size="sm" className="ml-auto" variant="outline" onClick={() => setForm("new")}>
          <Plus />
          Cartão Alelo
        </Button>
      </div>

      {cards.isSuccess && list.length === 0 && (
        <Card className="flex flex-col items-center gap-3 py-10 text-center">
          <span className="grid size-14 place-items-center rounded-2xl bg-success/10">
            <Utensils className="size-7 text-success" />
          </span>
          <p className="max-w-xs text-sm text-muted-foreground">Cadastre os cartões Alelo com o crédito mensal e o saldo de hoje. O que sobra passa para o mês seguinte.</p>
          <Button onClick={() => setForm("new")}>
            <Plus />
            Cadastrar cartão Alelo
          </Button>
        </Card>
      )}

      {list.length > 0 && (
        <div role="listbox" aria-label="Cartões Alelo" className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
          {list.map((c) => {
            const active = c.id === selected?.id;
            if (c.style === "tudo")
              return (
                <motion.button
                  key={c.id}
                  type="button"
                  role="option"
                  aria-selected={active}
                  aria-label={c.name}
                  onClick={() => {
                    haptic();
                    setSelectedId(c.id);
                  }}
                  animate={{ scale: active ? 1 : 0.97, opacity: c.active ? 1 : 0.6 }}
                  transition={spring}
                  className="flex flex-col gap-2 text-left"
                >
                  <AleloTudoVisual personName={c.personName} name={c.name} className={cn(active && "ring-3 ring-primary/40")} />
                  <span className="flex items-baseline justify-between gap-2 px-1">
                    <span className="text-xs text-muted-foreground">Saldo</span>
                    {c.balanceCents !== undefined ? <MoneyFlow cents={c.balanceCents} className="text-base font-semibold" /> : <span className="text-xs text-muted-foreground">só para quem é titular</span>}
                  </span>
                </motion.button>
              );
            return (
              <motion.button
                key={c.id}
                type="button"
                role="option"
                aria-selected={active}
                aria-label={c.name}
                onClick={() => {
                  haptic();
                  setSelectedId(c.id);
                }}
                animate={{ scale: active ? 1 : 0.97, opacity: c.active ? 1 : 0.6 }}
                transition={spring}
                className={cn(
                  "flex flex-col gap-2 rounded-2xl p-4 text-left text-white shadow-lg",
                  "bg-[linear-gradient(135deg,#1f9d61_0%,#0f6b40_60%,#1c8c56_100%)]",
                  active && "ring-3 ring-primary/40",
                )}
              >
                <span className="flex items-center justify-between gap-2 text-xs font-medium opacity-90">
                  <span className="truncate">{c.name}</span>
                  <Utensils className="size-4 shrink-0" />
                </span>
                <span className="text-[0.7rem] uppercase tracking-wider opacity-70">Saldo</span>
                {c.balanceCents !== undefined ? <MoneyFlow cents={c.balanceCents} className="text-xl font-semibold" /> : <span className="text-xs opacity-80">só para quem é titular</span>}
                <span className="truncate text-xs opacity-80">{c.personName}</span>
              </motion.button>
            );
          })}
        </div>
      )}

      {selected && <MonthCard card={selected} onBuy={() => setBuying(true)} onEdit={() => setForm(selected)} />}

      <AleloFormDialog state={form} onClose={() => setForm(null)} />
      {selected && <PurchaseDialog open={buying} card={selected} onClose={() => setBuying(false)} />}
    </div>
  );
}

/** The selected month of a card: balance carried in, credit, purchases and balance carried out (RN-15). A group reads
 * only its purchases; the holder reads the full month in the personal space (RF-83). */
function MonthCard({ card, onBuy, onEdit }: { card: AleloCard; onBuy: () => void; onEdit: () => void }) {
  const { year, month } = useMonth();
  const param = monthParam(year, month);
  const view = useAleloMonth(card.id, param);
  const confirm = useConfirmAleloCredit();
  const undo = useUndoAleloCredit();
  const remove = useDeleteAleloPurchase();
  const me = useMe();
  const [moving, setMoving] = useState<AleloMonth["purchases"][number] | null>(null);
  const v = view.data;
  /** Tells a purchase of the active space, the only ones changed here; the full month also shows the other spaces. */
  const mine = (p: AleloMonth["purchases"][number]) => !v?.full || p.groupId === me?.group.id;
  return (
    <Card className="flex flex-col gap-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <CardTitle>
            {card.name} · {monthName(year, month).toLowerCase()}
          </CardTitle>
          {card.owned && <CardDescription>Crédito de {<Money cents={card.monthlyCreditCents} />} todo dia {card.creditDay}.</CardDescription>}
        </div>
        <Button variant="ghost" size="icon-sm" aria-label={`Editar ${card.name}`} onClick={onEdit}>
          <Pencil />
        </Button>
      </div>

      {v && !v.full && (
        <div className="flex flex-col gap-1 rounded-xl bg-muted p-3 text-sm">
          <span className="flex items-baseline justify-between gap-3">
            <span className="text-xs text-muted-foreground">Compras deste espaço</span>
            <Money cents={-v.spentCents} className="font-medium" />
          </span>
          <span className="text-xs text-muted-foreground">
            {card.owned ? "O saldo, o crédito e as compras de todos os espaços ficam no seu Pessoal." : `O saldo e o crédito ficam com ${card.personName}, no Pessoal.`}
          </span>
        </div>
      )}

      {v && v.full && v.openingCents !== undefined && v.closingCents !== undefined && (
        <>
          <dl className="grid grid-cols-2 gap-2 rounded-xl bg-muted p-3 text-sm sm:grid-cols-4">
            <div>
              <dt className="text-xs text-muted-foreground">Veio do mês anterior</dt>
              <dd className="font-medium">
                <Money cents={v.openingCents} />
              </dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">Crédito</dt>
              <dd className={cn("font-medium", v.creditConfirmed && "text-success")}>
                <Money cents={v.creditCents} signed />
              </dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">Compras</dt>
              <dd className="font-medium">
                <Money cents={-v.spentCents} />
              </dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">Fica para o próximo</dt>
              <dd className={cn("font-semibold", v.closingCents < 0 && "text-destructive")}>
                <Money cents={v.closingCents} />
              </dd>
            </div>
          </dl>
        </>
      )}

      {v && (
        <>

          {v.hasCredit &&
            (v.creditConfirmed ? (
              <div className="flex items-center justify-between gap-3 rounded-xl bg-success/10 px-3 py-2 text-sm">
                <span className="flex items-center gap-2 text-success">
                  <Check className="size-4 shrink-0" />
                  Crédito recebido em {v.creditedOn ? formatDate(v.creditedOn) : ""}
                </span>
                <Button variant="ghost" size="sm" disabled={undo.isPending} onClick={() => undo.mutate({ id: card.id, month: param })}>
                  <Undo2 />
                  Desfazer
                </Button>
              </div>
            ) : (
              <div className="flex items-center justify-between gap-3 rounded-xl border border-dashed px-3 py-2 text-sm">
                <span className="text-muted-foreground">
                  Crédito de <Money cents={v.expectedCreditCents ?? 0} /> previsto para {v.expectedCreditOn ? formatDayMonth(v.expectedCreditOn) : ""}
                </span>
                <Button size="sm" disabled={confirm.isPending} onClick={() => confirm.mutate({ id: card.id, month: param, body: {} })}>
                  <Check />
                  Caiu
                </Button>
              </div>
            ))}
          <FieldError message={confirm.error?.message ?? undo.error?.message} />

          <div className="flex items-center justify-between gap-2">
            <CardTitle>Compras do mês</CardTitle>
            {card.active && (
              <Button size="sm" onClick={onBuy}>
                <Plus />
                Compra
              </Button>
            )}
          </div>
          <ul className="flex flex-col">
            <AnimatePresence initial={false}>
              {v.purchases.map((p) => (
                <motion.li
                  key={p.id}
                  layout
                  initial={{ opacity: 0, y: -6 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0, height: 0 }}
                  transition={{ ...spring, opacity: fade }}
                  className="flex items-center gap-3 border-b py-2.5 last:border-b-0"
                >
                  <span className="w-10 shrink-0 text-xs text-muted-foreground">{formatDayMonth(p.date)}</span>
                  <span className="grid size-9 shrink-0 place-items-center rounded-xl bg-muted">
                    <ShoppingBasket className="size-4 text-success" />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-medium">{p.description}</span>
                    <span className="block truncate text-xs text-muted-foreground">
                      Compras para casa{p.createdByName ? ` · por ${p.createdByName}` : ""}
                      {v.full && p.groupName ? ` · ${p.groupName}` : ""}
                    </span>
                  </span>
                  <Money cents={-p.amountCents} className="text-sm font-medium" />
                  {mine(p) && (
                    <>
                      <Button variant="ghost" size="icon-sm" aria-label={`Mover ${p.description}`} onClick={() => setMoving(p)} className="text-muted-foreground">
                        <ArrowRightLeft />
                      </Button>
                      <Button variant="ghost" size="icon-sm" aria-label={`Excluir ${p.description}`} disabled={remove.isPending} onClick={() => remove.mutate(p.id)} className="text-muted-foreground">
                        <Trash2 />
                      </Button>
                    </>
                  )}
                </motion.li>
              ))}
            </AnimatePresence>
          </ul>
          {v.purchases.length === 0 && <p className="py-2 text-center text-sm text-muted-foreground">Nenhuma compra neste mês.</p>}
        </>
      )}
      <Dialog open={moving !== null} onOpenChange={(o) => !o && setMoving(null)} title={moving ? `Mover ${moving.description}` : "Mover compra"} description="A compra sai deste espaço; a divisão entre membros fica para trás.">
        {moving && <MoveToSpace kind="alelo" itemId={moving.id} onMoved={() => setMoving(null)} />}
        <DialogActions>
          <Button variant="ghost" onClick={() => setMoving(null)}>
            Fechar
          </Button>
        </DialogActions>
      </Dialog>
    </Card>
  );
}

const aleloSchema = z.object({
  name: z.string().trim().min(1, "Informe o nome.").max(40, "Use no máximo 40 caracteres."),
  personId: z.number().int().positive("Escolha o titular."),
  monthlyCreditCents: z.number().int().positive("Informe o crédito mensal."),
  creditDay: z.number({ message: "Informe o dia." }).int().min(1, "De 1 a 31.").max(31, "De 1 a 31."),
  initialBalanceCents: z.number().int().min(0),
  style: z.enum(["classic", "tudo"]),
  active: z.boolean(),
});

/** Registers an Alelo card with its balance today, or edits one (RF-31). */
function AleloFormDialog({ state, onClose }: { state: AleloCard | "new" | null; onClose: () => void }) {
  const persons = usePersons();
  const create = useCreateAlelo();
  const update = useUpdateAlelo();
  const remove = useDeleteAlelo();
  const editing = state !== null && state !== "new" ? state : undefined;
  const mutation = editing ? update : create;
  const form = useForm<z.infer<typeof aleloSchema>>({
    resolver: zodResolver(aleloSchema),
    resetOptions: { keepDirtyValues: true },
    values: {
      name: editing?.name ?? "",
      personId: editing?.personId ?? 0,
      monthlyCreditCents: editing?.monthlyCreditCents ?? 0,
      creditDay: editing?.creditDay ?? 1,
      initialBalanceCents: 0,
      style: editing?.style ?? "tudo",
      active: editing?.active ?? true,
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

  /** Validates and sends the form; the starting balance is only set on registration. */
  const submit = form.handleSubmit(({ initialBalanceCents, active, ...v }) => {
    if (editing) update.mutate({ id: editing.id, body: { ...v, active } }, { onSuccess: close });
    else create.mutate({ ...v, initialBalanceCents }, { onSuccess: close });
  });

  return (
    <Dialog open={state !== null} onOpenChange={(o) => !o && close()} title={editing ? "Editar cartão Alelo" : "Novo cartão Alelo"}>
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Nome" htmlFor="alelo-name" error={errors.name?.message}>
            <Input id="alelo-name" autoComplete="off" placeholder="Alelo Ana" {...form.register("name")} />
          </Field>
          <Field label="Titular" htmlFor="alelo-person" error={errors.personId?.message}>
            <NativeSelect id="alelo-person" {...form.register("personId", { valueAsNumber: true })}>
              <option value={0}>Escolha</option>
              {(persons.data ?? []).map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </NativeSelect>
          </Field>
        </div>
        <div className="grid grid-cols-3 gap-3">
          <div className="col-span-2">
            <Field label="Crédito mensal" htmlFor="alelo-credit" error={errors.monthlyCreditCents?.message}>
              <Controller control={form.control} name="monthlyCreditCents" render={({ field }) => <MoneyInput id="alelo-credit" value={field.value} onChange={field.onChange} />} />
            </Field>
          </div>
          <Field label="Cai dia" htmlFor="alelo-day" error={errors.creditDay?.message}>
            <Input id="alelo-day" type="number" inputMode="numeric" min={1} max={31} {...form.register("creditDay", { valueAsNumber: true })} />
          </Field>
        </div>
        <div className="flex flex-col gap-1.5">
          <span className="text-sm font-medium">Visual</span>
          <Controller
            control={form.control}
            name="style"
            render={({ field }) => (
              <Segmented<AleloCard["style"]>
                ariaLabel="Visual"
                value={field.value}
                options={[
                  { value: "tudo", label: "Alelo Tudo" },
                  { value: "classic", label: "Clássico" },
                ]}
                onChange={field.onChange}
              />
            )}
          />
        </div>
        {!editing && (
          <Field label="Saldo hoje" htmlFor="alelo-balance">
            <Controller control={form.control} name="initialBalanceCents" render={({ field }) => <MoneyInput id="alelo-balance" value={field.value} onChange={field.onChange} />} />
          </Field>
        )}
        {editing && (
          <label className="flex items-center justify-between gap-3 text-sm">
            Ativo
            <Controller control={form.control} name="active" render={({ field }) => <Switch checked={field.value} onCheckedChange={field.onChange} />} />
          </label>
        )}
        <FieldError message={mutation.error?.message ?? remove.error?.message} />
        <DialogActions className="sm:justify-between">
          {editing ? (
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
              Salvar
            </Button>
          </span>
        </DialogActions>
      </form>
    </Dialog>
  );
}

const purchaseSchema = z.object({
  description: z.string().trim().min(1, "Descreva a compra.").max(120, "Use no máximo 120 caracteres."),
  amountCents: z.number().int().positive("Informe o valor."),
  date: z.string().regex(/^\d{4}-\d{2}-\d{2}$/, "Informe a data."),
});

/** Records a purchase with a free description; it counts as Alimentação, Compras para casa (RF-33, RN-15). */
function PurchaseDialog({ open, card, onClose }: { open: boolean; card: AleloCard; onClose: () => void }) {
  const add = useAddAleloPurchase();
  const split = useSplitState("alelo");
  const where = useSpaceChoice();
  const form = useForm<z.infer<typeof purchaseSchema>>({ resolver: zodResolver(purchaseSchema), resetOptions: { keepDirtyValues: true }, values: { description: "", amountCents: 0, date: todayISO() } });
  const errors = form.formState.errors;

  /** Clears the mutation state and closes. */
  const close = () => {
    add.reset();
    split.reset();
    where.setSpace(undefined);
    form.reset();
    onClose();
  };

  /** Validates and sends the form. */
  const submit = form.handleSubmit((v) => add.mutate({ id: card.id, body: { ...v, memberSplit: split.payload(where.space) }, space: where.target }, { onSuccess: close }));

  return (
    <Dialog open={open} onOpenChange={(o) => !o && close()} title={`Compra no ${card.name}`} description="Entra no gasto como Alimentação, Compras para casa.">
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <Field label="Descrição" htmlFor="alelo-desc" error={errors.description?.message}>
          <Input id="alelo-desc" autoComplete="off" placeholder="Mercado, padaria, feira..." {...form.register("description")} />
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Valor" htmlFor="alelo-amount" error={errors.amountCents?.message}>
            <Controller control={form.control} name="amountCents" render={({ field }) => <MoneyInput id="alelo-amount" value={field.value} onChange={field.onChange} />} />
          </Field>
          <Field label="Data" htmlFor="alelo-date" error={errors.date?.message}>
            <Input id="alelo-date" type="date" {...form.register("date")} />
          </Field>
        </div>
        <SpaceField value={where.space} onChange={where.setSpace} />
        <MemberSplitField value={split.value} onChange={split.onChange} space={where.space} />
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
