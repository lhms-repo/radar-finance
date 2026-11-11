import { AnimatePresence, motion } from "motion/react";
import { AlertTriangle, ChevronLeft, FileUp, Undo2, Upload } from "lucide-react";
import { useId, useState } from "react";
import { Link } from "react-router";
import { Button } from "@/components/ui/button";
import { Card, CardDescription, CardTitle } from "@/components/ui/card";
import { Field, FieldError } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Money } from "@/components/ui/money";
import { NativeSelect } from "@/components/ui/native-select";
import { useCards } from "@/features/cards/api";
import { CategorySelect } from "@/features/categories/CategorySelect";
import { useCommitImport, useImports, usePreviewImport, useUndoImport, type ImportPreviewLine } from "@/features/imports/api";
import { formatDate, formatDayMonth, monthName, monthParam } from "@/lib/format";
import { fade, spring } from "@/lib/motion";
import { useMonth } from "@/lib/month";
import { cn } from "@/lib/utils";

/** A preview line as edited by the user before confirming. */
type Draft = ImportPreviewLine & { remember: boolean };

/** Import of the Nubank invoice CSV: choose card and invoice, review each line, confirm (RF-53 to RF-57). */
export function ImportPage() {
  const cards = useCards();
  const { year, month } = useMonth();
  const [cardId, setCardId] = useState(0);
  const [invoice, setInvoice] = useState(monthParam(year, month));
  const [fileName, setFileName] = useState("");
  const [lines, setLines] = useState<Draft[] | null>(null);
  const preview = usePreviewImport();
  const commit = useCommitImport();
  const fileId = useId();
  const activeCards = (cards.data ?? []).filter((c) => c.active);
  const chosenCard = cardId || activeCards[0]?.id || 0;

  /** Reads the chosen file and asks the server for the preview. */
  const onFile = async (file: File | undefined) => {
    if (!file || !chosenCard) return;
    setFileName(file.name);
    const content = await file.text();
    preview.mutate(
      { cardId: chosenCard, invoiceMonth: invoice, content },
      { onSuccess: (data) => setLines(data.map((l) => ({ ...l, remember: false }))) },
    );
  };

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
        cardId: chosenCard,
        invoiceMonth: invoice,
        fileName,
        lines: lines.map((l) => ({
          date: l.date,
          title: l.title,
          merchant: l.merchant,
          amountCents: l.amountCents,
          kind: l.kind,
          installment: l.installment,
          installments: l.installments,
          friendlyName: l.friendlyName,
          categoryId: l.categoryId || undefined,
          skip: l.skip,
          remember: l.remember,
        })),
      },
      { onSuccess: () => setLines(null) },
    );
  };

  const chosen = (lines ?? []).filter((l) => !l.skip);
  const [iy, im] = invoice.split("-").map(Number);
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-0.5">
        <Link to="/settings" className="flex items-center gap-0.5 text-sm text-primary">
          <ChevronLeft className="size-4" />
          Ajustes
        </Link>
        <h1 className="px-1 text-lg font-semibold">Importar fatura</h1>
      </div>

      <Card className="flex flex-col gap-4">
        <div>
          <CardTitle>CSV do Nubank</CardTitle>
          <CardDescription>No app do Nubank, abra a fatura e exporte em CSV. As compras entram na fatura escolhida aqui.</CardDescription>
        </div>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Cartão" htmlFor="import-card">
            <NativeSelect id="import-card" value={chosenCard} onChange={(e) => setCardId(Number(e.target.value))} disabled={!!lines}>
              {activeCards.length === 0 && <option value={0}>Nenhum cartão cadastrado</option>}
              {activeCards.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </NativeSelect>
          </Field>
          <Field label="Fatura (mês do vencimento)" htmlFor="import-month">
            <Input id="import-month" type="month" value={invoice} onChange={(e) => setInvoice(e.target.value)} disabled={!!lines} />
          </Field>
        </div>
        {!lines ? (
          <label htmlFor={fileId} className={cn("flex cursor-pointer flex-col items-center gap-2 rounded-2xl border-2 border-dashed p-8 text-center transition-colors hover:bg-accent", !chosenCard && "pointer-events-none opacity-50")}>
            <Upload className="size-6 text-primary" />
            <span className="text-sm font-medium">{preview.isPending ? "Lendo o arquivo..." : "Escolher o arquivo CSV"}</span>
            <input id={fileId} aria-label="Arquivo CSV" type="file" accept=".csv,text/csv" className="sr-only" onChange={(e) => void onFile(e.target.files?.[0])} />
          </label>
        ) : (
          <div className="flex items-center justify-between gap-2 rounded-xl bg-muted px-3 py-2 text-sm">
            <span className="flex min-w-0 items-center gap-2">
              <FileUp className="size-4 shrink-0 text-primary" />
              <span className="truncate">{fileName}</span>
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
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div>
                  <CardTitle>
                    {lines.length} linhas para a fatura de {monthName(iy!, im!).toLowerCase()} de {iy}
                  </CardTitle>
                  <CardDescription>Desmarque o que não deve entrar. Duplicatas e o pagamento da fatura já vêm desmarcados.</CardDescription>
                </div>
              </div>
              <ul className="flex flex-col">
                {lines.map((l) => (
                  <PreviewRow key={l.index} line={l} onChange={(patch) => edit(l.index, patch)} />
                ))}
              </ul>
              <FieldError message={commit.error?.message} />
              <div className="flex flex-wrap items-center justify-end gap-2">
                <span className="mr-auto text-sm text-muted-foreground">{chosen.length} para importar</span>
                <Button variant="ghost" onClick={() => setLines(null)}>
                  Cancelar
                </Button>
                <Button onClick={confirm} disabled={commit.isPending || chosen.length === 0}>
                  {commit.isPending ? "Importando..." : "Importar"}
                </Button>
              </div>
            </Card>
          </motion.div>
        )}
      </AnimatePresence>

      <History />
    </div>
  );
}

/** One line of the preview with the import toggle, the friendly name, the category and the warnings (RF-56). */
function PreviewRow({ line: l, onChange }: { line: Draft; onChange: (patch: Partial<Draft>) => void }) {
  const payment = l.kind === "payment";
  return (
    <li className={cn("flex flex-col gap-2 border-b py-3 last:border-b-0", l.skip && "opacity-60")}>
      <div className="flex items-start gap-3">
        <input
          type="checkbox"
          className="mt-1 size-4 accent-primary"
          checked={!l.skip}
          disabled={payment}
          onChange={(e) => onChange({ skip: !e.target.checked })}
          aria-label={`Importar ${l.title}`}
        />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
            <span>{formatDayMonth(l.date)}</span>
            <span className="truncate">{l.merchant}</span>
            {l.installments && <span className="rounded-full bg-primary/10 px-1.5 text-primary">Parcela {l.installment}/{l.installments}</span>}
            {l.kind === "credit" && <span className="rounded-full bg-success/10 px-1.5 text-success">Estorno ou crédito</span>}
            {payment && <span className="rounded-full bg-muted px-1.5">Pagamento, já registrado pela fatura</span>}
          </div>
          {l.duplicate && (
            <p className="mt-1 flex items-center gap-1 text-xs text-warning">
              <AlertTriangle className="size-3.5 shrink-0" />
              Possível duplicata: {l.duplicate.description}
              {l.duplicate.date ? ` em ${formatDate(l.duplicate.date)}` : " (recorrência)"}, <Money cents={Math.abs(l.duplicate.amountCents)} />
            </p>
          )}
        </div>
        <Money cents={l.kind === "purchase" ? l.amountCents : -l.amountCents} className={cn("shrink-0 text-sm font-medium", l.kind !== "purchase" && "text-success")} />
      </div>
      <AnimatePresence initial={false}>
        {!l.skip && !payment && (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ height: spring, opacity: fade }}
            className="grid gap-2 overflow-hidden pl-7 sm:grid-cols-[1fr_1fr_auto]"
          >
            <Input aria-label={`Nome de ${l.title}`} value={l.friendlyName} onChange={(e) => onChange({ friendlyName: e.target.value })} />
            <CategorySelect
              aria-label={`Categoria de ${l.title}`}
              placeholder={l.kind === "credit" ? "Sem categoria" : "Escolha a categoria"}
              value={l.categoryId ?? 0}
              onChange={(e) => onChange({ categoryId: Number(e.target.value) || undefined })}
            />
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

/** Previous imports with the number of purchases and the undo button. */
function History() {
  const imports = useImports();
  const undo = useUndoImport();
  if (!imports.data?.length) return null;
  return (
    <Card className="flex flex-col gap-2">
      <CardTitle>Importações anteriores</CardTitle>
      <ul className="flex flex-col">
        {imports.data.map((i) => {
          const [y, m] = i.invoiceMonth.split("-").map(Number);
          return (
            <li key={i.id} className="flex items-center gap-3 border-b py-2 last:border-b-0">
              <FileUp className="size-4 shrink-0 text-primary" />
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm font-medium">
                  {i.cardName}, fatura de {monthName(y!, m!).toLowerCase()} de {y}
                </span>
                <span className="block truncate text-xs text-muted-foreground">
                  {i.purchaseCount} lançamentos · {i.fileName || "sem nome"} · por {i.createdByName}
                </span>
              </span>
              <Button variant="ghost" size="sm" disabled={undo.isPending || i.purchaseCount === 0} onClick={() => undo.mutate(i.id)}>
                <Undo2 />
                Desfazer
              </Button>
            </li>
          );
        })}
      </ul>
      {undo.error && <p className="text-sm text-destructive">{undo.error.message}</p>}
    </Card>
  );
}
