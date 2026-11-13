import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { spaceHeader, type SpaceTarget } from "@/app/space";
import { toast } from "sonner";
import { api, unwrap, type components } from "@/api/client";
import { ACCOUNT_KEYS } from "@/features/accounts/api";
import { haptic } from "@/lib/haptics";

export type Financing = components["schemas"]["Financing"];
export type FinancingInput = components["schemas"]["FinancingInput"];

/** Loads the financings with paid and remaining installments and balance (RF-39, RF-41). */
export function useFinancings() {
  return useQuery({ queryKey: ["financings", "list"], queryFn: async () => unwrap(await api.GET("/financings")) });
}

/** Wraps a financing write with haptics, a toast and a refresh of financings, agenda and balances. */
function useFinancingMutation<V, R>(fn: (vars: V) => Promise<R>, success: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      haptic();
      toast.success(success);
      void qc.invalidateQueries({ queryKey: ["financings"] });
      void qc.invalidateQueries({ queryKey: ["agenda"] });
      for (const key of ACCOUNT_KEYS) void qc.invalidateQueries({ queryKey: key });
    },
  });
}

/** Registers a financing (RF-39). */
export function useCreateFinancing() {
  return useFinancingMutation(
    async ({ space, ...body }: FinancingInput & SpaceTarget) => unwrap(await api.POST("/financings", { params: { header: spaceHeader(space) }, body })),
    "Financiamento criado",
  );
}

/** Edits name, category, account and due day. */
export function useUpdateFinancing() {
  return useFinancingMutation(
    async ({ id, body }: { id: number; body: components["schemas"]["FinancingUpdate"] }) =>
      unwrap(await api.PUT("/financings/{id}", { params: { path: { id } }, body })),
    "Financiamento atualizado",
  );
}

/** Deletes a financing without payments. */
export function useDeleteFinancing() {
  return useFinancingMutation(async (id: number) => {
    const res = await api.DELETE("/financings/{id}", { params: { path: { id } } });
    if (!res.response.ok) unwrap({ ...res, data: undefined });
  }, "Financiamento excluído");
}

/** Pays the installment of a month with the balance after it (RF-41, RN-16). */
export function usePayFinancing() {
  return useFinancingMutation(
    async ({ id, month, body }: { id: number; month: string; body: components["schemas"]["FinancingPaymentCreate"] }) =>
      unwrap(await api.POST("/financings/{id}/months/{month}/payment", { params: { path: { id, month } }, body })),
    "Parcela paga",
  );
}

/** Sets the balance and the installment by hand (RF-42). */
export function useCorrectFinancing() {
  return useFinancingMutation(
    async ({ id, body }: { id: number; body: components["schemas"]["FinancingCorrection"] }) =>
      unwrap(await api.POST("/financings/{id}/corrections", { params: { path: { id } }, body })),
    "Saldo corrigido",
  );
}
