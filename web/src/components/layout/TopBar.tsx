import { AnimatePresence, motion } from "motion/react";
import { Bell, Eye, EyeOff, Moon, Sun } from "lucide-react";
import { AddMenu } from "@/components/layout/AddMenu";
import { MonthSwitcher } from "@/components/layout/MonthSwitcher";
import { haptic } from "@/lib/haptics";
import { usePreferences } from "@/lib/preferences";
import { fade } from "@/lib/motion";

/** Top bar with the month selector and the global actions (section 5.1). */
export function TopBar() {
  const { privacy, togglePrivacy, resolvedTheme, setTheme } = usePreferences();
  // Notifications arrive in phase 5 (RF-64); the bell already shows the counter.
  const unread = 0;

  return (
    <header className="sticky top-0 z-20 pt-safe md:static md:pt-3">
      <div className="flex items-center justify-between gap-2 bg-background/80 px-3 py-2 backdrop-blur-xl md:rounded-2xl md:bg-card md:px-4 md:py-3 md:shadow-card md:backdrop-blur-none">
        <MonthSwitcher />
        <div className="flex items-center gap-1 md:gap-2">
          <IconButton
            label={privacy ? "Mostrar valores" : "Ocultar valores"}
            pressed={privacy}
            onClick={() => {
              haptic();
              togglePrivacy();
            }}
          >
            <SwapIcon id={privacy ? "off" : "on"}>{privacy ? <EyeOff className="size-[18px]" /> : <Eye className="size-[18px]" />}</SwapIcon>
          </IconButton>
          <IconButton
            label={resolvedTheme === "dark" ? "Usar tema claro" : "Usar tema escuro"}
            className="md:hidden"
            onClick={(e) => {
              haptic();
              const r = e.currentTarget.getBoundingClientRect();
              setTheme(resolvedTheme === "dark" ? "light" : "dark", { x: r.left + r.width / 2, y: r.top + r.height / 2 });
            }}
          >
            <SwapIcon id={resolvedTheme}>
              {resolvedTheme === "dark" ? <Sun className="size-[18px]" /> : <Moon className="size-[18px]" />}
            </SwapIcon>
          </IconButton>
          <IconButton label={unread > 0 ? `Notificações, ${unread} novas` : "Notificações"}>
            <Bell className="size-[18px]" />
            {unread > 0 && (
              <span className="absolute -right-0.5 -top-0.5 grid min-w-4 place-items-center rounded-full bg-destructive px-1 text-[10px] font-semibold text-white">
                {unread}
              </span>
            )}
          </IconButton>
          <div className="hidden md:block">
            <AddMenu />
          </div>
        </div>
      </div>
    </header>
  );
}

/** Round icon button of the top bar. */
function IconButton({
  label,
  children,
  className,
  pressed,
  onClick,
}: {
  label: string;
  children: React.ReactNode;
  className?: string;
  pressed?: boolean;
  onClick?: (e: React.MouseEvent<HTMLButtonElement>) => void;
}) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      aria-pressed={pressed}
      onClick={onClick}
      className={`relative grid size-10 place-items-center rounded-xl border bg-card text-muted-foreground transition-colors hover:text-foreground ${className ?? ""}`}
    >
      {children}
    </button>
  );
}

/** Crossfades between two icons when id changes. */
function SwapIcon({ id, children }: { id: string; children: React.ReactNode }) {
  return (
    <AnimatePresence mode="popLayout" initial={false}>
      <motion.span
        key={id}
        className="grid"
        initial={{ opacity: 0, scale: 0.6, rotate: -30 }}
        animate={{ opacity: 1, scale: 1, rotate: 0 }}
        exit={{ opacity: 0, scale: 0.6, rotate: 30 }}
        transition={fade}
      >
        {children}
      </motion.span>
    </AnimatePresence>
  );
}
