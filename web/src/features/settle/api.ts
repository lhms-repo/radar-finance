import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, unwrap, type components } from "@/api/client";
import { haptic } from "@/lib/haptics";

export type Settlement = components["schemas"]["Settlement"];
export type SettlementLine = components["schemas"]["SettlementLine"];
export type SettlementSuggestion = components["schemas"]["SettlementSuggestion"];
export type MemberSplit = components["schemas"]["MemberSplit"];
export type MemberSplitKind = components["schemas"]["MemberSplitKind"];

/** Loads the settlement of the active group, with the expenses of the month (RF-84). */
export function useSettlement(month: string, enabled = true) {
  return useQuery({
    queryKey: ["settlement", month],
    queryFn: async () => unwrap(await api.GET("/settlement", { params: { query: { month } } })),
    placeholderData: (prev) => prev,
    enabled,
  });
}

/** Wraps a settlement write with haptics, a toast and fresh balances and accounts. */
function useSettleMutation<V, R>(fn: (vars: V) => Promise<R>, success: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      haptic();
      toast.success(success);
      for (const key of [["settlement"], ["accounts"], ["statement"]]) void qc.invalidateQueries({ queryKey: key });
    },
  });
}

/** Records a payment between members as a transfer between their accounts (RN-30). */
export function useSettlementPayment() {
  return useSettleMutation(
    async (body: components["schemas"]["SettlementPaymentCreate"]) => unwrap(await api.POST("/settlement/payments", { body })),
    "Pagamento de acerto registrado",
  );
}

/** Undoes a payment between members. */
export function useDeleteSettlementPayment() {
  return useSettleMutation(async (id: number) => {
    const res = await api.DELETE("/settlement/payments/{id}", { params: { path: { id } } });
    if (!res.response.ok) unwrap({ ...res, data: undefined });
  }, "Pagamento desfeito");
}

/** Loads the division between members of one expense, for its edit form (RN-28). */
export function useMemberSplit(kind: MemberSplitKind, id: number | undefined) {
  return useQuery({
    queryKey: ["member-split", kind, id],
    queryFn: async () => unwrap(await api.GET("/member-splits/{kind}/{id}", { params: { path: { kind, id: id! } } })),
    enabled: id !== undefined,
    // Another tab or person may have changed it; a form always starts from what is saved now.
    staleTime: 0,
    refetchOnMount: "always",
  });
}
