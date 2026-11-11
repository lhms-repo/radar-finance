import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, unwrap, type components } from "@/api/client";
import { ACCOUNT_KEYS } from "@/features/accounts/api";
import { haptic } from "@/lib/haptics";

export type SavingsBox = components["schemas"]["SavingsBox"];

/** Loads the boxes as seen in the month, with the base of the suggestions (RF-45, RF-48). */
export function useSavingsMonth(month: string) {
  return useQuery({
    queryKey: ["savings", month],
    queryFn: async () => unwrap(await api.GET("/savings", { params: { query: { month } } })),
    placeholderData: (prev) => prev,
  });
}

/** Wraps a savings write with haptics, a toast and a refresh of boxes and balances. */
function useSavingsMutation<V, R>(fn: (vars: V) => Promise<R>, success: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      haptic();
      toast.success(success);
      void qc.invalidateQueries({ queryKey: ["savings"] });
      for (const key of ACCOUNT_KEYS) void qc.invalidateQueries({ queryKey: key });
    },
  });
}

/** Creates a box (RF-43). */
export function useCreateSavings() {
  return useSavingsMutation(async (body: components["schemas"]["SavingsInput"]) => unwrap(await api.POST("/savings", { body })), "Caixinha criada");
}

/** Edits a box (RF-44). */
export function useUpdateSavings() {
  return useSavingsMutation(
    async ({ id, body }: { id: number; body: components["schemas"]["SavingsUpdate"] }) => unwrap(await api.PUT("/savings/{id}", { params: { path: { id } }, body })),
    "Caixinha atualizada",
  );
}

/** Deletes a box without moves. */
export function useDeleteSavings() {
  return useSavingsMutation(async (id: number) => {
    const res = await api.DELETE("/savings/{id}", { params: { path: { id } } });
    if (!res.response.ok) unwrap({ ...res, data: undefined });
  }, "Caixinha excluída");
}

/** Confirms a deposit from an account (RF-46). */
export function useDepositSavings() {
  return useSavingsMutation(
    async ({ id, body }: { id: number; body: components["schemas"]["SavingsDepositCreate"] }) =>
      unwrap(await api.POST("/savings/{id}/deposits", { params: { path: { id } }, body })),
    "Aporte confirmado",
  );
}

/** Takes money out of a box (RF-47). */
export function useWithdrawSavings() {
  return useSavingsMutation(
    async ({ id, body }: { id: number; body: components["schemas"]["SavingsWithdrawalCreate"] }) =>
      unwrap(await api.POST("/savings/{id}/withdrawals", { params: { path: { id } }, body })),
    "Retirada registrada",
  );
}

/** Undoes a deposit or withdrawal by deleting its account entry. */
export function useUndoSavingsMove() {
  return useSavingsMutation(async (entryId: number) => {
    const res = await api.DELETE("/entries/{id}", { params: { path: { id: entryId } } });
    if (!res.response.ok) unwrap({ ...res, data: undefined });
  }, "Movimento desfeito");
}

/** Formats basis points as a Brazilian percentage, like 12,5%. */
export function formatPercent(bp: number) {
  return `${(bp / 100).toLocaleString("pt-BR", { maximumFractionDigits: 2 })}%`;
}
