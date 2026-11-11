import NumberFlow from "@number-flow/react";
import { AnimatePresence, motion } from "motion/react";
import { Popover } from "radix-ui";
import { AlertTriangle, Bell, CircleAlert, Info } from "lucide-react";
import { useState } from "react";
import { useNavigate } from "react-router";
import { useMarkNotificationsRead, useNotifications, type Notification } from "@/features/overview/api";
import { haptic } from "@/lib/haptics";
import { fade, spring } from "@/lib/motion";
import { cn } from "@/lib/utils";

/** Icon and color of each severity; color never stands alone, the icon and the title carry it too. */
const SEVERITY = {
  danger: { icon: CircleAlert, className: "text-destructive bg-destructive/10" },
  warning: { icon: AlertTriangle, className: "text-warning bg-warning/15" },
  info: { icon: Info, className: "text-primary bg-primary/10" },
} as const;

/** Bell with the unread counter; it opens the list of warnings (RF-64, section 5.1). */
export function NotificationsButton() {
  const notes = useNotifications();
  const mark = useMarkNotificationsRead();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const unread = notes.data?.unread ?? 0;
  const items = notes.data?.items ?? [];

  /** Marks one notification read and goes to the screen that solves it. */
  const go = (n: Notification) => {
    haptic();
    if (!n.read) mark.mutate([n.key]);
    setOpen(false);
    navigate(n.link);
  };

  return (
    <Popover.Root open={open} onOpenChange={setOpen}>
      <Popover.Trigger asChild>
        <button
          type="button"
          aria-label={unread > 0 ? `Notificações, ${unread} novas` : "Notificações"}
          title="Notificações"
          className="relative grid size-10 place-items-center rounded-xl border bg-card text-muted-foreground transition-colors hover:text-foreground"
        >
          <motion.span key={unread} animate={unread > 0 ? { rotate: [0, -14, 12, -6, 0] } : {}} transition={{ duration: 0.5 }} className="grid">
            <Bell className="size-[18px]" />
          </motion.span>
          <AnimatePresence>
            {unread > 0 && (
              <motion.span
                initial={{ scale: 0 }}
                animate={{ scale: 1 }}
                exit={{ scale: 0 }}
                transition={spring}
                className="absolute -right-1 -top-1 grid min-w-4 place-items-center rounded-full bg-destructive px-1 text-[10px] font-semibold text-white"
              >
                <NumberFlow value={unread} />
              </motion.span>
            )}
          </AnimatePresence>
        </button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          align="end"
          sideOffset={8}
          collisionPadding={12}
          className="z-50 flex max-h-[70dvh] w-[min(24rem,calc(100vw-1.5rem))] flex-col overflow-hidden rounded-2xl border bg-card shadow-xl data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95 motion-reduce:animate-none"
        >
          <div className="flex items-center justify-between gap-2 border-b px-4 py-3">
            <span className="text-sm font-semibold">Notificações</span>
            {unread > 0 && (
              <button type="button" className="text-xs text-primary" onClick={() => mark.mutate(undefined)}>
                Marcar todas como lidas
              </button>
            )}
          </div>
          <ul className="flex flex-col overflow-y-auto">
            <AnimatePresence initial={false}>
              {items.map((n) => {
                const sev = SEVERITY[n.severity];
                return (
                  <motion.li key={n.key} layout initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={fade}>
                    <button type="button" onClick={() => go(n)} className={cn("flex w-full items-start gap-3 px-4 py-3 text-left hover:bg-accent", n.read && "opacity-60")}>
                      <span className={cn("mt-0.5 grid size-8 shrink-0 place-items-center rounded-lg", sev.className)}>
                        <sev.icon className="size-4" />
                      </span>
                      <span className="min-w-0 flex-1">
                        <span className="block text-sm font-medium">{n.title}</span>
                        <span className="block text-xs text-muted-foreground">{n.message}</span>
                      </span>
                      {!n.read && <span className="mt-2 size-2 shrink-0 rounded-full bg-primary" aria-label="Não lida" />}
                    </button>
                  </motion.li>
                );
              })}
            </AnimatePresence>
            {items.length === 0 && <li className="px-4 py-8 text-center text-sm text-muted-foreground">Tudo em dia por aqui.</li>}
          </ul>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}
