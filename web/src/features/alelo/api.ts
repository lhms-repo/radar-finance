import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, unwrap, type components } from "@/api/client";
import { haptic } from "@/lib/haptics";

export type AleloCard = components["schemas"]["AleloCard"];
export type AleloMonth = components["schemas"]["AleloMonth"];

/** Loads the Alelo cards with their current balance (RF-31, RF-34). */
export function useAleloCards() {
  return useQuery({ queryKey: ["alelo", "list"], queryFn: async () => unwrap(await api.GET("/alelo")) });
}

/** Loads the month of an Alelo card (RN-15). */
export function useAleloMonth(id: number | undefined, month: string) {
  return useQuery({
    queryKey: ["alelo", "month", id, month],
    queryFn: async () => unwrap(await api.GET("/alelo/{id}/months/{month}", { params: { path: { id: id!, month } } })),
    enabled: id !== undefined,
    placeholderData: (prev) => prev,
  });
}

/** Wraps an Alelo write with haptics, a toast and a refresh of the Alelo data. */
function useAleloMutation<V, R>(fn: (vars: V) => Promise<R>, success: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      haptic();
      toast.success(success);
      void qc.invalidateQueries({ queryKey: ["alelo"] });
    },
  });
}

/** Sends a write that answers 204 and throws the server message on failure. */
async function noContent(p: Promise<{ response: Response; error?: unknown }>) {
  const res = await p;
  if (!res.response.ok) unwrap({ ...res, data: undefined });
}

/** Registers an Alelo card (RF-31). */
export function useCreateAlelo() {
  return useAleloMutation(async (body: components["schemas"]["AleloInput"]) => unwrap(await api.POST("/alelo", { body })), "Cartão Alelo criado");
}

/** Edits an Alelo card. */
export function useUpdateAlelo() {
  return useAleloMutation(
    async ({ id, body }: { id: number; body: components["schemas"]["AleloUpdate"] }) => unwrap(await api.PUT("/alelo/{id}", { params: { path: { id } }, body })),
    "Cartão Alelo atualizado",
  );
}

/** Deletes an Alelo card without history. */
export function useDeleteAlelo() {
  return useAleloMutation((id: number) => noContent(api.DELETE("/alelo/{id}", { params: { path: { id } } })), "Cartão Alelo excluído");
}

/** Confirms the monthly credit (RF-32). */
export function useConfirmAleloCredit() {
  return useAleloMutation(
    async ({ id, month, body }: { id: number; month: string; body: components["schemas"]["AleloCreditCreate"] }) =>
      unwrap(await api.POST("/alelo/{id}/months/{month}/credit", { params: { path: { id, month } }, body })),
    "Crédito confirmado",
  );
}

/** Undoes the confirmation of the monthly credit. */
export function useUndoAleloCredit() {
  return useAleloMutation(
    ({ id, month }: { id: number; month: string }) => noContent(api.DELETE("/alelo/{id}/months/{month}/credit", { params: { path: { id, month } } })),
    "Crédito desfeito",
  );
}

/** Records a purchase with a free description (RF-33). */
export function useAddAleloPurchase() {
  return useAleloMutation(
    async ({ id, body }: { id: number; body: components["schemas"]["AleloPurchaseCreate"] }) =>
      unwrap(await api.POST("/alelo/{id}/purchases", { params: { path: { id } }, body })),
    "Compra no Alelo lançada",
  );
}

/** Deletes an Alelo purchase. */
export function useDeleteAleloPurchase() {
  return useAleloMutation((id: number) => noContent(api.DELETE("/alelo-purchases/{id}", { params: { path: { id } } })), "Compra excluída");
}
