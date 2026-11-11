import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, unwrap, type components } from "@/api/client";
import { ACCOUNT_KEYS } from "@/features/accounts/api";
import { haptic } from "@/lib/haptics";

export type Income = components["schemas"]["Income"];
export type IncomeInput = components["schemas"]["IncomeInput"];
export type IncomeMonth = components["schemas"]["IncomeMonth"];
export type IncomeOccurrence = components["schemas"]["IncomeOccurrence"];

/** Loads the registered incomes (RF-09, RF-11). */
export function useIncomes() {
  return useQuery({ queryKey: ["incomes", "list"], queryFn: async () => unwrap(await api.GET("/incomes")) });
}

/** Loads the expected and received incomes of a month (RF-10, RN-13). */
export function useIncomeMonth(month: string) {
  return useQuery({
    queryKey: ["incomes", "month", month],
    queryFn: async () => unwrap(await api.GET("/incomes/month", { params: { query: { month } } })),
    placeholderData: (prev) => prev,
  });
}

/** Wraps an income write with haptics, a toast and a refresh of incomes and balances. */
function useIncomeMutation<V, R>(fn: (vars: V) => Promise<R>, success: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      haptic();
      toast.success(success);
      for (const key of ACCOUNT_KEYS) void qc.invalidateQueries({ queryKey: key });
    },
  });
}

/** Registers an income. */
export function useCreateIncome() {
  return useIncomeMutation(async (body: IncomeInput) => unwrap(await api.POST("/incomes", { body })), "Renda cadastrada");
}

/** Replaces the fields of an income (RF-12). */
export function useUpdateIncome() {
  return useIncomeMutation(
    async ({ id, body }: { id: number; body: IncomeInput }) => unwrap(await api.PUT("/incomes/{id}", { params: { path: { id } }, body })),
    "Renda atualizada",
  );
}

/** Deletes an income without receipts. */
export function useDeleteIncome() {
  return useIncomeMutation(async (id: number) => {
    const res = await api.DELETE("/incomes/{id}", { params: { path: { id } } });
    if (!res.response.ok) unwrap({ ...res, data: undefined });
  }, "Renda excluída");
}

/** Confirms the receipt of an income in a month (RF-10, RN-13). */
export function useConfirmIncome() {
  return useIncomeMutation(
    async ({ id, body }: { id: number; body: components["schemas"]["IncomeConfirm"] }) =>
      unwrap(await api.POST("/incomes/{id}/confirm", { params: { path: { id } }, body })),
    "Recebimento confirmado",
  );
}

/** Records a one off income in an account (RF-13). */
export function useAddOneOffIncome() {
  return useIncomeMutation(
    async ({ accountId, body }: { accountId: number; body: components["schemas"]["OneOffIncomeCreate"] }) =>
      unwrap(await api.POST("/accounts/{id}/incomes", { params: { path: { id: accountId } }, body })),
    "Renda lançada",
  );
}
