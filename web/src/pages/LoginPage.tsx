import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { motion } from "motion/react";
import { Navigate, useLocation, useNavigate } from "react-router";
import { toast } from "sonner";
import { api, unwrap } from "@/api/client";
import { ME_KEY, useMeQuery } from "@/app/session";
import { Splash } from "@/components/layout/AppShell";
import { Logo } from "@/components/layout/Logo";
import { haptic } from "@/lib/haptics";
import { spring } from "@/lib/motion";
import { cn } from "@/lib/utils";

/** Avatar colors, one per user in creation order. */
const AVATAR_COLORS = ["bg-primary", "bg-rose-500"];

/** "Who is using" screen: the couple picks a name, without password (RF-01). */
export function LoginPage() {
  const me = useMeQuery();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const location = useLocation();
  const from = (location.state as { from?: string } | null)?.from ?? "/";

  const users = useQuery({
    queryKey: ["auth", "users"],
    queryFn: async () => unwrap(await api.GET("/auth/users")),
    enabled: me.data === null,
  });

  const login = useMutation({
    mutationFn: async (loginName: string) => unwrap(await api.POST("/auth/login", { body: { login: loginName } })),
    onSuccess: (user) => {
      qc.setQueryData(ME_KEY, user);
      navigate(from === "/login" ? "/" : from, { replace: true });
    },
    onError: (err) => toast.error("Não foi possível entrar", { description: err.message }),
  });

  if (me.isPending) return <Splash />;
  if (me.data) return <Navigate to="/" replace />;

  return (
    <div className="grid min-h-dvh place-items-center px-4 pb-safe pt-safe">
      <motion.div
        className="flex w-full max-w-md flex-col items-center"
        initial={{ opacity: 0, y: 12, filter: "blur(6px)" }}
        animate={{ opacity: 1, y: 0, filter: "blur(0px)" }}
        transition={{ duration: 0.35, ease: "easeOut" }}
      >
        <Logo className="mb-4 size-16 rounded-2xl shadow-lg shadow-primary/20" />
        <h1 className="text-2xl font-semibold tracking-tight">Quem está usando?</h1>
        <p className="mb-8 text-sm text-muted-foreground">Radar, finanças do casal</p>

        {users.isError && <p className="text-sm text-destructive">Não foi possível falar com o servidor.</p>}
        {users.data?.length === 0 && (
          <p className="text-center text-sm text-muted-foreground">
            Nenhum usuário criado ainda. Veja o passo 4 de docs/instalacao.md.
          </p>
        )}

        <ul className="grid w-full grid-cols-2 gap-4">
          {(users.data ?? []).map((u, i) => {
            const busy = login.isPending && login.variables === u.login;
            return (
              <motion.li
                key={u.login}
                initial={{ opacity: 0, scale: 0.9, y: 10 }}
                animate={{ opacity: 1, scale: 1, y: 0 }}
                transition={{ ...spring, delay: 0.08 + i * 0.06 }}
              >
                <motion.button
                  type="button"
                  whileTap={{ scale: 0.95 }}
                  disabled={login.isPending}
                  onClick={() => {
                    haptic();
                    login.mutate(u.login);
                  }}
                  className="no-callout flex w-full flex-col items-center gap-3 rounded-2xl bg-card px-4 py-6 shadow-card outline-none transition-shadow hover:shadow-lg focus-visible:ring-2 focus-visible:ring-ring/50 disabled:opacity-70"
                >
                  <span
                    className={cn(
                      "grid size-20 place-items-center rounded-full text-3xl font-semibold text-white",
                      AVATAR_COLORS[i % AVATAR_COLORS.length],
                    )}
                  >
                    {busy ? (
                      <motion.span
                        className="size-7 rounded-full border-[3px] border-white/40 border-t-white"
                        animate={{ rotate: 360 }}
                        transition={{ repeat: Infinity, duration: 0.8, ease: "linear" }}
                      />
                    ) : (
                      u.name.charAt(0).toUpperCase()
                    )}
                  </span>
                  <span className="text-base font-medium">{u.name}</span>
                </motion.button>
              </motion.li>
            );
          })}
        </ul>
      </motion.div>
    </div>
  );
}
