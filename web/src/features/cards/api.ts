import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, unwrap, type components } from "@/api/client";
import { ACCOUNT_KEYS } from "@/features/accounts/api";
import { haptic } from "@/lib/haptics";

export type Card = components["schemas"]["Card"];
export type CardInput = components["schemas"]["CardInput"];
export type CardColor = components["schemas"]["CardColor"];
export type CardFinish = components["schemas"]["CardFinish"];
export type Invoice = components["schemas"]["Invoice"];
export type InvoiceItem = components["schemas"]["InvoiceItem"];
export type InvoiceSummary = components["schemas"]["InvoiceSummary"];
export type InvoiceStatus = components["schemas"]["InvoiceStatus"];
export type Recurrence = components["schemas"]["Recurrence"];
export type PurchaseInput = components["schemas"]["PurchaseInput"];

/** Loads every card with limit use (RF-14, RF-20). */
export function useCards() {
  return useQuery({ queryKey: ["cards", "list"], queryFn: async () => unwrap(await api.GET("/cards")) });
}

/** Loads the invoice of a card in a due month (RF-19, RN-01). */
export function useInvoice(cardId: number | undefined, month: string) {
  return useQuery({
    queryKey: ["cards", "invoice", cardId, month],
    queryFn: async () => unwrap(await api.GET("/cards/{id}/invoices/{month}", { params: { path: { id: cardId!, month } } })),
    enabled: cardId !== undefined,
    placeholderData: (prev) => prev,
  });
}

/** Loads the totals of the next invoices (RF-23). */
export function useForecast(cardId: number | undefined, count = 6) {
  return useQuery({
    queryKey: ["cards", "forecast", cardId, count],
    queryFn: async () => unwrap(await api.GET("/cards/{id}/forecast", { params: { path: { id: cardId! }, query: { count } } })),
    enabled: cardId !== undefined,
  });
}

/** Loads the recurrences of a card (RF-17). */
export function useRecurrences(cardId: number | undefined) {
  return useQuery({
    queryKey: ["cards", "recurrences", cardId],
    queryFn: async () => unwrap(await api.GET("/cards/{id}/recurrences", { params: { path: { id: cardId! } } })),
    enabled: cardId !== undefined,
  });
}

/** Wraps a card write with haptics, a toast and a refresh of cards, invoices and balances. */
function useCardMutation<V, R>(fn: (vars: V) => Promise<R>, success: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      haptic();
      toast.success(success);
      void qc.invalidateQueries({ queryKey: ["cards"] });
      void qc.invalidateQueries({ queryKey: ["agenda"] });
      for (const key of ACCOUNT_KEYS) void qc.invalidateQueries({ queryKey: key });
    },
  });
}

/** Sends a write that answers 204 and throws the server message on failure. */
async function noContent(p: Promise<{ response: Response; error?: unknown }>) {
  const res = await p;
  if (!res.response.ok) unwrap({ ...res, data: undefined });
}

/** Registers a card (RF-14). */
export function useCreateCard() {
  return useCardMutation(async (body: CardInput) => unwrap(await api.POST("/cards", { body })), "Cartão criado");
}

/** Replaces the fields of a card. */
export function useUpdateCard(success = "Cartão atualizado") {
  return useCardMutation(
    async ({ id, body }: { id: number; body: components["schemas"]["CardUpdate"] }) =>
      unwrap(await api.PUT("/cards/{id}", { params: { path: { id } }, body })),
    success,
  );
}

/** Deletes an unused card. */
export function useDeleteCard() {
  return useCardMutation((id: number) => noContent(api.DELETE("/cards/{id}", { params: { path: { id } } })), "Cartão excluído");
}

/** Registers a purchase (RF-15, RF-16). */
export function useAddPurchase() {
  return useCardMutation(
    async ({ cardId, body }: { cardId: number; body: PurchaseInput }) =>
      unwrap(await api.POST("/cards/{id}/purchases", { params: { path: { id: cardId } }, body })),
    "Compra lançada",
  );
}

/** Replaces a purchase and its installments. */
export function useUpdatePurchase() {
  return useCardMutation(
    async ({ id, body }: { id: number; body: PurchaseInput }) => unwrap(await api.PUT("/purchases/{id}", { params: { path: { id } }, body })),
    "Compra atualizada",
  );
}

/** Deletes a purchase or credit. */
export function useDeletePurchase() {
  return useCardMutation((id: number) => noContent(api.DELETE("/purchases/{id}", { params: { path: { id } } })), "Lançamento excluído");
}

/** Registers a refund or credit (RF-22). */
export function useAddCredit() {
  return useCardMutation(
    async ({ cardId, body }: { cardId: number; body: components["schemas"]["CreditInput"] }) =>
      unwrap(await api.POST("/cards/{id}/credits", { params: { path: { id: cardId } }, body })),
    "Crédito lançado",
  );
}

/** Registers a recurrence (RF-17). */
export function useAddRecurrence() {
  return useCardMutation(
    async ({ cardId, body }: { cardId: number; body: components["schemas"]["RecurrenceInput"] }) =>
      unwrap(await api.POST("/cards/{id}/recurrences", { params: { path: { id: cardId } }, body })),
    "Recorrência criada",
  );
}

/** Edits a recurrence; a new price applies from the current invoice. */
export function useUpdateRecurrence() {
  return useCardMutation(
    async ({ id, body }: { id: number; body: components["schemas"]["RecurrenceUpdate"] }) =>
      unwrap(await api.PUT("/recurrences/{id}", { params: { path: { id } }, body })),
    "Recorrência atualizada",
  );
}

/** Deletes a recurrence that is not on a paid invoice. */
export function useDeleteRecurrence() {
  return useCardMutation((id: number) => noContent(api.DELETE("/recurrences/{id}", { params: { path: { id } } })), "Recorrência excluída");
}

/** Pays an invoice in full from an account (RF-21). */
export function usePayInvoice() {
  return useCardMutation(
    async ({ cardId, month, body }: { cardId: number; month: string; body: components["schemas"]["InvoicePaymentCreate"] }) =>
      unwrap(await api.POST("/cards/{id}/invoices/{month}/payment", { params: { path: { id: cardId, month } }, body })),
    "Fatura paga",
  );
}

/** Undoes an invoice payment by deleting its account entry. */
export function useUndoPayment() {
  return useCardMutation((entryId: number) => noContent(api.DELETE("/entries/{id}", { params: { path: { id: entryId } } })), "Pagamento desfeito");
}
