import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, unwrap, type components } from "@/api/client";
import { haptic } from "@/lib/haptics";

export type Category = components["schemas"]["Category"];
export type CategoryCreate = components["schemas"]["CategoryCreate"];
export type CategoryUpdate = components["schemas"]["CategoryUpdate"];
export type CategoryMove = components["schemas"]["CategoryMove"];

/** Query key shared by every category list. */
export const CATEGORIES_KEY = ["categories"] as const;

/** Loads the category tree; includeInactive is for the management screen (RN-18). */
export function useCategories(includeInactive = false) {
  return useQuery({
    queryKey: [...CATEGORIES_KEY, { includeInactive }],
    queryFn: async () => unwrap(await api.GET("/categories", { params: { query: { includeInactive } } })),
  });
}

/** Wraps a category write with haptics, a toast and a refresh of every list. */
function useCategoryMutation<V>(fn: (vars: V) => Promise<unknown>, success: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      haptic();
      toast.success(success);
      void qc.invalidateQueries({ queryKey: CATEGORIES_KEY });
    },
  });
}

/** Creates a category or subcategory (RF-50). */
export function useCreateCategory() {
  return useCategoryMutation(
    async (body: CategoryCreate) => unwrap(await api.POST("/categories", { body })),
    "Categoria criada",
  );
}

/** Renames, changes the icon or toggles a category (RF-50). */
export function useUpdateCategory(success = "Categoria atualizada") {
  return useCategoryMutation(
    async ({ id, body }: { id: number; body: CategoryUpdate }) =>
      unwrap(await api.PATCH("/categories/{id}", { params: { path: { id } }, body })),
    success,
  );
}

/** Moves a category under another parent or to the top level (RF-50). */
export function useMoveCategory() {
  return useCategoryMutation(
    async ({ id, body }: { id: number; body: CategoryMove }) =>
      unwrap(await api.POST("/categories/{id}/move", { params: { path: { id } }, body })),
    "Categoria movida",
  );
}

/** Deletes an unused category (RN-18). */
export function useDeleteCategory() {
  return useCategoryMutation(async (id: number) => {
    const res = await api.DELETE("/categories/{id}", { params: { path: { id } } });
    if (!res.response.ok) unwrap(res);
  }, "Categoria excluída");
}
