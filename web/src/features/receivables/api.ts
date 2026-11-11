import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, unwrap, type components } from "@/api/client";
import { ACCOUNT_KEYS } from "@/features/accounts/api";
import { haptic } from "@/lib/haptics";

export type ReceivablePerson = components["schemas"]["ReceivablePerson"];
export type ReceivableItem = components["schemas"]["ReceivableItem"];

/** Refreshes everything a receipt touches: receivables, invoices and balances. */
function invalidateAll(qc: ReturnType<typeof useQueryClient>) {
  for (const key of [["receivables"], ["cards"], ...ACCOUNT_KEYS]) void qc.invalidateQueries({ queryKey: key });
}

/** Loads what other people owe, by person (RF-28). */
export function useReceivables() {
  return useQuery({ queryKey: ["receivables"], queryFn: async () => unwrap(await api.GET("/receivables")) });
}

/** Marks shares of one person as paid back into an account (RF-26, RF-30). */
export function useMarkReceived() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: components["schemas"]["ReceiptCreate"]) => unwrap(await api.POST("/receivables/receipts", { body })),
    onSuccess: () => {
      haptic();
      toast.success("Recebimento registrado");
      invalidateAll(qc);
    },
  });
}

/** Undoes a receipt by deleting its account entry. */
export function useUndoReceipt() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (entryId: number) => {
      const res = await api.DELETE("/entries/{id}", { params: { path: { id: entryId } } });
      if (!res.response.ok) unwrap({ ...res, data: undefined });
    },
    onSuccess: () => {
      haptic();
      toast.success("Recebimento desfeito");
      invalidateAll(qc);
    },
  });
}

/** Loads the charge message for a person (RF-29). */
export async function fetchChargeMessage(partyId: number) {
  return unwrap(await api.GET("/receivables/{id}/message", { params: { path: { id: partyId } } })).text;
}
