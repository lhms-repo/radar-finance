import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router";
import { api, clientId, setActiveSpace, unwrap, type Me, type Theme } from "@/api/client";

export const ME_KEY = ["me"] as const;

/** Fetches the logged in user, returning null when there is no session. */
export async function fetchMe(): Promise<Me | null> {
  const res = await api.GET("/me");
  if (res.response.status === 401) {
    setActiveSpace(undefined);
    return null;
  }
  const me = unwrap(res);
  setActiveSpace(me.group.id);
  return me;
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
      setActiveSpace(undefined);
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

/** Makes another group of the user the active one; cached data of the old group is dropped so it never shows (RF-81). */
export function useSwitchGroup() {
  const qc = useQueryClient();
  const navigate = useNavigate();
  return useMutation({
    mutationFn: async (groupId: number) => unwrap(await api.PUT("/me/group", { body: { groupId } })),
    onSuccess: (me) => {
      setActiveSpace(me.group.id);
      qc.removeQueries({ predicate: (q) => q.queryKey[0] !== ME_KEY[0] });
      qc.setQueryData(ME_KEY, me);
      announceSessionChange();
      navigate("/", { replace: true });
    },
  });
}

/** Channel the tabs of this browser share to learn that the session changed group or person. */
const SESSION_CHANNEL = "radar-session";

/** Tells the other tabs that the session now acts on another group or person, so they reload instead of showing old data. */
export function announceSessionChange() {
  if (typeof BroadcastChannel === "undefined") return;
  const channel = new BroadcastChannel(SESSION_CHANNEL);
  // The id of this tab goes along: other channels of this same tab also hear it and must not reload it.
  channel.postMessage(clientId);
  channel.close();
}

/** Calls back when another tab changed the group or the person of the session; returns the remover. */
export function onSessionChange(fn: () => void) {
  if (typeof BroadcastChannel === "undefined") return () => {};
  const channel = new BroadcastChannel(SESSION_CHANNEL);
  channel.onmessage = (e: MessageEvent<unknown>) => {
    if (e.data !== clientId) fn();
  };
  return () => channel.close();
}
