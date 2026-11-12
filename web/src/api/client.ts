import createClient, { type Middleware } from "openapi-fetch";
import type { components, paths } from "./schema";

export type Me = components["schemas"]["Me"];
export type Theme = components["schemas"]["Theme"];
export type Health = components["schemas"]["Health"];
export type Backup = components["schemas"]["Backup"];
export type ApiError = components["schemas"]["Error"];
export type { components };

/** Listeners called when the server says the session is gone. */
const unauthorizedListeners = new Set<() => void>();

/** Registers a callback for expired sessions and returns its remover. */
export function onUnauthorized(fn: () => void) {
  unauthorizedListeners.add(fn);
  return () => {
    unauthorizedListeners.delete(fn);
  };
}

/** Id of this browser tab, sent with each write so the live updates skip only the tab that wrote (RF-71). */
export const clientId = newClientId();

/** Returns a random tab id; randomUUID only exists over HTTPS, and the dev server is also opened by IP over HTTP. */
function newClientId() {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") return crypto.randomUUID();
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
}

const sessionMiddleware: Middleware = {
  /** Tags the request with the id of this tab. */
  onRequest({ request }) {
    request.headers.set("X-Radar-Client", clientId);
    return request;
  },
  /** Notifies listeners when a protected call returns 401. */
  onResponse({ request, response }) {
    if (response.status === 401 && !request.url.endsWith("/auth/login")) {
      unauthorizedListeners.forEach((fn) => fn());
    }
    return response;
  },
};

export const api = createClient<paths>({ baseUrl: "/api", credentials: "same-origin" });
api.use(sessionMiddleware);

/** Thrown by unwrap with the message the server wrote for the user. */
export class ApiRequestError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message);
  }
}

/** Returns the data of an openapi-fetch result or throws ApiRequestError. */
export function unwrap<T>(result: { data?: T; error?: unknown; response: Response }): T {
  if (result.error !== undefined || result.data === undefined) {
    const err = result.error as Partial<ApiError> | undefined;
    throw new ApiRequestError(
      result.response.status,
      err?.code ?? "unknown",
      err?.message ?? "Não foi possível falar com o servidor.",
    );
  }
  return result.data;
}
