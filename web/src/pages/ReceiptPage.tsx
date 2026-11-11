import { AnimatePresence, motion } from "motion/react";
import { AlertTriangle, ChevronLeft, Link2, ScanLine } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate } from "react-router";
import { Button } from "@/components/ui/button";
import { Card, CardDescription, CardTitle } from "@/components/ui/card";
import { Field, FieldError } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Money } from "@/components/ui/money";
import { NativeSelect } from "@/components/ui/native-select";
import { Switch } from "@/components/ui/switch";
import {
  UNITS,
  formatQuantity,
  parseSizeMilli,
  useCommitReceipt,
  useMarkets,
  usePreviewReceipt,
  useProducts,
  type ProductUnit,
  type ReceiptCommit,
  type ReceiptLine,
  type ReceiptPreview,
} from "@/features/market/api";
import { QrScanner } from "@/features/market/QrScanner";
import { formatDate } from "@/lib/format";
import { fade, spring } from "@/lib/motion";
import { cn } from "@/lib/utils";

/** What to do with a receipt line: skip now, ignore always, link to a product or create one. */
type Decision =
  | { kind: "skip" }
  | { kind: "ignore" }
  | { kind: "product"; productId: number }
  | { kind: "new"; name: string; brand: string; byWeight: boolean; size: string; unit: ProductUnit };

/** Starting decision of each line, from what was learned for the market (RF-69). */
function initialDecisions(p: ReceiptPreview): Decision[] {
  return p.items.map((l) => (l.productId ? { kind: "product", productId: l.productId } : l.ignored ? { kind: "ignore" } : { kind: "skip" }));
}

/** Capitalizes each word of a name printed in capitals on the receipt. */
function titleCase(s: string) {
  return s.toLowerCase().replace(/(^|\s)\S/g, (c) => c.toUpperCase());
}

/** Reads the receipt from the QR code or the pasted link, links each line to a product and records the prices (RF-68, RF-69). */
export function ReceiptPage() {
  const navigate = useNavigate();
  const preview = usePreviewReceipt();
  const commit = useCommitReceipt();
  const markets = useMarkets();
  const products = useProducts();
  const [link, setLink] = useState("");
  const [data, setData] = useState<ReceiptPreview | null>(null);
  const [decisions, setDecisions] = useState<Decision[]>([]);
  const [market, setMarket] = useState<number | "new">("new");
  const [newMarket, setNewMarket] = useState({ name: "", neighborhood: "" });

  /** Asks the server for the receipt, optionally with the chosen market to bring its links. */
  const load = (url: string, marketId?: number) =>
    preview.mutate(
      { url, marketId },
      {
        onSuccess: (p) => {
          setData(p);
          setDecisions(initialDecisions(p));
          setMarket(p.marketId ?? "new");
          if (!marketId) setNewMarket({ name: titleCase(p.storeName), neighborhood: titleCase(p.neighborhood) });
        },
      },
    );

  /** Changes the market; an existing one brings its learned links. */
  const chooseMarket = (value: string) => {
    if (value === "new") {
      setMarket("new");
      if (data) setDecisions(data.items.map(() => ({ kind: "skip" })));
      return;
    }
    if (data) load(data.url, Number(value));
  };

  /** Sends the chosen lines. */
  const confirm = () => {
    if (!data) return;
    const items: ReceiptCommit["items"] = [];
    decisions.forEach((d, index) => {
      if (d.kind === "ignore") items.push({ index, ignore: true });
      else if (d.kind === "product") items.push({ index, productId: d.productId });
      else if (d.kind === "new")
        items.push({ index, newProduct: { name: d.name, brand: d.brand, byWeight: d.byWeight, sizeMilli: d.byWeight ? 1000 : parseSizeMilli(d.size), unit: d.byWeight ? "kg" : d.unit } });
    });
    commit.mutate(
      { url: data.url, items, ...(market === "new" ? { newMarket } : { marketId: market }) },
      { onSuccess: () => navigate("/market?tab=receipts") },
    );
  };

  const chosen = decisions.filter((d) => d.kind !== "skip").length;
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-0.5">
        <Link to="/market" className="flex items-center gap-0.5 text-sm text-primary">
          <ChevronLeft className="size-4" />
          Mercado
        </Link>
        <h1 className="px-1 text-lg font-semibold">Ler nota fiscal</h1>
      </div>

      {!data ? (
        <Card className="mx-auto flex w-full max-w-md flex-col gap-4">
          {preview.isPending ? (
            <div className="flex aspect-square w-full flex-col items-center justify-center gap-3 rounded-2xl bg-muted text-sm text-muted-foreground">
              <ScanLine className="size-6 animate-pulse text-primary motion-reduce:animate-none" />
              Consultando a nota na Sefaz...
            </div>
          ) : (
            <QrScanner onRead={(text) => load(text)} />
          )}
          <form
            className="flex items-end gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              if (link.trim()) load(link.trim());
            }}
          >
            <div className="flex-1">
              <Field label="Ou cole o link da nota" htmlFor="receipt-link">
                <Input id="receipt-link" inputMode="url" autoComplete="off" placeholder="https://nfeweb.sefaz.go.gov.br/..." value={link} onChange={(e) => setLink(e.target.value)} />
              </Field>
            </div>
            <Button type="submit" disabled={preview.isPending || !link.trim()}>
              <Link2 />
              Ler
            </Button>
          </form>
          <FieldError message={preview.error?.message} />
        </Card>
      ) : (
        <motion.div initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={spring} className="flex flex-col gap-4">
          <Card className="flex flex-col gap-3">
            <div className="flex flex-wrap items-start justify-between gap-2">
              <div>
                <CardTitle>{titleCase(data.storeName)}</CardTitle>
                <CardDescription>
                  {data.issuedOn ? formatDate(data.issuedOn) : "Sem data"} · {data.items.length} itens · total <Money cents={data.totalCents} />
                </CardDescription>
              </div>
              <Button variant="ghost" size="sm" onClick={() => setData(null)}>
                Ler outra
              </Button>
            </div>
            {data.importedId && (
              <p className="flex items-center gap-1.5 rounded-xl bg-warning/10 px-3 py-2 text-sm text-warning">
                <AlertTriangle className="size-4 shrink-0" />
                Essa nota já foi lida. Os preços dela já estão registrados.
              </p>
            )}
            <div className="grid gap-3 sm:grid-cols-[1fr_1fr_1fr]">
              <Field label="Mercado" htmlFor="receipt-market">
                <NativeSelect id="receipt-market" value={market} onChange={(e) => chooseMarket(e.target.value)} disabled={preview.isPending}>
                  <option value="new">Novo mercado</option>
                  {(markets.data ?? [])
                    .filter((m) => m.active)
                    .map((m) => (
                      <option key={m.id} value={m.id}>
                        {m.name}
                        {m.neighborhood ? `, ${m.neighborhood}` : ""}
                      </option>
                    ))}
                </NativeSelect>
              </Field>
              {market === "new" && (
                <>
                  <Field label="Nome do mercado" htmlFor="receipt-market-name">
                    <Input id="receipt-market-name" autoComplete="off" value={newMarket.name} onChange={(e) => setNewMarket({ ...newMarket, name: e.target.value })} />
                  </Field>
                  <Field label="Bairro" htmlFor="receipt-market-neighborhood">
                    <Input id="receipt-market-neighborhood" autoComplete="off" value={newMarket.neighborhood} onChange={(e) => setNewMarket({ ...newMarket, neighborhood: e.target.value })} />
                  </Field>
                </>
              )}
            </div>
          </Card>

          <Card className="flex flex-col gap-1">
            <CardTitle>Itens</CardTitle>
            <CardDescription>Ligue cada linha a um produto. Nas próximas notas desse mercado, a ligação já vem pronta.</CardDescription>
            <ul className="mt-2 flex flex-col">
              {data.items.map((l, i) => (
                <LineRow
                  key={l.index}
                  line={l}
                  decision={decisions[i] ?? { kind: "skip" }}
                  products={products.data ?? []}
                  onChange={(d) => setDecisions((ds) => ds.map((x, j) => (j === i ? d : x)))}
                />
              ))}
            </ul>
          </Card>
          <FieldError message={commit.error?.message ?? preview.error?.message} />
          <div className="flex flex-wrap items-center justify-end gap-2">
            <span className="mr-auto text-sm text-muted-foreground">{chosen} de {data.items.length} linhas</span>
            <Button variant="ghost" onClick={() => setData(null)}>
              Cancelar
            </Button>
            <Button onClick={confirm} disabled={commit.isPending || chosen === 0 || !!data.importedId}>
              {commit.isPending ? "Registrando..." : "Registrar preços"}
            </Button>
          </div>
        </motion.div>
      )}
    </div>
  );
}

/** One receipt line with its decision; a new product opens its fields prefilled from the line. */
function LineRow({
  line: l,
  decision: d,
  products,
  onChange,
}: {
  line: ReceiptLine;
  decision: Decision;
  products: { id: number; label: string }[];
  onChange: (d: Decision) => void;
}) {
  const value = d.kind === "product" ? String(d.productId) : d.kind;
  const weighed = l.unit.toUpperCase() === "KG";
  return (
    <li className={cn("flex flex-col gap-2 border-b py-3 last:border-b-0", d.kind === "skip" && "opacity-70")}>
      <div className="flex items-start justify-between gap-3">
        <span className="min-w-0">
          <span className="block truncate text-sm font-medium">{l.description}</span>
          <span className="block text-xs text-muted-foreground">
            {formatQuantity(l.quantityMilli, l.unit)} × <Money cents={l.unitPriceCents} />
            {weighed && "/kg"}
          </span>
        </span>
        <Money cents={l.totalCents} className="shrink-0 text-sm" />
      </div>
      <NativeSelect
        aria-label={`Produto de ${l.description}`}
        value={value}
        onChange={(e) => {
          const v = e.target.value;
          if (v === "skip" || v === "ignore") onChange({ kind: v });
          else if (v === "new") onChange({ kind: "new", name: titleCase(l.description), brand: weighed ? "Sem marca" : "", byWeight: weighed, size: "", unit: "un" });
          else onChange({ kind: "product", productId: Number(v) });
        }}
      >
        <option value="skip">Não registrar agora</option>
        <option value="ignore">Ignorar sempre neste mercado</option>
        <option value="new">Novo produto...</option>
        {products.map((p) => (
          <option key={p.id} value={p.id}>
            {p.label}
          </option>
        ))}
      </NativeSelect>
      <AnimatePresence initial={false}>
        {d.kind === "new" && (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ height: spring, opacity: fade }}
            className="grid grid-cols-2 gap-2 overflow-hidden sm:grid-cols-4"
          >
            <Input aria-label={`Nome do produto de ${l.description}`} placeholder="Produto" value={d.name} onChange={(e) => onChange({ ...d, name: e.target.value })} />
            <Input aria-label={`Marca de ${l.description}`} placeholder="Marca" value={d.brand} onChange={(e) => onChange({ ...d, brand: e.target.value })} />
            {d.byWeight ? (
              <span className="col-span-2 flex items-center text-xs text-muted-foreground sm:col-span-1">Preço por kg</span>
            ) : (
              <span className="col-span-2 grid grid-cols-2 gap-2 sm:col-span-1">
                <Input aria-label={`Tamanho de ${l.description}`} inputMode="decimal" placeholder="500" value={d.size} onChange={(e) => onChange({ ...d, size: e.target.value })} />
                <NativeSelect aria-label={`Unidade de ${l.description}`} value={d.unit} onChange={(e) => onChange({ ...d, unit: e.target.value as ProductUnit })}>
                  {UNITS.map((u) => (
                    <option key={u.value} value={u.value}>
                      {u.label}
                    </option>
                  ))}
                </NativeSelect>
              </span>
            )}
            <label className="col-span-2 flex items-center gap-2 text-xs text-muted-foreground sm:col-span-1">
              <Switch checked={d.byWeight} onCheckedChange={(v) => onChange({ ...d, byWeight: v, brand: v && !d.brand ? "Sem marca" : d.brand })} aria-label={`Por peso, ${l.description}`} />
              Por peso
            </label>
          </motion.div>
        )}
      </AnimatePresence>
    </li>
  );
}
