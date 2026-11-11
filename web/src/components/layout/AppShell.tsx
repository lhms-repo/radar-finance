import { useQueryClient } from "@tanstack/react-query";
import { AnimatePresence, motion } from "motion/react";
import { useEffect, useRef } from "react";
import { Navigate, Outlet, useLocation, useNavigate } from "react-router";
import { onUnauthorized } from "@/api/client";
import { ME_KEY, useMeQuery } from "@/app/session";
import { BottomNav } from "@/components/layout/BottomNav";
import { Logo } from "@/components/layout/Logo";
import { Sidebar } from "@/components/layout/Sidebar";
import { TopBar } from "@/components/layout/TopBar";
import { usePreferences } from "@/lib/preferences";
import { fade } from "@/lib/motion";

/** Authenticated layout: sidebar on desktop, bottom bar on iPhone (section 5). */
export function AppShell() {
  const me = useMeQuery();
  const location = useLocation();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const { theme, setTheme, haptics, setHaptics } = usePreferences();
  const synced = useRef(false);

  useEffect(
    () =>
      onUnauthorized(() => {
        qc.setQueryData(ME_KEY, null);
        navigate("/login", { replace: true, state: { from: location.pathname } });
      }),
    [qc, navigate, location.pathname],
  );

  // Adopts the preferences saved on the server once per session (RNF-14, RNF-25).
  useEffect(() => {
    if (!me.data || synced.current) return;
    synced.current = true;
    if (me.data.theme !== theme) setTheme(me.data.theme);
    if (me.data.haptics !== haptics) setHaptics(me.data.haptics);
  }, [me.data, theme, haptics, setTheme, setHaptics]);

  if (me.isPending) return <Splash />;
  if (!me.data) return <Navigate to="/login" replace state={{ from: location.pathname }} />;

  return (
    <div className="min-h-dvh pl-safe pr-safe md:flex">
      <Sidebar />
      <div className="flex min-w-0 flex-1 flex-col md:px-4">
        <TopBar />
        <main className="flex-1 px-3 pb-[calc(env(safe-area-inset-bottom)+6rem)] pt-3 md:px-0 md:pb-8 md:pt-4">
          <AnimatePresence mode="wait" initial={false}>
            <motion.div
              key={location.pathname}
              initial={{ opacity: 0, y: 6 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -4 }}
              transition={fade}
            >
              <Outlet />
            </motion.div>
          </AnimatePresence>
        </main>
      </div>
      <BottomNav />
    </div>
  );
}

/** Loading screen shown while the session is checked. */
export function Splash() {
  return (
    <div className="grid min-h-dvh place-items-center">
      <motion.div
        initial={{ opacity: 0, scale: 0.9 }}
        animate={{ opacity: 1, scale: 1 }}
        transition={{ duration: 0.3 }}
      >
        <Logo className="size-14 rounded-2xl shadow-lg" />
      </motion.div>
    </div>
  );
}
