import NumberFlow from "@number-flow/react";
import { zodResolver } from "@hookform/resolvers/zod";
import { AnimatePresence, LayoutGroup, motion } from "motion/react";
import { ArrowDownLeft, ArrowUpRight, CreditCard, Landmark, Minus, Plus, Repeat, Shuffle, Utensils } from "lucide-react";
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Controller, useForm, useWatch, type Control, type UseFormReturn } from "react-hook-form";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Field, FieldError } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { NativeSelect } from "@/components/ui/native-select";
import { Segmented } from "@/components/ui/segmented";
import { useAccounts, useAddExpense, usePersons } from "@/features/accounts/api";
import { useAddAleloPurchase, useAleloCards } from "@/features/alelo/api";
import { useCreateBill } from "@/features/bills/api";
import { useAddPurchase, useAddRecurrence, useCards } from "@/features/cards/api";
import { EMPTY_SPLIT, SplitEditor, splitBody, type SplitValue } from "@/features/cards/SplitEditor";
import { useCategories, type Category } from "@/features/categories/api";
import { CategoryIcon } from "@/features/categories/icons";
import { useAddOneOffIncome, useCreateIncome } from "@/features/incomes/api";
import { todayISO } from "@/lib/format";
import { haptic } from "@/lib/haptics";
import { fade, spring } from "@/lib/motion";
import { cn, readStorage, writeStorage } from "@/lib/utils";

type Nature = "expense" | "income";
type Method = "credit" | "cash" | "alelo";
type Kind = "variable" | "fixed";

/** How the card opens, set by the shortcut that was tapped. */
export type QuickEntryPreset = { nature: Nature; method?: Method };

/** Keys of the last card and account used, a per device convenience (RNF-13). */
const LAST_CARD_KEY = "radar.quick.card";
const LAST_ACCOUNT_KEY = "radar.quick.account";
const LAST_ALELO_KEY = "radar.quick.alelo";

/** Largest number of digits typed in the value, enough for billions in cents. */
const MAX_DIGITS = 12;

const QuickEntryContext = createContext<{ open: (preset: QuickEntryPreset) => void } | null>(null);

/** Holds the quick entry card and lets any shortcut open it (section 5.6). */
export function QuickEntryProvider({ children }: { children: ReactNode }) {
  const [preset, setPreset] = useState<QuickEntryPreset | null>(null);
  // A new key per opening resets the form to the preset.
  const [session, setSession] = useState(0);
  const keyboardProxy = useRef<HTMLInputElement>(null);
  const open = useCallback((p: QuickEntryPreset) => {
    // iOS only opens the keyboard for a focus made inside the tap; the value field takes it over on mount (RNF-13).
    keyboardProxy.current?.focus({ preventScroll: true });
    setPreset(p);
    setSession((s) => s + 1);
  }, []);
  const value = useMemo(() => ({ open }), [open]);
  return (
    <QuickEntryContext.Provider value={value}>
      {children}
      <input
        ref={keyboardProxy}
        aria-hidden
        tabIndex={-1}
        inputMode="numeric"
        className="pointer-events-none fixed top-0 left-0 h-px w-px opacity-0"
      />
      {preset && <QuickEntrySheet key={session} preset={preset} onClose={() => setPreset(null)} />}
    </QuickEntryContext.Provider>
  );
}

/** Returns the function that opens the quick entry card. */
export function useQuickEntry() {
  const ctx = useContext(QuickEntryContext);
  if (!ctx) throw new Error("useQuickEntry must be used inside QuickEntryProvider");
  return ctx;
}

const schema = z
  .object({
    nature: z.enum(["expense", "income"]),
    method: z.enum(["credit", "cash", "alelo"]),
    kind: z.enum(["variable", "fixed"]),
    amountCents: z.number().int(),
    categoryId: z.number().int(),
    cardId: z.number().int(),
    accountId: z.number().int(),
    aleloId: z.number().int(),
    installments: z.number().int().min(1).max(48),
    merchant: z.string().trim().max(80, "Use no máximo 80 caracteres."),
    description: z.string().trim().max(80, "Use no máximo 80 caracteres."),
    buyerPersonId: z.number().int(),
    date: z.string(),
    day: z.number().int(),
    personId: z.number().int(),
    endOn: z.string(),
    split: z.custom<SplitValue>(),
  })
  .superRefine((v, ctx) => {
    /** Adds an error message to a field. */
    const fail = (path: string, message: string) => ctx.addIssue({ code: "custom", path: [path], message });
    if (v.amountCents <= 0) fail("amountCents", "Digite o valor.");
    if (!/^\d{4}-\d{2}-\d{2}$/.test(v.date)) fail("date", "Informe a data.");
    const combo = comboOf(v);
    if (v.nature === "expense" && combo !== "alelo" && v.categoryId <= 0) fail("categoryId", "Escolha a categoria.");
    if (combo === "purchase" || combo === "recurrence") {
      if (v.cardId <= 0) fail("cardId", "Escolha o cartão.");
    } else if (combo === "alelo") {
      if (v.aleloId <= 0) fail("aleloId", "Escolha o cartão Alelo.");
      if (!v.description) fail("description", "Descreva a compra.");
    } else if (v.accountId <= 0) fail("accountId", "Escolha a conta.");
    if (combo === "purchase" && !v.merchant) fail("merchant", "Informe o estabelecimento.");
    if ((combo === "recurrence" || combo === "recurringIncome" || combo === "bill") && !v.description) fail("description", "Informe a descrição.");
    if (combo === "bill" && (v.day < 1 || v.day > 31)) fail("day", "De 1 a 31.");
    if (combo === "recurringIncome") {
      if (v.day < 1 || v.day > 31) fail("day", "De 1 a 31.");
      if (v.personId <= 0) fail("personId", "Escolha de quem é.");
    }
  });
type Values = z.infer<typeof schema>;

/** What the chosen combination becomes in the system (section 5.6). */
type Combo = "purchase" | "recurrence" | "expense" | "bill" | "alelo" | "oneOffIncome" | "recurringIncome";

/** Resolves the combination of nature, payment method and kind. */
function comboOf(v: Pick<Values, "nature" | "method" | "kind">): Combo {
  if (v.nature === "income") return v.kind === "fixed" ? "recurringIncome" : "oneOffIncome";
  if (v.method === "alelo") return "alelo";
  if (v.method === "credit") return v.kind === "fixed" ? "recurrence" : "purchase";
  return v.kind === "fixed" ? "bill" : "expense";
}

/** Bottom sheet with nature, payment method, kind, value, category, details and confirm (section 5.6). */
function QuickEntrySheet({ preset, onClose }: { preset: QuickEntryPreset; onClose: () => void }) {
  const cards = useCards();
  const accounts = useAccounts();
  const alelos = useAleloCards();
  const [open, setOpen] = useState(true);
  const [done, setDone] = useState(false);
  const activeCards = (cards.data ?? []).filter((c) => c.active);
  const activeAccounts = (accounts.data ?? []).filter((a) => a.active);
  const lastCard = Number(readStorage(LAST_CARD_KEY));
  const lastAccount = Number(readStorage(LAST_ACCOUNT_KEY));
  const activeAlelos = (alelos.data ?? []).filter((a) => a.active);
  const lastAlelo = Number(readStorage(LAST_ALELO_KEY));
  const aleloId = activeAlelos.some((a) => a.id === lastAlelo) ? lastAlelo : (activeAlelos[0]?.id ?? 0);

  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: {
      nature: preset.nature,
      method: preset.method ?? "credit",
      kind: "variable",
      amountCents: 0,
      categoryId: 0,
      cardId: 0,
      accountId: 0,
      aleloId: 0,
      installments: 1,
      merchant: "",
      description: "",
      buyerPersonId: 0,
      date: todayISO(),
      day: Number(todayISO().slice(8)),
      personId: 0,
      endOn: "",
      split: EMPTY_SPLIT,
    },
  });
  // The last card and account used come preselected once the lists load (RNF-13).
  const cardId = activeCards.some((c) => c.id === lastCard) ? lastCard : (activeCards[0]?.id ?? 0);
  const accountId = activeAccounts.some((a) => a.id === lastAccount) ? lastAccount : (activeAccounts[0]?.id ?? 0);
  const values = useWatch({ control: form.control });

  // Puts the preselected card and account in the form once the lists load (RNF-13).
  useEffect(() => {
    if (cardId && form.getValues("cardId") === 0) form.setValue("cardId", cardId);
    if (accountId && form.getValues("accountId") === 0) form.setValue("accountId", accountId);
    if (aleloId && form.getValues("aleloId") === 0) form.setValue("aleloId", aleloId);
  }, [cardId, accountId, aleloId, form]);
  const combo = comboOf({ nature: values.nature ?? "expense", method: values.method ?? "credit", kind: values.kind ?? "variable" });

  const purchase = useAddPurchase();
  const recurrence = useAddRecurrence();
  const expense = useAddExpense();
  const oneOff = useAddOneOffIncome();
  const recurring = useCreateIncome();
  const bill = useCreateBill();
  const aleloPurchase = useAddAleloPurchase();
  const mutation = { purchase, recurrence, expense, bill, alelo: aleloPurchase, oneOffIncome: oneOff, recurringIncome: recurring }[combo];

  /** Moves the focus to the next text field after the subcategory, saving a tap (RNF-13). */
  const focusNextField = () => {
    if (combo === "purchase" && !form.getValues("merchant")) form.setFocus("merchant");
    else if ((combo === "recurrence" || combo === "bill") && !form.getValues("description")) form.setFocus("description");
  };

  /** Plays the closing animation and then unmounts the sheet. */
  const close = () => setOpen(false);

  /** Shows the animated check, then closes. */
  const finish = () => {
    setDone(true);
    window.setTimeout(close, 650);
  };

  /** Validates and sends the entry as the system object of its combination. */
  const submit = form.handleSubmit((raw) => {
    const v = { ...raw, cardId: raw.cardId || cardId, accountId: raw.accountId || accountId, aleloId: raw.aleloId || aleloId };
    if (combo === "purchase" || combo === "recurrence") writeStorage(LAST_CARD_KEY, String(v.cardId));
    else if (combo === "alelo") writeStorage(LAST_ALELO_KEY, String(v.aleloId));
    else writeStorage(LAST_ACCOUNT_KEY, String(v.accountId));
    const done = { onSuccess: finish };
    switch (combo) {
      case "purchase":
        purchase.mutate(
          {
            cardId: v.cardId,
            body: {
              date: v.date,
              merchant: v.merchant,
              categoryId: v.categoryId,
              amountCents: v.amountCents,
              installments: v.installments,
              buyerPersonId: v.buyerPersonId || undefined,
              split: splitBody(v.split),
            },
          },
          done,
        );
        break;
      case "recurrence":
        recurrence.mutate({ cardId: v.cardId, body: { description: v.description, categoryId: v.categoryId, amountCents: v.amountCents, startDate: v.date } }, done);
        break;
      case "expense":
        expense.mutate({ id: v.accountId, body: { amountCents: v.amountCents, date: v.date, categoryId: v.categoryId, description: v.description } }, done);
        break;
      case "alelo":
        aleloPurchase.mutate({ id: v.aleloId, body: { date: v.date, description: v.description, amountCents: v.amountCents } }, done);
        break;
      case "bill":
        bill.mutate(
          {
            name: v.description,
            categoryId: v.categoryId,
            accountId: v.accountId,
            dueDay: v.day,
            expectedCents: v.amountCents,
            startMonth: v.date.slice(0, 7),
          },
          done,
        );
        break;
      case "oneOffIncome":
        oneOff.mutate({ accountId: v.accountId, body: { amountCents: v.amountCents, date: v.date, description: v.description } }, done);
        break;
      case "recurringIncome":
        recurring.mutate(
          {
            kind: "recurring",
            name: v.description,
            personId: v.personId,
            accountId: v.accountId,
            amountCents: v.amountCents,
            day: v.day,
            startMonth: v.date.slice(0, 7),
            endOn: v.endOn || undefined,
          },
          done,
        );
        break;
    }
  });

  const errors = form.formState.errors;
  const isExpense = values.nature === "expense";
  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o) close();
      }}
      title={isExpense ? "Novo gasto" : "Nova receita"}
      className="md:max-w-lg"
    >
      <ExitAfterClose open={open} onExited={onClose} />
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <Segmented<Nature>
          ariaLabel="Natureza"
          value={values.nature ?? "expense"}
          options={[
            { value: "expense", label: "Gasto", icon: <ArrowUpRight className="size-4" /> },
            { value: "income", label: "Receita", icon: <ArrowDownLeft className="size-4" /> },
          ]}
          onChange={(v) => {
            haptic();
            form.setValue("nature", v);
          }}
          className="w-full"
        />
        <LayoutGroup>
          <AnimatePresence initial={false}>
            {isExpense && (
              <Reveal key="method">
                <Segmented<Method>
                  ariaLabel="Forma de pagamento"
                  value={values.method ?? "credit"}
                  options={[
                    { value: "credit", label: "Crédito", icon: <CreditCard className="size-4" /> },
                    { value: "cash", label: "Dinheiro", icon: <Landmark className="size-4" /> },
                    { value: "alelo", label: "Alelo", icon: <Utensils className="size-4" /> },
                  ]}
                  onChange={(v) => {
                    haptic();
                    form.setValue("method", v);
                  }}
                  className="w-full"
                />
              </Reveal>
            )}
            {!(isExpense && values.method === "alelo") && (
              <Reveal key="kind">
                <Segmented<Kind>
                  ariaLabel="Tipo"
                  value={values.kind ?? "variable"}
                  options={[
                    { value: "variable", label: "Variável", icon: <Shuffle className="size-4" /> },
                    { value: "fixed", label: "Fixo", icon: <Repeat className="size-4" /> },
                  ]}
                  onChange={(v) => {
                    haptic();
                    form.setValue("kind", v);
                  }}
                  className="w-full"
                />
              </Reveal>
            )}
          </AnimatePresence>

          <motion.div layout transition={spring}>
            <Controller control={form.control} name="amountCents" render={({ field }) => <AmountInput value={field.value} onChange={field.onChange} income={!isExpense} />} />
            <FieldError message={errors.amountCents?.message} />
          </motion.div>

          <AnimatePresence initial={false}>
            {isExpense && values.method !== "alelo" && (
              <Reveal key="category">
                <Controller
                  control={form.control}
                  name="categoryId"
                  render={({ field }) => (
                    <CategoryPicker
                      value={field.value}
                      onChange={(id) => {
                        field.onChange(id);
                        focusNextField();
                      }}
                    />
                  )}
                />
                <FieldError message={errors.categoryId?.message} />
              </Reveal>
            )}
          </AnimatePresence>

          <motion.div layout transition={spring} className="flex flex-col gap-3">
            <Details combo={combo} form={form} />
          </motion.div>
        </LayoutGroup>

        <FieldError message={mutation.error?.message} />
        <ConfirmButton done={done} pending={mutation.isPending} />
      </form>
    </Dialog>
  );
}

/** Calls onExited once the dialog finished closing, so its exit animation plays. */
function ExitAfterClose({ open, onExited }: { open: boolean; onExited: () => void }) {
  useEffect(() => {
    if (open) return;
    const t = window.setTimeout(onExited, 220);
    return () => window.clearTimeout(t);
  }, [open, onExited]);
  return null;
}

/** Block that grows into place when its choice makes it visible. */
function Reveal({ children }: { children: ReactNode }) {
  return (
    <motion.div
      layout
      initial={{ opacity: 0, height: 0 }}
      animate={{ opacity: 1, height: "auto" }}
      exit={{ opacity: 0, height: 0 }}
      transition={{ height: spring, opacity: fade, layout: spring }}
      className="overflow-hidden"
    >
      {children}
    </motion.div>
  );
}

/** Big centered value typed on the iPhone number pad, each digit rolling into place (section 5.6, 8.6). */
function AmountInput({ value, onChange, income }: { value: number; onChange: (cents: number) => void; income: boolean }) {
  return (
    <label className="relative flex cursor-text flex-col items-center py-3">
      <span className="sr-only">Valor</span>
      <NumberFlow
        value={value / 100}
        locales="pt-BR"
        format={{ style: "currency", currency: "BRL" }}
        className={cn("money-flow text-5xl font-semibold tracking-tight tabular-nums", value === 0 && "text-muted-foreground", income && value > 0 && "text-success")}
      />
      <input
        aria-label="Valor"
        inputMode="numeric"
        autoComplete="off"
        autoFocus
        className="absolute inset-0 h-full w-full cursor-text opacity-0"
        value={value === 0 ? "" : String(value)}
        onChange={(e) => {
          const digits = e.target.value.replace(/\D/g, "").replace(/^0+/, "").slice(0, MAX_DIGITS);
          onChange(digits === "" ? 0 : Number(digits));
        }}
      />
    </label>
  );
}

/** Grid of category icons; tapping one expands its subcategories, tapping a subcategory picks it (section 5.6). */
function CategoryPicker({ value, onChange }: { value: number; onChange: (id: number) => void }) {
  const categories = useCategories();
  const tree = categories.data ?? [];
  const selectedParent = tree.find((c) => c.children?.some((s) => s.id === value));
  const [openId, setOpenId] = useState<number | undefined>(selectedParent?.id);
  const open = tree.find((c) => c.id === openId);
  return (
    <div className="flex flex-col gap-2">
      <div role="radiogroup" aria-label="Categoria" className="grid grid-cols-4 gap-2 sm:grid-cols-5">
        {tree.map((c) => (
          <CategoryTile
            key={c.id}
            category={c}
            active={c.id === openId}
            picked={c.id === selectedParent?.id}
            onClick={() => {
              haptic();
              setOpenId(c.id === openId ? undefined : c.id);
            }}
          />
        ))}
      </div>
      <AnimatePresence initial={false} mode="popLayout">
        {open && (
          <motion.div
            key={open.id}
            layout
            initial={{ opacity: 0, y: -6, height: 0 }}
            animate={{ opacity: 1, y: 0, height: "auto" }}
            exit={{ opacity: 0, height: 0 }}
            transition={{ ...spring, opacity: fade }}
            className="overflow-hidden"
          >
            <div role="radiogroup" aria-label={`Subcategorias de ${open.name}`} className="flex flex-wrap gap-2 rounded-2xl bg-muted p-2">
              {(open.children ?? []).map((s) => {
                const active = s.id === value;
                return (
                  <button
                    key={s.id}
                    type="button"
                    role="radio"
                    aria-checked={active}
                    onClick={() => {
                      haptic();
                      onChange(s.id);
                    }}
                    className={cn("relative rounded-full px-3 py-1.5 text-sm transition-colors", active ? "text-primary-foreground" : "bg-card")}
                  >
                    {active && <motion.span layoutId="quick-sub" transition={spring} className="absolute inset-0 rounded-full bg-primary" />}
                    <span className="relative">{s.name}</span>
                  </button>
                );
              })}
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}

/** One top level category of the grid. */
function CategoryTile({ category, active, picked, onClick }: { category: Category; active: boolean; picked: boolean; onClick: () => void }) {
  return (
    <button
      type="button"
      role="radio"
      aria-checked={picked}
      aria-expanded={active}
      onClick={onClick}
      className={cn(
        "flex flex-col items-center gap-1 rounded-2xl p-2 text-[0.7rem] leading-tight transition-colors",
        active ? "bg-primary/10 text-primary" : picked ? "bg-primary/5 text-primary" : "hover:bg-accent",
      )}
    >
      <span className={cn("grid size-10 place-items-center rounded-xl", active || picked ? "bg-primary text-primary-foreground" : "bg-muted text-primary")}>
        <CategoryIcon name={category.icon} className="size-5" />
      </span>
      <span className="line-clamp-2 text-center">{category.name}</span>
    </button>
  );
}

/** Fields that change with the combination (table of section 5.6). */
function Details({ combo, form }: { combo: Combo; form: UseFormReturn<Values> }) {
  const cards = useCards();
  const accounts = useAccounts();
  const persons = usePersons();
  const alelos = useAleloCards();
  const errors = form.formState.errors;
  const reg = form.register;
  const activeCards = (cards.data ?? []).filter((c) => c.active);
  const activeAccounts = (accounts.data ?? []).filter((a) => a.active);
  const cardField = (
    <Field label="Cartão" htmlFor="quick-card" error={errors.cardId?.message}>
      <NativeSelect id="quick-card" {...reg("cardId", { valueAsNumber: true })}>
        {activeCards.length === 0 && <option value={0}>Nenhum cartão cadastrado</option>}
        {activeCards.map((c) => (
          <option key={c.id} value={c.id}>
            {c.name}
          </option>
        ))}
      </NativeSelect>
    </Field>
  );
  const accountField = (label: string) => (
    <Field label={label} htmlFor="quick-account" error={errors.accountId?.message}>
      <NativeSelect id="quick-account" {...reg("accountId", { valueAsNumber: true })}>
        {activeAccounts.length === 0 && <option value={0}>Nenhuma conta cadastrada</option>}
        {activeAccounts.map((a) => (
          <option key={a.id} value={a.id}>
            {a.name}
          </option>
        ))}
      </NativeSelect>
    </Field>
  );
  const dateField = (label: string) => (
    <Field label={label} htmlFor="quick-date" error={errors.date?.message}>
      <Input id="quick-date" type="date" {...reg("date")} />
    </Field>
  );
  const personOptions = (persons.data ?? []).map((p) => (
    <option key={p.id} value={p.id}>
      {p.name}
    </option>
  ));

  switch (combo) {
    case "purchase":
      return (
        <>
          <div className="grid grid-cols-2 gap-3">
            {cardField}
            <Installments control={form.control} />
          </div>
          <Field label="Estabelecimento" htmlFor="quick-merchant" error={errors.merchant?.message}>
            <Input id="quick-merchant" autoComplete="off" {...reg("merchant")} />
          </Field>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Quem fez" htmlFor="quick-buyer">
              <NativeSelect id="quick-buyer" {...reg("buyerPersonId", { valueAsNumber: true })}>
                <option value={0}>Não informar</option>
                {personOptions}
              </NativeSelect>
            </Field>
            {dateField("Data")}
          </div>
          <Controller control={form.control} name="split" render={({ field }) => <SplitEditor value={field.value} onChange={field.onChange} />} />
        </>
      );
    case "recurrence":
      return (
        <>
          {cardField}
          <div className="grid grid-cols-2 gap-3">
            <Field label="Descrição" htmlFor="quick-desc" error={errors.description?.message}>
              <Input id="quick-desc" autoComplete="off" placeholder="Streaming, barbearia..." {...reg("description")} />
            </Field>
            {dateField("Início")}
          </div>
        </>
      );
    case "expense":
    case "oneOffIncome":
      return (
        <>
          <div className="grid grid-cols-2 gap-3">
            {accountField(combo === "expense" ? "Conta de origem" : "Conta de destino")}
            {dateField("Data")}
          </div>
          <Field label="Descrição" htmlFor="quick-desc" error={errors.description?.message}>
            <Input id="quick-desc" autoComplete="off" placeholder="Opcional" {...reg("description")} />
          </Field>
        </>
      );
    case "alelo":
      return (
        <>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Cartão Alelo" htmlFor="quick-alelo" error={errors.aleloId?.message}>
              <NativeSelect id="quick-alelo" {...reg("aleloId", { valueAsNumber: true })}>
                {(alelos.data ?? []).filter((a) => a.active).length === 0 && <option value={0}>Nenhum cartão Alelo</option>}
                {(alelos.data ?? [])
                  .filter((a) => a.active)
                  .map((a) => (
                    <option key={a.id} value={a.id}>
                      {a.name}
                    </option>
                  ))}
              </NativeSelect>
            </Field>
            {dateField("Data")}
          </div>
          <Field label="Descrição" htmlFor="quick-desc" error={errors.description?.message}>
            <Input id="quick-desc" autoComplete="off" placeholder="Mercado, padaria, feira..." {...reg("description")} />
          </Field>
        </>
      );
    case "bill":
      return (
        <>
          <Field label="Nome" htmlFor="quick-desc" error={errors.description?.message}>
            <Input id="quick-desc" autoComplete="off" placeholder="Energia, água, aluguel..." {...reg("description")} />
          </Field>
          <div className="grid grid-cols-3 gap-3">
            <div className="col-span-2">{accountField("Conta de pagamento")}</div>
            <Field label="Vence dia" htmlFor="quick-day" error={errors.day?.message}>
              <Input id="quick-day" type="number" inputMode="numeric" min={1} max={31} {...reg("day", { valueAsNumber: true })} />
            </Field>
          </div>
          <p className="-mt-1 text-xs text-muted-foreground">O valor acima é o previsto. Quando a conta chegar, informe o real na tela de contas fixas.</p>
        </>
      );
    case "recurringIncome":
      return (
        <>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Nome" htmlFor="quick-desc" error={errors.description?.message}>
              <Input id="quick-desc" autoComplete="off" placeholder="Salário, bolsa..." {...reg("description")} />
            </Field>
            {accountField("Conta de destino")}
          </div>
          <div className="grid grid-cols-3 gap-3">
            <Field label="Dia" htmlFor="quick-day" error={errors.day?.message}>
              <Input id="quick-day" type="number" inputMode="numeric" min={1} max={31} {...reg("day", { valueAsNumber: true })} />
            </Field>
            <div className="col-span-2">
              <Field label="Pessoa" htmlFor="quick-person" error={errors.personId?.message}>
                <NativeSelect id="quick-person" {...reg("personId", { valueAsNumber: true })}>
                  <option value={0}>Escolha</option>
                  {personOptions}
                </NativeSelect>
              </Field>
            </div>
          </div>
          <Field label="Término (opcional)" htmlFor="quick-end">
            <Input id="quick-end" type="date" {...reg("endOn")} />
          </Field>
        </>
      );
  }
}

/** Installment count with plus and minus buttons (section 5.6). */
function Installments({ control }: { control: Control<Values> }) {
  return (
    <Controller
      control={control}
      name="installments"
      render={({ field }) => (
        <div className="flex flex-col gap-1.5">
          <span className="text-sm font-medium" id="quick-installments">
            Parcelas
          </span>
          <div className="flex h-11 items-center justify-between rounded-xl border border-input px-1" role="group" aria-labelledby="quick-installments">
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label="Menos parcelas"
              disabled={field.value <= 1}
              onClick={() => {
                haptic();
                field.onChange(field.value - 1);
              }}
            >
              <Minus />
            </Button>
            <NumberFlow value={field.value} suffix="x" className="text-base font-semibold tabular-nums" aria-live="polite" />
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label="Mais parcelas"
              disabled={field.value >= 48}
              onClick={() => {
                haptic();
                field.onChange(field.value + 1);
              }}
            >
              <Plus />
            </Button>
          </div>
        </div>
      )}
    />
  );
}

/** Confirm button whose check draws itself when the entry is saved (section 5.6). */
function ConfirmButton({ done, pending }: { done: boolean; pending: boolean }) {
  return (
    <Button type="submit" size="lg" disabled={pending || done} className={cn("w-full transition-colors", done && "bg-success hover:bg-success disabled:opacity-100")}>
      <svg viewBox="0 0 24 24" className="size-5" aria-hidden>
        {/* A new key per state redraws the check from zero when the entry is saved. */}
        <motion.path
          key={done ? "done" : "idle"}
          d="M5 12.5l4.5 4.5L19 7.5"
          fill="none"
          stroke="currentColor"
          strokeWidth="2.6"
          strokeLinecap="round"
          strokeLinejoin="round"
          initial={{ pathLength: done ? 0 : 1 }}
          animate={{ pathLength: 1 }}
          transition={{ duration: 0.35, ease: "easeOut" }}
        />
      </svg>
      {done ? "Lançado" : pending ? "Lançando..." : "Confirmar"}
    </Button>
  );
}
