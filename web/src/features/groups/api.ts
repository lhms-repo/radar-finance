import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, unwrap, type components } from "@/api/client";
import { ME_KEY } from "@/app/session";
import { haptic } from "@/lib/haptics";

export type GroupDetail = components["schemas"]["GroupDetail"];
export type GroupMember = components["schemas"]["GroupMember"];
export type GroupRole = components["schemas"]["GroupRole"];

/** Query key of the active group and its people. */
export const GROUP_KEY = ["group"] as const;

/** Loads the active group with its members (RF-79). */
export function useGroup() {
  return useQuery({ queryKey: GROUP_KEY, queryFn: async () => unwrap(await api.GET("/group")) });
}

/** Wraps a group write: haptics, a toast, and fresh group, user and people lists, since names and roles show there. */
function useGroupMutation<V, R>(fn: (vars: V) => Promise<R>, success: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      haptic();
      toast.success(success);
      for (const key of [GROUP_KEY, ME_KEY, ["persons"]]) void qc.invalidateQueries({ queryKey: key });
    },
  });
}

/** Renames the active group; administrators only (RF-80). */
export function useRenameGroup() {
  return useGroupMutation(async (name: string) => unwrap(await api.PUT("/group", { body: { name } })), "Grupo renomeado");
}

/** Creates another group with the user as its administrator (RF-78, RF-80). */
export function useCreateGroup() {
  return useGroupMutation(async (name: string) => unwrap(await api.POST("/groups", { body: { name } })), "Grupo criado");
}

/** Adds a person already registered, by login, to the active group (RF-80). */
export function useAddMember() {
  return useGroupMutation(
    async (body: components["schemas"]["GroupMemberAdd"]) => unwrap(await api.POST("/group/members", { body })),
    "Pessoa adicionada",
  );
}

/** Registers a new person with login and PIN in the active group (RF-80, RN-27). */
export function useCreatePerson() {
  return useGroupMutation(
    async (body: components["schemas"]["GroupPersonCreate"]) => unwrap(await api.POST("/group/people", { body })),
    "Pessoa cadastrada",
  );
}

/** Changes the role of a member of the active group (RF-80). */
export function useSetRole() {
  return useGroupMutation(
    async ({ userId, role }: { userId: number; role: GroupRole }) =>
      unwrap(await api.PUT("/group/members/{id}", { params: { path: { id: userId } }, body: { role } })),
    "Papel alterado",
  );
}

/** Sets a new PIN for a member of the active group (RF-80, RN-27). */
export function useResetPin() {
  return useGroupMutation(async ({ userId, pin }: { userId: number; pin: string }) => {
    const res = await api.PUT("/group/members/{id}/pin", { params: { path: { id: userId } }, body: { pin } });
    if (!res.response.ok) unwrap({ ...res, data: undefined });
  }, "PIN redefinido");
}

/** Sets the weight of every member in the settlement, from today on; administrators only (RF-79, RN-28). */
export function useSetWeights() {
  return useGroupMutation(
    async (weights: { userId: number; weightBp: number }[]) => unwrap(await api.PUT("/group/weights", { body: { weights } })),
    "Pesos salvos",
  );
}
