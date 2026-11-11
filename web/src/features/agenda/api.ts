import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, unwrap, type components } from "@/api/client";
import { ACCOUNT_KEYS } from "@/features/accounts/api";
import { haptic } from "@/lib/haptics";

export type Agenda = components["schemas"]["Agenda"];
export type AgendaItem = components["schemas"]["AgendaItem"];

/** Loads everything due in the month, pending first (RF-38). */
export function useAgenda(month: string) {
  return useQuery({
    queryKey: ["agenda", month],
    queryFn: async () => unwrap(await api.GET("/agenda", { params: { query: { month } } })),
    placeholderData: (prev) => prev,
  });
}

/** Undoes a payment from the agenda by deleting its account entry. */
export function useUndoAgendaPayment() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (entryId: number) => {
      const res = await api.DELETE("/entries/{id}", { params: { path: { id: entryId } } });
      if (!res.response.ok) unwrap({ ...res, data: undefined });
    },
    onSuccess: () => {
      haptic();
      toast.success("Pagamento desfeito");
      for (const key of [["agenda"], ["bills"], ["financings"], ["cards"], ...ACCOUNT_KEYS]) void qc.invalidateQueries({ queryKey: key });
    },
  });
}
