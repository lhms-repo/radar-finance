import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, Lock } from "lucide-react";
import { AnimatePresence, motion } from "motion/react";
import { useId, useState } from "react";
import { Navigate, useLocation, useNavigate } from "react-router";
import { api, unwrap, setActiveSpace } from "@/api/client";
import { ME_KEY, announceSessionChange, useMeQuery } from "@/app/session";
import { Splash } from "@/components/layout/AppShell";
import { Logo } from "@/components/layout/Logo";
import { Button } from "@/components/ui/button";
import { haptic } from "@/lib/haptics";
import { spring } from "@/lib/motion";
import { cn } from "@/lib/utils";

/** Avatar colors, one per member in the order they joined. */
const AVATAR_COLORS = ["bg-primary", "bg-rose-500", "bg-emerald-600", "bg-amber-500", "bg-violet-500", "bg-cyan-600"];

/** Device storage key of the last group used to enter, so a single tap skips the group step next time. */
const LAST_GROUP_KEY = "radar.lastGroup";

/** Group and person chosen so far; a null group means the person asked to see the list of groups. */
type Pick = { groupId?: number | null; userId?: number };

/** Reads the last group used on this device, if storage is available. */
function readLastGroup(): number | undefined {
  try {
    const v = Number(localStorage.getItem(LAST_GROUP_KEY));
    return v > 0 ? v : undefined;
  } catch {
    return undefined;
  }
}

/** Remembers the group used to enter on this device. */
function writeLastGroup(id: number) {
  try {
    localStorage.setItem(LAST_GROUP_KEY, String(id));
  } catch {
    // Private mode or blocked storage: the group step just shows again.
  }
}

/** Entry screen: choose the group, then the person, then type the PIN (RF-01, RN-27). */
export function LoginPage() {
  const me = useMeQuery();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const location = useLocation();
  const from = (location.state as { from?: string } | null)?.from ?? "/";
  const [pick, setPick] = useState<Pick>({});
  // Each refused PIN remounts the form, which clears the field and focuses it again.
  const [attempt, setAttempt] = useState(0);

  const groups = useQuery({
    queryKey: ["auth", "groups"],
    queryFn: async () => unwrap(await api.GET("/auth/groups")),
    enabled: me.data === null,
  });
  // One group, or the last one used here, is chosen without a tap, unless the person asked for the list.
  const list = groups.data;
  const last = readLastGroup();
  const automatic = list?.length === 1 ? list[0]!.id : list?.some((g) => g.id === last) ? last : undefined;
  const groupId = pick.groupId === undefined ? automatic : (pick.groupId ?? undefined);

  const members = useQuery({
    queryKey: ["auth", "groups", groupId, "members"],
    queryFn: async () => unwrap(await api.GET("/auth/groups/{id}/members", { params: { path: { id: groupId! } } })),
    enabled: me.data === null && groupId !== undefined,
  });

  const login = useMutation({
    mutationFn: async (pin: string) => unwrap(await api.POST("/auth/login", { body: { groupId: groupId!, userId: pick.userId!, pin } })),
    onSuccess: (user) => {
      writeLastGroup(groupId!);
      // A new session starts with no data of the previous one, here and in the other tabs.
      qc.clear();
      setActiveSpace(user.group.id);
      qc.setQueryData(ME_KEY, user);
      announceSessionChange();
      navigate(from === "/login" ? "/" : from, { replace: true });
    },
    onError: () => setAttempt((n) => n + 1),
  });

  if (me.isPending) return <Splash />;
  if (me.data) return <Navigate to="/" replace />;

  const group = list?.find((g) => g.id === groupId);
  const person = members.data?.find((m) => m.userId === pick.userId);
  const personIndex = members.data?.findIndex((m) => m.userId === pick.userId) ?? 0;
  const step = pick.userId !== undefined ? "pin" : groupId !== undefined ? "person" : "group";

  return (
    <div className="grid min-h-dvh place-items-center px-4 pb-safe pt-safe">
      <motion.div
        className="flex w-full max-w-md flex-col items-center"
        initial={{ opacity: 0, y: 12, filter: "blur(6px)" }}
        animate={{ opacity: 1, y: 0, filter: "blur(0px)" }}
        transition={{ duration: 0.35, ease: "easeOut" }}
      >
        <Logo className="mb-4 size-16 rounded-2xl shadow-lg shadow-primary/20" />
        <h1 className="text-2xl font-semibold tracking-tight">{step === "group" ? "Qual grupo?" : step === "person" ? "Quem está usando?" : "Digite seu PIN"}</h1>
        <p className="mb-8 text-sm text-muted-foreground">{group ? group.name : "Radar, finanças em grupo"}</p>

        {groups.isError && <p className="text-sm text-destructive">Não foi possível falar com o servidor.</p>}
        {list?.length === 0 && (
          <p className="text-center text-sm text-muted-foreground">Nenhum grupo criado ainda. Veja o passo 4 de docs/instalacao.md.</p>
        )}

        <AnimatePresence mode="wait" initial={false}>
          {step === "group" && (
            <motion.ul key="group" className="flex w-full flex-col gap-2" initial={{ opacity: 0, x: -12 }} animate={{ opacity: 1, x: 0 }} exit={{ opacity: 0, x: -12 }}>
              {(list ?? []).map((g) => (
                <li key={g.id}>
                  <button
                    type="button"
                    onClick={() => {
                      haptic();
                      setPick({ groupId: g.id });
                    }}
                    className="flex w-full items-center justify-between rounded-2xl bg-card px-5 py-4 text-left text-base font-medium shadow-card outline-none transition-shadow hover:shadow-lg focus-visible:ring-2 focus-visible:ring-ring/50"
                  >
                    {g.name}
                  </button>
                </li>
              ))}
            </motion.ul>
          )}

          {step === "person" && (
            <motion.div key="person" className="flex w-full flex-col gap-4" initial={{ opacity: 0, x: 12 }} animate={{ opacity: 1, x: 0 }} exit={{ opacity: 0, x: -12 }}>
              <ul className="grid w-full grid-cols-2 gap-4">
                {(members.data ?? []).map((m, i) => (
                  <motion.li key={m.userId} initial={{ opacity: 0, scale: 0.9, y: 10 }} animate={{ opacity: 1, scale: 1, y: 0 }} transition={{ ...spring, delay: 0.04 + i * 0.05 }}>
                    <motion.button
                      type="button"
                      whileTap={{ scale: 0.95 }}
                      disabled={!m.hasPin}
                      onClick={() => {
                        haptic();
                        login.reset();
                        setPick({ groupId, userId: m.userId });
                      }}
                      className="no-callout flex w-full flex-col items-center gap-3 rounded-2xl bg-card px-4 py-6 shadow-card outline-none transition-shadow hover:shadow-lg focus-visible:ring-2 focus-visible:ring-ring/50 disabled:opacity-60"
                    >
                      <span className={cn("grid size-20 place-items-center rounded-full text-3xl font-semibold text-white", AVATAR_COLORS[i % AVATAR_COLORS.length])}>
                        {m.name.charAt(0).toUpperCase()}
                      </span>
                      <span className="text-base font-medium">{m.name}</span>
                      {!m.hasPin && <span className="text-xs text-muted-foreground">PIN ainda não definido</span>}
                    </motion.button>
                  </motion.li>
                ))}
              </ul>
              {(list?.length ?? 0) > 1 && (
                <Button variant="ghost" className="self-center" onClick={() => setPick({ groupId: null })}>
                  <ArrowLeft />
                  Trocar de grupo
                </Button>
              )}
            </motion.div>
          )}

          {step === "pin" && person && (
            <motion.div key="pin" className="w-full" initial={{ opacity: 0, x: 12 }} animate={{ opacity: 1, x: 0 }} exit={{ opacity: 0, x: 12 }}>
              <PinForm
                key={attempt}
                name={person.name}
                color={AVATAR_COLORS[personIndex % AVATAR_COLORS.length]!}
                pending={login.isPending}
                error={login.error?.message}
                onSubmit={(pin) => login.mutate(pin)}
                onBack={() => {
                  login.reset();
                  setPick({ groupId });
                }}
              />
            </motion.div>
          )}
        </AnimatePresence>
      </motion.div>
    </div>
  );
}

/** PIN field with the numeric keyboard of the iPhone; a wrong PIN clears it so the next try starts clean. */
function PinForm({
  name,
  color,
  pending,
  error,
  onSubmit,
  onBack,
}: {
  name: string;
  color: string;
  pending: boolean;
  error?: string;
  onSubmit: (pin: string) => void;
  onBack: () => void;
}) {
  const id = useId();
  const [pin, setPin] = useState("");

  return (
    <form
      className="flex w-full flex-col items-center gap-4"
      onSubmit={(e) => {
        e.preventDefault();
        if (pin.length >= 4 && !pending) {
          haptic();
          onSubmit(pin);
        }
      }}
    >
      <span className={cn("grid size-16 place-items-center rounded-full text-2xl font-semibold text-white", color)}>{name.charAt(0).toUpperCase()}</span>
      <span className="text-base font-medium">{name}</span>
      <label htmlFor={id} className="sr-only">
        PIN
      </label>
      <div className="flex w-full max-w-60 items-center gap-2 rounded-2xl border border-input bg-card px-4 focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/20">
        <Lock className="size-4 shrink-0 text-muted-foreground" />
        <input
          id={id}
          type="password"
          inputMode="numeric"
          autoComplete="current-password"
          pattern="[0-9]*"
          maxLength={6}
          autoFocus
          value={pin}
          aria-invalid={!!error}
          onChange={(e) => setPin(e.target.value.replace(/\D/g, "").slice(0, 6))}
          className="h-12 min-w-0 flex-1 bg-transparent text-center text-2xl tracking-[0.5em] outline-none"
        />
      </div>
      {error && (
        <p role="alert" className="text-center text-sm text-destructive">
          {error}
        </p>
      )}
      <Button type="submit" size="lg" className="w-full max-w-60" disabled={pin.length < 4 || pending}>
        {pending ? "Entrando..." : "Entrar"}
      </Button>
      <Button type="button" variant="ghost" onClick={onBack}>
        <ArrowLeft />
        Não sou eu
      </Button>
    </form>
  );
}
