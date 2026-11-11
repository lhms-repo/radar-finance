import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router";
import { api, unwrap, type Me, type Theme } from "@/api/client";

export const ME_KEY = ["me"] as const;

/** Fetches the logged in user, returning null when there is no session. */
export async function fetchMe(): Promise<Me | null> {
  const res = await api.GET("/me");
  if (res.response.status === 401) return null;
  return unwrap(res);
}

/** Query of the logged in user. */
export function useMeQuery() {
  return useQuery({ queryKey: ME_KEY, queryFn: fetchMe, staleTime: 5 * 60_000, retry: false });
}

/** Returns the logged in user once loaded. */
export function useMe(): Me | null {
  return useMeQuery().data ?? null;
}

/** Signs out and goes back to the login screen. */
export function useLogout() {
  const qc = useQueryClient();
  const navigate = useNavigate();
  return useMutation({
    mutationFn: async () => {
      await api.POST("/auth/logout");
    },
    onSettled: () => {
      qc.clear();
      qc.setQueryData(ME_KEY, null);
      navigate("/login", { replace: true });
    },
  });
}

/** Saves theme and haptics preferences on the server. */
export function useUpdatePreferences() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: { theme?: Theme; haptics?: boolean }) =>
      unwrap(await api.PATCH("/me/preferences", { body })),
    onSuccess: (me) => qc.setQueryData(ME_KEY, me),
  });
}
