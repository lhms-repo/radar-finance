import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, unwrap, type components } from "@/api/client";
import { haptic } from "@/lib/haptics";

export type Overview = components["schemas"]["Overview"];
export type ForecastGroup = components["schemas"]["ForecastGroup"];
export type MonthSpend = components["schemas"]["MonthSpend"];
export type BudgetStatus = components["schemas"]["BudgetStatus"];
export type Notification = components["schemas"]["Notification"];
export type SearchLine = components["schemas"]["SearchLine"];

/** Loads the indicators of the month: forecast, housing, closing, net worth and commitments (RF-58, RF-60 to RF-62). */
export function useOverview(month: string) {
  return useQuery({
    queryKey: ["overview", month],
    queryFn: async () => unwrap(await api.GET("/overview", { params: { query: { month } } })),
    placeholderData: keepPreviousData,
  });
}

/** Loads the spending of each month in the range, by category (RF-59). */
export function useSpending(from: string, to: string) {
  return useQuery({
    queryKey: ["spending", from, to],
    queryFn: async () => unwrap(await api.GET("/reports/spending", { params: { query: { from, to } } })),
    placeholderData: keepPreviousData,
  });
}

/** Loads the budgets compared with the spending of the month (RF-51, RF-52). */
export function useBudgets(month: string) {
  return useQuery({
    queryKey: ["budgets", month],
    queryFn: async () => unwrap(await api.GET("/budgets", { params: { query: { month } } })),
    placeholderData: keepPreviousData,
  });
}

/** Sets or removes the budget of a top level category (RF-51). */
export function useSetBudget() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ categoryId, amountCents }: { categoryId: number; amountCents: number }) => {
      const res = await api.PUT("/budgets/{id}", { params: { path: { id: categoryId } }, body: { amountCents } });
      if (!res.response.ok) unwrap({ ...res, data: undefined });
    },
    onSuccess: () => {
      haptic();
      toast.success("Orçamento salvo");
      void qc.invalidateQueries({ queryKey: ["budgets"] });
      void qc.invalidateQueries({ queryKey: ["notifications"] });
    },
  });
}

/** Loads the warnings of the user, refreshing every minute (RF-64). */
export function useNotifications() {
  return useQuery({
    queryKey: ["notifications"],
    queryFn: async () => unwrap(await api.GET("/notifications")),
    refetchInterval: 60_000,
  });
}

/** Marks notifications as read; without keys marks all. */
export function useMarkNotificationsRead() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (keys?: string[]) => {
      const res = await api.POST("/notifications/read", { body: { keys } });
      if (!res.response.ok) unwrap({ ...res, data: undefined });
    },
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["notifications"] }),
  });
}

/** Filters of the entry search (RF-63); zero and empty values are off. */
export type SearchFilter = { from: string; to: string; categoryId: number; accountId: number; cardId: number; personId: number; text: string };

/** Searches entries, card purchases and Alelo purchases (RF-63). */
export function useSearch(f: SearchFilter) {
  return useQuery({
    queryKey: ["search", f],
    queryFn: async () =>
      unwrap(
        await api.GET("/search", {
          params: {
            query: {
              from: f.from,
              to: f.to,
              categoryId: f.categoryId || undefined,
              accountId: f.accountId || undefined,
              cardId: f.cardId || undefined,
              personId: f.personId || undefined,
              text: f.text.trim() || undefined,
            },
          },
        }),
      ),
    placeholderData: keepPreviousData,
  });
}
