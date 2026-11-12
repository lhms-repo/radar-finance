import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, unwrap, type components } from "@/api/client";
import { ACCOUNT_KEYS } from "@/features/accounts/api";
import { haptic } from "@/lib/haptics";

export type AliasRule = components["schemas"]["AliasRule"];
export type ImportPreviewLine = components["schemas"]["ImportPreviewLine"];
export type ImportCommitLine = components["schemas"]["ImportCommitLine"];
export type ImportRecord = components["schemas"]["ImportRecord"];

/** Refreshes what an import touches: cards, rules, history and every indicator. */
function invalidateAll(qc: ReturnType<typeof useQueryClient>) {
  // Imports can split lines with other people, so receivables and the known names refresh too.
  for (const key of [["cards"], ["alias-rules"], ["imports"], ["receivables"], ["parties"], ["search"], ...ACCOUNT_KEYS]) void qc.invalidateQueries({ queryKey: key });
}

/** Loads the alias rules (RF-54). */
export function useAliasRules() {
  return useQuery({ queryKey: ["alias-rules"], queryFn: async () => unwrap(await api.GET("/alias-rules")) });
}

/** Loads the import history. */
export function useImports() {
  return useQuery({ queryKey: ["imports"], queryFn: async () => unwrap(await api.GET("/imports")) });
}

/** Reads the CSV on the server and returns the lines with rules and duplicates (RF-53, RF-56). */
export function usePreviewImport() {
  return useMutation({
    mutationFn: async (body: components["schemas"]["ImportPreviewRequest"]) => unwrap(await api.POST("/imports/preview", { body })),
  });
}

/** Records the confirmed lines (RF-53, RF-55, RF-57). */
export function useCommitImport() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: components["schemas"]["ImportCommitRequest"]) => unwrap(await api.POST("/imports", { body })),
    onSuccess: (r) => {
      haptic();
      toast.success("Fatura importada", { description: `${r.created} lançamentos, ${r.skipped} ignorados, ${r.rulesSaved} regras novas.` });
      invalidateAll(qc);
    },
  });
}

/** Removes every purchase of an import. */
export function useUndoImport() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: number) => {
      const res = await api.DELETE("/imports/{id}", { params: { path: { id } } });
      if (!res.response.ok) unwrap({ ...res, data: undefined });
    },
    onSuccess: () => {
      haptic();
      toast.success("Importação desfeita");
      invalidateAll(qc);
    },
  });
}

/** Saves an alias rule, new or existing (RF-54). */
export function useSaveRule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, body }: { id?: number; body: components["schemas"]["AliasRuleInput"] }) =>
      id ? unwrap(await api.PUT("/alias-rules/{id}", { params: { path: { id } }, body })) : unwrap(await api.POST("/alias-rules", { body })),
    onSuccess: () => {
      haptic();
      toast.success("Regra salva");
      void qc.invalidateQueries({ queryKey: ["alias-rules"] });
    },
  });
}

/** Deletes an alias rule. */
export function useDeleteRule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: number) => {
      const res = await api.DELETE("/alias-rules/{id}", { params: { path: { id } } });
      if (!res.response.ok) unwrap({ ...res, data: undefined });
    },
    onSuccess: () => {
      haptic();
      toast.success("Regra excluída");
      void qc.invalidateQueries({ queryKey: ["alias-rules"] });
    },
  });
}
