import { zodResolver } from "@hookform/resolvers/zod";
import { useQueryClient } from "@tanstack/react-query";
import { AnimatePresence, motion } from "motion/react";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { Navigate, useLocation, useNavigate } from "react-router";
import { z } from "zod";
import { api, ApiRequestError, unwrap } from "@/api/client";
import { ME_KEY, useMeQuery } from "@/app/session";
import { Splash } from "@/components/layout/AppShell";
import { Logo } from "@/components/layout/Logo";
import { Button } from "@/components/ui/button";
import { Input, Label } from "@/components/ui/input";
import { fade } from "@/lib/motion";

const schema = z.object({
  login: z.string().trim().min(1, "Informe o usuário."),
  password: z.string().min(1, "Informe a senha."),
});
type FormValues = z.infer<typeof schema>;

/** Login screen with user and password (RF-01). */
export function LoginPage() {
  const me = useMeQuery();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const location = useLocation();
  const [serverError, setServerError] = useState<string | null>(null);
  const from = (location.state as { from?: string } | null)?.from ?? "/";

  const form = useForm<FormValues>({ resolver: zodResolver(schema), defaultValues: { login: "", password: "" } });

  /** Sends the credentials and enters the app on success. */
  const onSubmit = form.handleSubmit(async (values) => {
    setServerError(null);
    try {
      const user = unwrap(await api.POST("/auth/login", { body: values }));
      qc.setQueryData(ME_KEY, user);
      navigate(from === "/login" ? "/" : from, { replace: true });
    } catch (err) {
      setServerError(err instanceof ApiRequestError ? err.message : "Não foi possível entrar. Tente de novo.");
      form.setValue("password", "");
      form.setFocus("password");
    }
  });

  if (me.isPending) return <Splash />;
  if (me.data) return <Navigate to="/" replace />;

  const { errors, isSubmitting } = form.formState;
  return (
    <div className="grid min-h-dvh place-items-center px-4 pb-safe pt-safe">
      <motion.div
        className="w-full max-w-sm"
        initial={{ opacity: 0, y: 12, filter: "blur(6px)" }}
        animate={{ opacity: 1, y: 0, filter: "blur(0px)" }}
        transition={{ duration: 0.35, ease: "easeOut" }}
      >
        <div className="mb-8 flex flex-col items-center gap-3 text-center">
          <Logo className="size-16 rounded-2xl shadow-lg shadow-primary/20" />
          <div>
            <h1 className="text-2xl font-semibold tracking-tight">Radar</h1>
            <p className="text-sm text-muted-foreground">Finanças do casal</p>
          </div>
        </div>
        <form onSubmit={onSubmit} noValidate className="flex flex-col gap-4 rounded-2xl bg-card p-6 shadow-card">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="login">Usuário</Label>
            <Input
              id="login"
              autoComplete="username"
              autoCapitalize="none"
              autoCorrect="off"
              spellCheck={false}
              aria-invalid={!!errors.login}
              {...form.register("login")}
            />
            {errors.login && <p className="text-xs text-destructive">{errors.login.message}</p>}
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="password">Senha</Label>
            <Input id="password" type="password" autoComplete="current-password" aria-invalid={!!errors.password} {...form.register("password")} />
            {errors.password && <p className="text-xs text-destructive">{errors.password.message}</p>}
          </div>
          <AnimatePresence>
            {serverError && (
              <motion.p
                role="alert"
                initial={{ opacity: 0, height: 0 }}
                animate={{ opacity: 1, height: "auto" }}
                exit={{ opacity: 0, height: 0 }}
                transition={fade}
                className="rounded-xl bg-destructive/10 px-3 py-2 text-sm text-destructive"
              >
                {serverError}
              </motion.p>
            )}
          </AnimatePresence>
          <Button type="submit" size="lg" disabled={isSubmitting} className="mt-1">
            {isSubmitting ? "Entrando..." : "Entrar"}
          </Button>
        </form>
      </motion.div>
    </div>
  );
}
