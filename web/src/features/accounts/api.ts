import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, unwrap, type components } from "@/api/client";
import { haptic } from "@/lib/haptics";

export type Person = components["schemas"]["Person"];
export type Account = components["schemas"]["Account"];
export type AccountCreate = components["schemas"]["AccountCreate"];
export type AccountUpdate = components["schemas"]["AccountUpdate"];
export type Entry = components["schemas"]["Entry"];
export type Statement = components["schemas"]["Statement"];

/** Query keys refreshed by every account write. */
export const ACCOUNT_KEYS = [["accounts"], ["statement"], ["month-flow"], ["incomes"]] as const;

/** Loads the people of the couple, holders of accounts. */
export function usePersons() {
  return useQuery({
    queryKey: ["persons"],
    queryFn: async () => unwrap(await api.GET("/persons")),
    staleTime: Infinity,
  });
}

/** Loads every account with its current balance (RF-05). */
export function useAccounts() {
  return useQuery({ queryKey: ["accounts"], queryFn: async () => unwrap(await api.GET("/accounts")) });
}

/** Loads one account with its current balance. */
export function useAccount(id: number) {
  return useQuery({
    queryKey: ["accounts", id],
    queryFn: async () => unwrap(await api.GET("/accounts/{id}", { params: { path: { id } } })),
  });
}

/** Loads the entries of an account in a month (RF-05). */
export function useStatement(id: number, month: string) {
  return useQuery({
    queryKey: ["statement", id, month],
    queryFn: async () =>
      unwrap(await api.GET("/accounts/{id}/statement", { params: { path: { id }, query: { month } } })),
    placeholderData: (prev) => prev,
  });
}

/** Wraps an account write with haptics, a toast and a refresh of balances and statements. */
function useAccountMutation<V, R>(fn: (vars: V) => Promise<R>, success: string) {
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

/** Sends a write that answers 204 and throws the server message on failure. */
async function noContent(p: Promise<{ response: Response; error?: unknown }>) {
  const res = await p;
  if (!res.response.ok) unwrap({ ...res, data: undefined });
}

/** Registers an account (RF-04). */
export function useCreateAccount() {
  return useAccountMutation(async (body: AccountCreate) => unwrap(await api.POST("/accounts", { body })), "Conta criada");
}

/** Edits an account. */
export function useUpdateAccount(success = "Conta atualizada") {
  return useAccountMutation(
    async ({ id, body }: { id: number; body: AccountUpdate }) =>
      unwrap(await api.PATCH("/accounts/{id}", { params: { path: { id } }, body })),
    success,
  );
}

/** Deletes an account without entries. */
export function useDeleteAccount() {
  return useAccountMutation(
    (id: number) => noContent(api.DELETE("/accounts/{id}", { params: { path: { id } } })),
    "Conta excluída",
  );
}

/** Sets the balance to the value the bank shows (RF-06). */
export function useAdjustBalance() {
  return useAccountMutation(
    async ({ id, body }: { id: number; body: components["schemas"]["AdjustmentCreate"] }) =>
      unwrap(await api.POST("/accounts/{id}/adjustments", { params: { path: { id } }, body })),
    "Saldo ajustado",
  );
}

/** Records a one off expense from the account. */
export function useAddExpense() {
  return useAccountMutation(
    async ({ id, body }: { id: number; body: components["schemas"]["ExpenseCreate"] }) =>
      unwrap(await api.POST("/accounts/{id}/expenses", { params: { path: { id } }, body })),
    "Saída lançada",
  );
}

/** Moves money between two accounts (RF-07). */
export function useTransfer() {
  return useAccountMutation(
    async (body: components["schemas"]["TransferCreate"]) => unwrap(await api.POST("/transfers", { body })),
    "Transferência feita",
  );
}

/** Deletes an entry, or both sides of a transfer. */
export function useDeleteEntry() {
  return useAccountMutation(
    (id: number) => noContent(api.DELETE("/entries/{id}", { params: { path: { id } } })),
    "Lançamento excluído",
  );
}
