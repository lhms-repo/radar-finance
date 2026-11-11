import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, unwrap, type components } from "@/api/client";
import { ACCOUNT_KEYS } from "@/features/accounts/api";
import { haptic } from "@/lib/haptics";

export type Bill = components["schemas"]["Bill"];
export type BillInput = components["schemas"]["BillInput"];
export type BillOccurrence = components["schemas"]["BillOccurrence"];

/** Loads the registered fixed bills (RF-35). */
export function useBills() {
  return useQuery({ queryKey: ["bills", "list"], queryFn: async () => unwrap(await api.GET("/bills")) });
}

/** Loads the bills due in a month (RF-36, RF-37). */
export function useBillMonth(month: string) {
  return useQuery({
    queryKey: ["bills", "month", month],
    queryFn: async () => unwrap(await api.GET("/bills/month", { params: { query: { month } } })),
    placeholderData: (prev) => prev,
  });
}

/** Wraps a bill write with haptics, a toast and a refresh of bills and balances. */
function useBillMutation<V, R>(fn: (vars: V) => Promise<R>, success: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      haptic();
      toast.success(success);
      void qc.invalidateQueries({ queryKey: ["bills"] });
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

/** Registers a fixed bill (RF-35). */
export function useCreateBill() {
  return useBillMutation(async (body: BillInput) => unwrap(await api.POST("/bills", { body })), "Conta fixa criada");
}

/** Replaces the fields of a bill. */
export function useUpdateBill() {
  return useBillMutation(
    async ({ id, body }: { id: number; body: BillInput }) => unwrap(await api.PUT("/bills/{id}", { params: { path: { id } }, body })),
    "Conta fixa atualizada",
  );
}

/** Deletes a bill without payments. */
export function useDeleteBill() {
  return useBillMutation((id: number) => noContent(api.DELETE("/bills/{id}", { params: { path: { id } } })), "Conta fixa excluída");
}

/** Informs the real amount of a month; zero clears it (RF-36, RN-14). */
export function useSetBillActual() {
  return useBillMutation(
    ({ id, month, actualCents }: { id: number; month: string; actualCents: number }) =>
      noContent(api.PUT("/bills/{id}/months/{month}/actual", { params: { path: { id, month } }, body: { actualCents } })),
    "Valor real salvo",
  );
}

/** Pays the bill of a month (RF-37). */
export function usePayBill() {
  return useBillMutation(
    async ({ id, month, body }: { id: number; month: string; body: components["schemas"]["BillPaymentCreate"] }) =>
      unwrap(await api.POST("/bills/{id}/months/{month}/payment", { params: { path: { id, month } }, body })),
    "Conta paga",
  );
}
