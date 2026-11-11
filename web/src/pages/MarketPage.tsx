import { ScanLine, Settings2 } from "lucide-react";
import { useState } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { Button } from "@/components/ui/button";
import { Dialog, DialogActions } from "@/components/ui/dialog";
import { Field, FieldError } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { MoneyInput } from "@/components/ui/money-input";
import { Segmented } from "@/components/ui/segmented";
import { MarketsTab, ProductsTab, ReceiptsTab } from "@/features/market/catalog";
import { ListsTab } from "@/features/market/lists";
import { useMarketSettings, useSaveMarketSettings, type MarketSettings } from "@/features/market/api";

/** Tabs of the market screen. */
type Tab = "lists" | "products" | "markets" | "receipts";

/** Market screen: products and prices, markets and the comparison settings (section 2.14). */
export function MarketPage() {
  const [params, setParams] = useSearchParams();
  const navigate = useNavigate();
  const raw = params.get("tab");
  const tab: Tab = raw === "products" || raw === "markets" || raw === "receipts" ? raw : "lists";
  const [settingsOpen, setSettingsOpen] = useState(false);
  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between gap-3">
        <h1 className="px-1 text-lg font-semibold">Mercado</h1>
        <span className="flex items-center gap-1">
          <Button variant="ghost" size="icon" aria-label="Ajustes da comparação" onClick={() => setSettingsOpen(true)}>
            <Settings2 />
          </Button>
          <Button onClick={() => navigate("/market/receipt")}>
            <ScanLine />
            Ler nota
          </Button>
        </span>
      </div>
      <Segmented<Tab>
        ariaLabel="Seção"
        value={tab}
        options={[
          { value: "lists", label: "Listas" },
          { value: "products", label: "Produtos" },
          { value: "markets", label: "Mercados" },
          { value: "receipts", label: "Notas" },
        ]}
        onChange={(v) => setParams(v === "lists" ? {} : { tab: v }, { replace: true })}
        className="self-start"
      />
      {tab === "lists" && <ListsTab />}
      {tab === "products" && <ProductsTab />}
      {tab === "markets" && <MarketsTab />}
      {tab === "receipts" && <ReceiptsTab />}
      <SettingsDialog open={settingsOpen} onClose={() => setSettingsOpen(false)} />
    </div>
  );
}

/** Maximum markets per purchase, minimum saving and stale price days (RF-74, RN-25). */
function SettingsDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const current = useMarketSettings();
  const save = useSaveMarketSettings();
  const [draft, setDraft] = useState<MarketSettings | null>(null);
  const v = draft ?? current.data ?? { maxMarkets: 2, minSavingsCents: 1000, staleDays: 30 };

  /** Discards the draft and closes. */
  const close = () => {
    save.reset();
    setDraft(null);
    onClose();
  };

  return (
    <Dialog open={open} onOpenChange={(o) => !o && close()} title="Ajustes da comparação">
      <form
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate(v, { onSuccess: close });
        }}
      >
        <Field label="Máximo de mercados por compra" htmlFor="settings-markets">
          <Input id="settings-markets" type="number" inputMode="numeric" min={1} max={5} value={v.maxMarkets} onChange={(e) => setDraft({ ...v, maxMarkets: Number(e.target.value) })} />
        </Field>
        <Field label="Economia mínima para incluir mais um mercado" htmlFor="settings-savings">
          <MoneyInput id="settings-savings" value={v.minSavingsCents} onChange={(c) => setDraft({ ...v, minSavingsCents: c })} />
        </Field>
        <Field label="Preço desatualizado depois de quantos dias" htmlFor="settings-days">
          <Input id="settings-days" type="number" inputMode="numeric" min={1} max={365} value={v.staleDays} onChange={(e) => setDraft({ ...v, staleDays: Number(e.target.value) })} />
        </Field>
        <FieldError message={save.error?.message} />
        <DialogActions>
          <Button variant="ghost" onClick={close}>
            Cancelar
          </Button>
          <Button type="submit" disabled={save.isPending}>
            Salvar
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}
