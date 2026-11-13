import { Clock, History, Pencil, Plus, ReceiptText, Search, Store, Trash2 } from "lucide-react";
import { useState } from "react";
import { useSpaceWords } from "@/app/space";
import { Button } from "@/components/ui/button";
import { Card, CardDescription } from "@/components/ui/card";
import { Dialog, DialogActions } from "@/components/ui/dialog";
import { Field, FieldError } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Money } from "@/components/ui/money";
import { MoneyInput } from "@/components/ui/money-input";
import { NativeSelect } from "@/components/ui/native-select";
import { Switch } from "@/components/ui/switch";
import {
  UNITS,
  formatSizeMilli,
  parseSizeMilli,
  useAddPrice,
  useDeleteMarket,
  useDeletePrice,
  useDeleteProduct,
  useDeleteReceipt,
  useMarkets,
  useProductPrices,
  useProducts,
  useReceipts,
  useSaveMarket,
  useSaveProduct,
  useUpdatePrice,
  type Market,
  type MarketPrice,
  type MarketProduct,
  type ProductUnit,
} from "@/features/market/api";
import { formatDate, todayISO } from "@/lib/format";
import { cn } from "@/lib/utils";

/** Products with the latest price in each market and the stale warning (RF-66, RF-75, RF-76). */
export function ProductsTab() {
  const products = useProducts();
  const markets = useMarkets();
  const [query, setQuery] = useState("");
  const [editing, setEditing] = useState<MarketProduct | "new" | null>(null);
  const [history, setHistory] = useState<MarketProduct | null>(null);
  const names = new Map((markets.data ?? []).map((m) => [m.id, m.name]));
  const needle = query.trim().toLowerCase();
  const list = (products.data ?? []).filter((p) => !needle || p.label.toLowerCase().includes(needle));
  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-2">
        <div className="relative flex-1">
          <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input aria-label="Buscar produto" placeholder="Buscar produto" className="pl-9" value={query} onChange={(e) => setQuery(e.target.value)} />
        </div>
        <Button onClick={() => setEditing("new")}>
          <Plus />
          Produto
        </Button>
      </div>
      <Card className="p-2">
        <ul className="flex flex-col">
          {list.map((p) => (
            <li key={p.id} className="flex items-start gap-3 border-b px-2 py-2.5 last:border-b-0">
              <button type="button" className="min-w-0 flex-1 text-left" onClick={() => setHistory(p)} aria-label={`Preços de ${p.label}`}>
                <span className="block truncate text-sm font-medium">{p.label}</span>
                {p.prices.length === 0 ? (
                  <span className="text-xs text-muted-foreground">Sem preço ainda</span>
                ) : (
                  <span className="mt-1 flex flex-wrap gap-1.5">
                    {p.prices.map((pr) => (
                      <span
                        key={pr.marketId}
                        className={cn("inline-flex items-center gap-1 rounded-full bg-muted px-2 py-0.5 text-xs", pr.stale && "bg-warning/10 text-warning")}
                        title={pr.stale ? `Desatualizado, de ${formatDate(pr.date)}` : formatDate(pr.date)}
                      >
                        {pr.stale && <Clock className="size-3" aria-label="Desatualizado" />}
                        {names.get(pr.marketId)} <Money cents={pr.priceCents} />
                        {p.byWeight && "/kg"}
                      </span>
                    ))}
                  </span>
                )}
              </button>
              <Button variant="ghost" size="icon-sm" aria-label={`Histórico de ${p.label}`} onClick={() => setHistory(p)}>
                <History />
              </Button>
              <Button variant="ghost" size="icon-sm" aria-label={`Editar ${p.label}`} onClick={() => setEditing(p)}>
                <Pencil />
              </Button>
            </li>
          ))}
          {products.data && list.length === 0 && (
            <li className="px-2 py-6 text-center text-sm text-muted-foreground">
              {needle ? "Nenhum produto com esse nome." : "Cadastre os produtos com marca e tamanho, como Café Marca 500 g."}
            </li>
          )}
        </ul>
      </Card>
      <ProductDialog state={editing} onClose={() => setEditing(null)} />
      <PriceHistoryDialog product={history} markets={markets.data ?? []} onClose={() => setHistory(null)} />
    </div>
  );
}

/** Creates or edits a product; items sold by weight are priced per kg (RF-66). */
function ProductDialog({ state, onClose }: { state: MarketProduct | "new" | null; onClose: () => void }) {
  const save = useSaveProduct();
  const remove = useDeleteProduct();
  const product = state !== null && state !== "new" ? state : undefined;
  const [loaded, setLoaded] = useState<MarketProduct | "new" | null>(null);
  const [name, setName] = useState("");
  const [brand, setBrand] = useState("");
  const [byWeight, setByWeight] = useState(false);
  const [size, setSize] = useState("");
  const [unit, setUnit] = useState<ProductUnit>("g");
  // Loads the product into the fields each time the dialog opens for another one.
  if (state !== loaded) {
    setLoaded(state);
    setName(product?.name ?? "");
    setBrand(product?.brand ?? "");
    setByWeight(product?.byWeight ?? false);
    setSize(product && !product.byWeight ? formatSizeMilli(product.sizeMilli) : "");
    setUnit(product?.unit ?? "g");
  }

  /** Clears the mutation state and closes. */
  const close = () => {
    save.reset();
    remove.reset();
    onClose();
  };

  return (
    <Dialog open={state !== null} onOpenChange={(o) => !o && close()} title={product ? "Editar produto" : "Novo produto"}>
      <form
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate({ id: product?.id, body: { name, brand, byWeight, sizeMilli: byWeight ? 1000 : parseSizeMilli(size), unit: byWeight ? "kg" : unit } }, { onSuccess: close });
        }}
      >
        <div className="grid grid-cols-2 gap-3">
          <Field label="Produto" htmlFor="product-name">
            <Input id="product-name" autoComplete="off" placeholder="Café" value={name} onChange={(e) => setName(e.target.value)} />
          </Field>
          <Field label="Marca" htmlFor="product-brand">
            <Input id="product-brand" autoComplete="off" placeholder={byWeight ? "Sem marca" : "Marca"} value={brand} onChange={(e) => setBrand(e.target.value)} />
          </Field>
        </div>
        <label className="flex items-center justify-between gap-3 text-sm">
          <span>
            Vendido por peso
            <span className="block text-xs text-muted-foreground">Hortifrúti, carnes e frios: o preço é por kg.</span>
          </span>
          <Switch checked={byWeight} onCheckedChange={setByWeight} aria-label="Vendido por peso" />
        </label>
        {!byWeight && (
          <div className="grid grid-cols-2 gap-3">
            <Field label="Tamanho" htmlFor="product-size">
              <Input id="product-size" inputMode="decimal" autoComplete="off" placeholder="500" value={size} onChange={(e) => setSize(e.target.value)} />
            </Field>
            <Field label="Unidade" htmlFor="product-unit">
              <NativeSelect id="product-unit" value={unit} onChange={(e) => setUnit(e.target.value as ProductUnit)}>
                {UNITS.map((u) => (
                  <option key={u.value} value={u.value}>
                    {u.label}
                  </option>
                ))}
              </NativeSelect>
            </Field>
          </div>
        )}
        <FieldError message={save.error?.message ?? remove.error?.message} />
        <DialogActions className="sm:justify-between">
          {product ? (
            <Button variant="ghost" className="text-destructive" disabled={remove.isPending} onClick={() => remove.mutate(product.id, { onSuccess: close })}>
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
            <Button type="submit" disabled={save.isPending}>
              Salvar
            </Button>
          </span>
        </DialogActions>
      </form>
    </Dialog>
  );
}

/** Price history of a product in every market, with a new price and corrections (RF-67, RF-70, RF-76). */
function PriceHistoryDialog({ product, markets, onClose }: { product: MarketProduct | null; markets: Market[]; onClose: () => void }) {
  const prices = useProductPrices(product?.id);
  const add = useAddPrice();
  const remove = useDeletePrice();
  const [editing, setEditing] = useState<MarketPrice | null>(null);
  const active = markets.filter((m) => m.active);
  const [marketId, setMarketId] = useState(0);
  const [cents, setCents] = useState(0);
  const [date, setDate] = useState(todayISO());
  const chosen = marketId || active[0]?.id || 0;

  /** Clears the form and closes. */
  const close = () => {
    add.reset();
    setCents(0);
    onClose();
  };

  return (
    <Dialog open={product !== null} onOpenChange={(o) => !o && close()} title={product?.label ?? ""} description={product?.byWeight ? "Preços por kg" : undefined}>
      <div className="flex flex-col gap-4">
        {active.length === 0 ? (
          <p className="text-sm text-muted-foreground">Cadastre um mercado para registrar preços.</p>
        ) : (
          <form
            className="grid grid-cols-2 gap-3 sm:grid-cols-[1.4fr_1fr_1fr_auto] sm:items-end"
            noValidate
            onSubmit={(e) => {
              e.preventDefault();
              if (!product) return;
              add.mutate({ productId: product.id, marketId: chosen, priceCents: cents, date }, { onSuccess: () => setCents(0) });
            }}
          >
            <Field label="Mercado" htmlFor="price-market">
              <NativeSelect id="price-market" value={chosen} onChange={(e) => setMarketId(Number(e.target.value))}>
                {active.map((m) => (
                  <option key={m.id} value={m.id}>
                    {m.name}
                    {m.neighborhood ? `, ${m.neighborhood}` : ""}
                  </option>
                ))}
              </NativeSelect>
            </Field>
            <Field label="Data" htmlFor="price-date">
              <Input id="price-date" type="date" value={date} max={todayISO()} onChange={(e) => setDate(e.target.value)} />
            </Field>
            <Field label="Preço" htmlFor="price-value">
              <MoneyInput id="price-value" value={cents} onChange={setCents} />
            </Field>
            <Button type="submit" disabled={add.isPending} className="self-end">
              <Plus />
              Registrar
            </Button>
          </form>
        )}
        <FieldError message={add.error?.message ?? remove.error?.message} />
        <ul className="flex max-h-[45dvh] flex-col overflow-y-auto">
          {(prices.data ?? []).map((p) => (
            <li key={p.id} className="flex items-center gap-3 border-b py-2 last:border-b-0">
              <Store className="size-4 shrink-0 text-muted-foreground" />
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm">
                  {p.marketName}
                  {p.neighborhood ? `, ${p.neighborhood}` : ""}
                </span>
                <span className="block text-xs text-muted-foreground">
                  {formatDate(p.date)} · {p.source === "receipt" ? "nota fiscal" : "manual"}
                </span>
              </span>
              <Money cents={p.priceCents} className="text-sm font-medium" />
              <Button variant="ghost" size="icon-sm" aria-label={`Corrigir preço de ${formatDate(p.date)} em ${p.marketName}`} onClick={() => setEditing(p)}>
                <Pencil />
              </Button>
              <Button variant="ghost" size="icon-sm" aria-label={`Excluir preço de ${formatDate(p.date)} em ${p.marketName}`} disabled={remove.isPending} onClick={() => remove.mutate(p.id)}>
                <Trash2 />
              </Button>
            </li>
          ))}
          {prices.data?.length === 0 && <li className="py-4 text-center text-sm text-muted-foreground">Nenhum preço registrado.</li>}
        </ul>
      </div>
      <PriceCorrectionDialog price={editing} onClose={() => setEditing(null)} />
    </Dialog>
  );
}

/** Corrects the value or the date of a price (RF-70). */
function PriceCorrectionDialog({ price, onClose }: { price: MarketPrice | null; onClose: () => void }) {
  const update = useUpdatePrice();
  const [loaded, setLoaded] = useState<MarketPrice | null>(null);
  const [cents, setCents] = useState(0);
  const [date, setDate] = useState("");
  // Loads the price into the fields each time the dialog opens for another one.
  if (price !== loaded) {
    setLoaded(price);
    setCents(price?.priceCents ?? 0);
    setDate(price?.date ?? "");
  }

  /** Clears the mutation state and closes. */
  const close = () => {
    update.reset();
    onClose();
  };

  return (
    <Dialog open={price !== null} onOpenChange={(o) => !o && close()} title="Corrigir preço" description={price ? `${price.marketName}` : undefined}>
      <form
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault();
          if (price) update.mutate({ id: price.id, body: { priceCents: cents, date } }, { onSuccess: close });
        }}
      >
        <div className="grid grid-cols-2 gap-3">
          <Field label="Preço" htmlFor="fix-value">
            <MoneyInput id="fix-value" value={cents} onChange={setCents} />
          </Field>
          <Field label="Data" htmlFor="fix-date">
            <Input id="fix-date" type="date" value={date} max={todayISO()} onChange={(e) => setDate(e.target.value)} />
          </Field>
        </div>
        <FieldError message={update.error?.message} />
        <DialogActions>
          <Button variant="ghost" onClick={close}>
            Cancelar
          </Button>
          <Button type="submit" disabled={update.isPending}>
            Salvar
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}

/** Markets with neighborhood and number of prices (RF-65). */
export function MarketsTab() {
  const markets = useMarkets();
  const words = useSpaceWords();
  const [editing, setEditing] = useState<Market | "new" | null>(null);
  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center justify-between gap-2">
        <CardDescription className="px-1">Os mercados aparecem também ao ler uma nota fiscal.</CardDescription>
        <Button onClick={() => setEditing("new")}>
          <Plus />
          Mercado
        </Button>
      </div>
      <Card className="p-2">
        <ul className="flex flex-col">
          {(markets.data ?? []).map((m) => (
            <li key={m.id} className={cn("flex items-center gap-3 border-b px-2 py-2.5 last:border-b-0", !m.active && "opacity-60")}>
              <Store className="size-4 shrink-0 text-primary" />
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm font-medium">{m.name}</span>
                <span className="block truncate text-xs text-muted-foreground">
                  {[m.neighborhood, `${m.priceCount} ${m.priceCount === 1 ? "preço" : "preços"}`, !m.active && "desativado"].filter(Boolean).join(" · ")}
                </span>
              </span>
              <Button variant="ghost" size="icon-sm" aria-label={`Editar ${m.name}`} onClick={() => setEditing(m)}>
                <Pencil />
              </Button>
            </li>
          ))}
          {markets.data?.length === 0 && <li className="px-2 py-6 text-center text-sm text-muted-foreground">{words.personal ? "Cadastre os mercados onde você compra." : "Cadastre os mercados onde o grupo compra."}</li>}
        </ul>
      </Card>
      <MarketDialog state={editing} onClose={() => setEditing(null)} />
    </div>
  );
}

/** Creates or edits a market. */
function MarketDialog({ state, onClose }: { state: Market | "new" | null; onClose: () => void }) {
  const save = useSaveMarket();
  const remove = useDeleteMarket();
  const market = state !== null && state !== "new" ? state : undefined;
  const [loaded, setLoaded] = useState<Market | "new" | null>(null);
  const [name, setName] = useState("");
  const [neighborhood, setNeighborhood] = useState("");
  const [active, setActive] = useState(true);
  // Loads the market into the fields each time the dialog opens for another one.
  if (state !== loaded) {
    setLoaded(state);
    setName(market?.name ?? "");
    setNeighborhood(market?.neighborhood ?? "");
    setActive(market?.active ?? true);
  }

  /** Clears the mutation state and closes. */
  const close = () => {
    save.reset();
    remove.reset();
    onClose();
  };

  return (
    <Dialog open={state !== null} onOpenChange={(o) => !o && close()} title={market ? "Editar mercado" : "Novo mercado"}>
      <form
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate({ id: market?.id, body: { name, neighborhood, active } }, { onSuccess: close });
        }}
      >
        <Field label="Nome" htmlFor="market-name">
          <Input id="market-name" autoComplete="off" value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
        <Field label="Bairro" htmlFor="market-neighborhood">
          <Input id="market-neighborhood" autoComplete="off" value={neighborhood} onChange={(e) => setNeighborhood(e.target.value)} />
        </Field>
        {market && (
          <label className="flex items-center justify-between gap-3 text-sm">
            Ativo
            <Switch checked={active} onCheckedChange={setActive} />
          </label>
        )}
        <FieldError message={save.error?.message ?? remove.error?.message} />
        <DialogActions className="sm:justify-between">
          {market ? (
            <Button variant="ghost" className="text-destructive" disabled={remove.isPending} onClick={() => remove.mutate(market.id, { onSuccess: close })}>
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
            <Button type="submit" disabled={save.isPending}>
              Salvar
            </Button>
          </span>
        </DialogActions>
      </form>
    </Dialog>
  );
}

/** Receipts already read, with the removal of a wrong one and its prices. */
export function ReceiptsTab() {
  const receipts = useReceipts();
  const remove = useDeleteReceipt();
  return (
    <Card className="p-2">
      <ul className="flex flex-col">
        {(receipts.data ?? []).map((r) => (
          <li key={r.id} className="flex items-center gap-3 border-b px-2 py-2.5 last:border-b-0">
            <ReceiptText className="size-4 shrink-0 text-primary" />
            <span className="min-w-0 flex-1">
              <span className="block truncate text-sm font-medium">
                {r.marketName}, {formatDate(r.issuedOn)}
              </span>
              <span className="block truncate text-xs text-muted-foreground">
                {r.priceCount} {r.priceCount === 1 ? "preço" : "preços"} de {r.itemCount} itens · lida por {r.createdByName}
              </span>
            </span>
            <Money cents={r.totalCents} className="text-sm" />
            <Button variant="ghost" size="icon-sm" aria-label={`Remover nota de ${r.marketName} de ${formatDate(r.issuedOn)}`} disabled={remove.isPending} onClick={() => remove.mutate(r.id)}>
              <Trash2 />
            </Button>
          </li>
        ))}
        {receipts.data?.length === 0 && <li className="px-2 py-6 text-center text-sm text-muted-foreground">As notas lidas aparecem aqui.</li>}
      </ul>
      {remove.error && <p className="px-2 text-sm text-destructive">{remove.error.message}</p>}
    </Card>
  );
}
