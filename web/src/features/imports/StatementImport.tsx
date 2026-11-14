import { AnimatePresence, motion } from "motion/react";
import { AlertTriangle, FileUp, Scale, Undo2, Upload } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";
import { useMe } from "@/app/session";
import { Button } from "@/components/ui/button";
import { Card, CardDescription, CardTitle } from "@/components/ui/card";
import { Field, FieldError } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Money } from "@/components/ui/money";
import { NativeSelect } from "@/components/ui/native-select";
import { useAccounts, useAdjustBalance } from "@/features/accounts/api";
import { useCards } from "@/features/cards/api";
import { CategorySelect } from "@/features/categories/CategorySelect";
import {
  useCommitStatement,
  usePreviewStatement,
  useStatementImports,
  useUndoStatementImport,
  type StatementPreviewLine,
  type StatementResult,
} from "@/features/imports/api";
import { formatDate, formatDayMonth, monthName, todayISO } from "@/lib/format";
import { fade, spring } from "@/lib/motion";
import { cn } from "@/lib/utils";

/** A statement line as edited by the user before confirming. */
type Draft = StatementPreviewLine & { remember: boolean };

/** A file read in the page, handed over when it turns out to be a statement. */
export type PickedFile = { name: string; content: string };

/** Import of the Santander account statement: choose the account, review each line, confirm, compare the balance (RF-86 to RF-90). */
export function StatementImport({ picked, onPick }: { picked: PickedFile | null; onPick: (file: File | undefined) => void }) {
  const accounts = useAccounts();
  const [accountId, setAccountId] = useState(0);
  const [lines, setLines] = useState<Draft[] | null>(null);
  const [closing, setClosing] = useState(0);
  const [result, setResult] = useState<StatementResult | null>(null);
  const preview = usePreviewStatement();
  const commit = useCommitStatement();
  const fileId = useId();
  const active = (accounts.data ?? []).filter((a) => a.active);
  const chosen = accountId || active.find((a) => a.owned)?.id || active[0]?.id || 0;
  const account = active.find((a) => a.id === chosen);
  const previewed = useRef<PickedFile | null>(null);

  // A new file picked in the page is previewed for the chosen account, once.
  useEffect(() => {
    if (!picked || !chosen || previewed.current === picked) return;
    previewed.current = picked;
    setResult(null);
    preview.mutate(
      { accountId: chosen, content: picked.content },
      {
        onSuccess: (data) => {
          setClosing(data.closingCents);
          setLines(data.lines.map((l) => ({ ...l, remember: false })));
        },
      },
    );
  }, [picked, chosen, preview]);

  /** Changes one line; a new name or category is remembered by default (RF-55). */
  const edit = (index: number, patch: Partial<Draft>) =>
    setLines((ls) =>
      (ls ?? []).map((l) => {
        if (l.index !== index) return l;
        const next = { ...l, ...patch };
        if ("friendlyName" in patch || "categoryId" in patch) next.remember = true;
        return next;
      }),
    );

  /** Sends the confirmed lines. */
  const confirm = () => {
    if (!lines) return;
    commit.mutate(
      {
        accountId: chosen,
        fileName: picked?.name,
        closingCents: closing,
        lines: lines.map((l) => ({
          date: l.date,
          description: l.description,
          detail: l.detail,
          merchant: l.merchant,
          amountCents: l.amountCents,
          kind: l.kind,
          categoryKey: l.categoryKey,
          friendlyName: l.friendlyName,
          categoryId: l.categoryId || undefined,
          skip: l.skip,
          remember: l.remember,
          incomeId: l.incomeId,
          incomeMonth: l.incomeMonth,
          cardId: l.cardId,
          invoiceMonth: l.invoiceMonth,
        })),
      },
      {
        onSuccess: (r) => {
          setLines(null);
          setResult(r);
        },
      },
    );
  };

  const toImport = (lines ?? []).filter((l) => !l.skip);
  return (
    <>
      <Card className="flex flex-col gap-4">
        <div>
          <CardTitle>Extrato do Santander</CardTitle>
          <CardDescription>No app ou no site do Santander, exporte o extrato da conta corrente em CSV. Os lançamentos entram na conta escolhida aqui.</CardDescription>
        </div>
        <Field label="Conta" htmlFor="statement-account">
          <NativeSelect id="statement-account" value={chosen} onChange={(e) => setAccountId(Number(e.target.value))} disabled={!!lines}>
            {active.length === 0 && <option value={0}>Nenhuma conta cadastrada</option>}
            {active.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
              </option>
            ))}
          </NativeSelect>
        </Field>
        {!lines ? (
          <label htmlFor={fileId} className={cn("flex cursor-pointer flex-col items-center gap-2 rounded-2xl border-2 border-dashed p-8 text-center transition-colors hover:bg-accent", !chosen && "pointer-events-none opacity-50")}>
            <Upload className="size-6 text-primary" />
            <span className="text-sm font-medium">{preview.isPending ? "Lendo o arquivo..." : "Escolher o arquivo CSV"}</span>
            <input
              id={fileId}
              aria-label="Arquivo CSV"
              type="file"
              accept=".csv,text/csv"
              className="sr-only"
              onChange={(e) => {
                const file = e.target.files?.[0];
                // Cleared at once, so picking the same file again, after a failed preview, still fires.
                e.target.value = "";
                previewed.current = null;
                onPick(file);
              }}
            />
          </label>
        ) : (
          <div className="flex items-center justify-between gap-2 rounded-xl bg-muted px-3 py-2 text-sm">
            <span className="flex min-w-0 items-center gap-2">
              <FileUp className="size-4 shrink-0 text-primary" />
              <span className="truncate">{picked?.name}</span>
            </span>
            <Button variant="ghost" size="sm" onClick={() => setLines(null)}>
              Trocar
            </Button>
          </div>
        )}
        <FieldError message={preview.error?.message} />
      </Card>

      <AnimatePresence>
        {lines && (
          <motion.div initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0 }} transition={spring}>
            <Card className="flex flex-col gap-3">
              <div>
                <CardTitle>
                  {lines.length} linhas para {account?.name ?? "a conta"}
                </CardTitle>
                <CardDescription>Desmarque o que não deve entrar. Duplicatas e pagamentos de fatura sem fatura correspondente já vêm desmarcados.</CardDescription>
              </div>
              <ul className="flex flex-col">
                {lines.map((l) => (
                  <StatementRow key={l.index} line={l} onChange={(patch) => edit(l.index, patch)} />
                ))}
              </ul>
              <FieldError message={commit.error?.message} />
              <div className="flex flex-wrap items-center justify-end gap-2">
                <span className="mr-auto text-sm text-muted-foreground">{toImport.length} para importar</span>
                <Button variant="ghost" onClick={() => setLines(null)}>
                  Cancelar
                </Button>
                <Button onClick={confirm} disabled={commit.isPending || toImport.length === 0}>
                  {commit.isPending ? "Importando..." : "Importar"}
                </Button>
              </div>
            </Card>
          </motion.div>
        )}
      </AnimatePresence>

      {result && <BalanceCheck accountId={chosen} result={result} />}
      <StatementHistory />
    </>
  );
}

/** Label of what a statement line becomes. */
function kindLabel(l: Draft, cardName?: string) {
  switch (l.kind) {
    case "salary":
      return l.incomeId ? `Salário, confirma ${l.incomeName ?? "a renda"}` : "Salário, receita avulsa";
    case "card_payment": {
      if (!l.cardId || !l.invoiceMonth) return "Pagamento de fatura sem fatura fechada com esse valor";
      const [y, m] = l.invoiceMonth.split("-").map(Number);
      return `Paga a fatura de ${monthName(y!, m!).toLowerCase()} do ${cardName ?? "cartão"}`;
    }
    case "income":
      return "Receita avulsa";
    default:
      return "Despesa";
  }
}

/** One statement line with the import toggle, what it becomes, the name, the category and the warnings (RF-87 to RF-89, RN-32). */
function StatementRow({ line: l, onChange }: { line: Draft; onChange: (patch: Partial<Draft>) => void }) {
  const cards = useCards();
  const cardName = (cards.data ?? []).find((c) => c.id === l.cardId)?.name;
  const unpaid = l.kind === "card_payment" && !l.cardId;
  const editable = l.kind === "expense" || l.kind === "income" || (l.kind === "salary" && !l.incomeId);
  const credit = l.kind === "income" || l.kind === "salary";
  return (
    <li className={cn("flex flex-col gap-2 border-b py-3 last:border-b-0", l.skip && "opacity-60")}>
      <div className="flex items-start gap-3">
        <input
          type="checkbox"
          className="mt-1 size-4 accent-primary"
          checked={!l.skip}
          disabled={unpaid}
          onChange={(e) => onChange({ skip: !e.target.checked })}
          aria-label={`Importar ${l.merchant}`}
        />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
            <span>{formatDayMonth(l.date)}</span>
            <span className="truncate">{l.merchant}</span>
            <span className={cn("rounded-full px-1.5", credit ? "bg-success/10 text-success" : "bg-muted")}>{kindLabel(l, cardName)}</span>
          </div>
          {l.duplicate && (
            <p className="mt-1 flex items-center gap-1 text-xs text-warning">
              <AlertTriangle className="size-3.5 shrink-0" />
              Possível duplicata: {l.duplicate.description || "lançamento"}
              {l.duplicate.date ? ` em ${formatDate(l.duplicate.date)}` : ""}, <Money cents={Math.abs(l.duplicate.amountCents)} />
            </p>
          )}
        </div>
        <Money cents={credit ? l.amountCents : -l.amountCents} className={cn("shrink-0 text-sm font-medium", credit && "text-success")} />
      </div>
      <AnimatePresence initial={false}>
        {!l.skip && editable && (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ height: spring, opacity: fade }}
            className={cn("grid gap-2 overflow-hidden pl-7", l.kind === "expense" ? "sm:grid-cols-[1fr_1fr_auto]" : "sm:grid-cols-[1fr_auto]")}
          >
            <Input aria-label={`Nome de ${l.merchant}`} value={l.friendlyName} onChange={(e) => onChange({ friendlyName: e.target.value })} />
            {l.kind === "expense" && (
              <CategorySelect
                aria-label={`Categoria de ${l.merchant}`}
                placeholder="Escolha a categoria"
                value={l.categoryId ?? 0}
                onChange={(e) => onChange({ categoryId: Number(e.target.value) || undefined })}
              />
            )}
            <label className="flex items-center gap-2 text-xs text-muted-foreground">
              <input type="checkbox" className="size-4 accent-primary" checked={l.remember} onChange={(e) => onChange({ remember: e.target.checked })} />
              Lembrar
            </label>
          </motion.div>
        )}
      </AnimatePresence>
    </li>
  );
}

/** Compares the closing balance of the statement with the account in the Radar and offers the adjustment (RF-90, RF-06). */
function BalanceCheck({ accountId, result }: { accountId: number; result: StatementResult }) {
  const adjust = useAdjustBalance();
  const me = useMe();
  const [done, setDone] = useState(false);
  const radar = result.balanceCents;
  const diff = radar === undefined ? 0 : result.closingCents - radar;
  return (
    <Card className="flex flex-col gap-3">
      <div className="flex items-center gap-2">
        <Scale className="size-5 text-primary" />
        <CardTitle>Saldo depois da importação</CardTitle>
      </div>
      <dl className="grid grid-cols-2 gap-2 rounded-xl bg-muted p-3 text-sm">
        <div>
          <dt className="text-xs text-muted-foreground">Saldo no extrato</dt>
          <dd className="font-medium">
            <Money cents={result.closingCents} />
          </dd>
        </div>
        <div>
          <dt className="text-xs text-muted-foreground">Saldo no Radar</dt>
          <dd className="font-medium">{radar === undefined ? <span className="text-muted-foreground">só para quem é titular</span> : <Money cents={radar} />}</dd>
        </div>
      </dl>
      {radar !== undefined && (diff === 0 || done ? (
        <p className="text-sm text-success">Os saldos batem.</p>
      ) : (
        <div className="flex flex-wrap items-center justify-between gap-2">
          <p className="text-sm text-muted-foreground">
            Diferença de <Money cents={diff} signed />. O ajuste faz o saldo do Radar ficar igual ao do extrato.
          </p>
          {/* The adjustment tells the real balance, so it is made in the personal space of the holder (RF-83). */}
          {me?.group.personal ? (
          <Button
            variant="outline"
            disabled={adjust.isPending}
            onClick={() =>
              adjust.mutate({ id: accountId, body: { balanceCents: result.closingCents, date: todayISO(), description: "Ajuste pelo extrato" } }, { onSuccess: () => setDone(true) })
            }
          >
            Ajustar saldo
          </Button>
          ) : (
            <p className="text-xs text-muted-foreground">Para ajustar, abra a conta no seu espaço Pessoal.</p>
          )}
        </div>
      ))}
      <FieldError message={adjust.error?.message} />
    </Card>
  );
}

/** Previous statement imports with the number of entries and the undo button. */
function StatementHistory() {
  const imports = useStatementImports();
  const undo = useUndoStatementImport();
  if (!imports.data?.length) return null;
  return (
    <Card className="flex flex-col gap-2">
      <CardTitle>Extratos importados</CardTitle>
      <ul className="flex flex-col">
        {imports.data.map((i) => (
          <li key={i.id} className="flex items-center gap-3 border-b py-2 last:border-b-0">
            <FileUp className="size-4 shrink-0 text-primary" />
            <span className="min-w-0 flex-1">
              <span className="block truncate text-sm font-medium">
                {i.accountName}, {formatDate(i.createdAt.slice(0, 10))}
              </span>
              <span className="block truncate text-xs text-muted-foreground">
                {i.entryCount} lançamentos · {i.fileName || "sem nome"} · por {i.createdByName}
              </span>
            </span>
            <Button variant="ghost" size="sm" disabled={undo.isPending || i.entryCount === 0} onClick={() => undo.mutate(i.id)} aria-label={`Desfazer extrato ${i.fileName}`}>
              <Undo2 />
              Desfazer
            </Button>
          </li>
        ))}
      </ul>
      {undo.error && <p className="text-sm text-destructive">{undo.error.message}</p>}
    </Card>
  );
}
