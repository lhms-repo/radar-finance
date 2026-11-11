import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, unwrap, type components } from "@/api/client";
import { haptic } from "@/lib/haptics";

export type Market = components["schemas"]["Market"];
export type MarketProduct = components["schemas"]["MarketProduct"];
export type MarketPrice = components["schemas"]["MarketPrice"];
export type ProductUnit = components["schemas"]["ProductUnit"];
export type MarketSettings = components["schemas"]["MarketSettings"];

/** Every query of the market screen, refreshed after any write. */
export const MARKET_KEYS = [["market"]] as const;

/** Loads the markets (RF-65). */
export function useMarkets() {
  return useQuery({ queryKey: ["market", "markets"], queryFn: async () => unwrap(await api.GET("/market/markets")) });
}

/** Loads the products with the latest price in each market (RF-66, RN-22, RN-25). */
export function useProducts() {
  return useQuery({ queryKey: ["market", "products"], queryFn: async () => unwrap(await api.GET("/market/products")) });
}

/** Loads the price history of a product (RF-76). */
export function useProductPrices(id: number | undefined) {
  return useQuery({
    queryKey: ["market", "prices", id],
    queryFn: async () => unwrap(await api.GET("/market/products/{id}/prices", { params: { path: { id: id! } } })),
    enabled: !!id,
  });
}

/** Loads the comparison settings (RF-74, RN-25). */
export function useMarketSettings() {
  return useQuery({ queryKey: ["market", "settings"], queryFn: async () => unwrap(await api.GET("/market/settings")) });
}

/** Wraps a market write with haptics, a toast and a refresh of the market screen. */
export function useMarketMutation<V, R>(fn: (vars: V) => Promise<R>, success: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      haptic();
      toast.success(success);
      for (const key of MARKET_KEYS) void qc.invalidateQueries({ queryKey: key });
    },
  });
}

/** Sends a request without a response body, raising its error. */
async function noContent(res: { response: Response; error?: unknown }) {
  if (!res.response.ok) unwrap({ ...res, data: undefined });
}

/** Creates or edits a market (RF-65). */
export function useSaveMarket() {
  return useMarketMutation(
    async ({ id, body }: { id?: number; body: components["schemas"]["MarketInput"] }) =>
      id ? unwrap(await api.PUT("/market/markets/{id}", { params: { path: { id } }, body })) : unwrap(await api.POST("/market/markets", { body })),
    "Mercado salvo",
  );
}

/** Deletes a market without prices. */
export function useDeleteMarket() {
  return useMarketMutation(async (id: number) => noContent(await api.DELETE("/market/markets/{id}", { params: { path: { id } } })), "Mercado excluído");
}

/** Creates or edits a product (RF-66). */
export function useSaveProduct() {
  return useMarketMutation(
    async ({ id, body }: { id?: number; body: components["schemas"]["ProductInput"] }) =>
      id ? unwrap(await api.PUT("/market/products/{id}", { params: { path: { id } }, body })) : unwrap(await api.POST("/market/products", { body })),
    "Produto salvo",
  );
}

/** Deletes a product without prices. */
export function useDeleteProduct() {
  return useMarketMutation(async (id: number) => noContent(await api.DELETE("/market/products/{id}", { params: { path: { id } } })), "Produto excluído");
}

/** Records a price typed by hand (RF-67, RF-70). */
export function useAddPrice() {
  return useMarketMutation(async (body: components["schemas"]["PriceInput"]) => noContent(await api.POST("/market/prices", { body })), "Preço registrado");
}

/** Corrects a price (RF-70). */
export function useUpdatePrice() {
  return useMarketMutation(
    async ({ id, body }: { id: number; body: components["schemas"]["PriceChange"] }) => noContent(await api.PUT("/market/prices/{id}", { params: { path: { id } }, body })),
    "Preço corrigido",
  );
}

/** Deletes a wrong price (RF-70). */
export function useDeletePrice() {
  return useMarketMutation(async (id: number) => noContent(await api.DELETE("/market/prices/{id}", { params: { path: { id } } })), "Preço excluído");
}

/** Saves the comparison settings (RF-74, RN-25). */
export function useSaveMarketSettings() {
  return useMarketMutation(async (body: MarketSettings) => noContent(await api.PUT("/market/settings", { body })), "Ajustes salvos");
}

/** Names of the units as shown in the forms. */
export const UNITS: { value: ProductUnit; label: string }[] = [
  { value: "un", label: "un" },
  { value: "g", label: "g" },
  { value: "kg", label: "kg" },
  { value: "ml", label: "ml" },
  { value: "l", label: "L" },
];

/** Converts a size typed with a comma, such as 1,5, to thousandths; returns 0 when invalid. */
export function parseSizeMilli(text: string): number {
  const m = /^(\d{1,7})(?:[.,](\d{1,3}))?$/.exec(text.trim());
  if (!m) return 0;
  return Number(m[1]) * 1000 + Number((m[2] ?? "").padEnd(3, "0") || 0);
}

/** Writes thousandths back as the size typed by the user, such as 1,5. */
export function formatSizeMilli(milli: number): string {
  const whole = Math.floor(milli / 1000);
  const frac = milli % 1000;
  return frac ? `${whole},${String(frac).padStart(3, "0").replace(/0+$/, "")}` : String(whole);
}

export type ReceiptPreview = components["schemas"]["ReceiptPreview"];
export type ReceiptLine = components["schemas"]["ReceiptLine"];
export type ReceiptCommit = components["schemas"]["ReceiptCommit"];
export type ReceiptRecord = components["schemas"]["ReceiptRecord"];

/** Downloads the receipt of the QR code and shows the learned links (RF-68, RF-69). */
export function usePreviewReceipt() {
  return useMutation({
    mutationFn: async (body: components["schemas"]["ReceiptPreviewRequest"]) => unwrap(await api.POST("/market/receipts/preview", { body })),
  });
}

/** Records the receipt prices and the learned links (RF-68, RF-69). */
export function useCommitReceipt() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: ReceiptCommit) => unwrap(await api.POST("/market/receipts", { body })),
    onSuccess: (r) => {
      haptic();
      toast.success("Nota registrada", { description: `${r.prices} ${r.prices === 1 ? "preço novo" : "preços novos"}, ${r.ignored} ${r.ignored === 1 ? "linha ignorada" : "linhas ignoradas"}.` });
      for (const key of MARKET_KEYS) void qc.invalidateQueries({ queryKey: key });
    },
  });
}

/** Loads the receipts already read. */
export function useReceipts() {
  return useQuery({ queryKey: ["market", "receipts"], queryFn: async () => unwrap(await api.GET("/market/receipts")) });
}

/** Removes a receipt and its prices. */
export function useDeleteReceipt() {
  return useMarketMutation(async (id: number) => noContent(await api.DELETE("/market/receipts/{id}", { params: { path: { id } } })), "Nota removida");
}

/** Writes a receipt quantity in thousandths, such as 1,288 kg or 2 un. */
export function formatQuantity(milli: number, unit: string): string {
  return `${formatSizeMilli(milli)} ${unit.toLowerCase()}`;
}
