import { AnimatePresence, motion } from "motion/react";
import { Ellipsis, Plus, type LucideIcon } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { NavLink, useLocation } from "react-router";
import { BOTTOM_LEFT, BOTTOM_RIGHT, MORE_ITEMS, activeItem, navItem } from "@/app/nav";
import { AddFan } from "@/components/layout/AddMenu";
import { MoreSheet } from "@/components/layout/MoreSheet";
import { haptic } from "@/lib/haptics";
import { fade, spring } from "@/lib/motion";
import { cn } from "@/lib/utils";

/** How long a press must last to expand the labels. */
const LONG_PRESS_MS = 420;
/** How long the labels stay expanded after the finger leaves. */
const EXPANDED_LINGER_MS = 1400;

/**
 * iPhone bottom bar with five items and Add in the center (section 5.5).
 * Pressing and holding anywhere on the bar expands every label, inspired by
 * the Playful Bottom Nav reference. The active label is always visible.
 */
export function BottomNav() {
  const location = useLocation();
  const active = activeItem(location.pathname);
  const [expanded, setExpanded] = useState(false);
  const [moreOpen, setMoreOpen] = useState(false);
  const [addOpen, setAddOpen] = useState(false);
  const pressTimer = useRef<number | undefined>(undefined);
  const lingerTimer = useRef<number | undefined>(undefined);
  const longPressed = useRef(false);

  useEffect(
    () => () => {
      window.clearTimeout(pressTimer.current);
      window.clearTimeout(lingerTimer.current);
    },
    [],
  );

  /** Starts the long press timer. */
  const onPointerDown = useCallback(() => {
    longPressed.current = false;
    window.clearTimeout(lingerTimer.current);
    pressTimer.current = window.setTimeout(() => {
      longPressed.current = true;
      setExpanded(true);
      haptic();
    }, LONG_PRESS_MS);
  }, []);

  /** Ends the press, collapsing the labels after a short linger. */
  const onPointerUp = useCallback(() => {
    window.clearTimeout(pressTimer.current);
    if (longPressed.current) {
      lingerTimer.current = window.setTimeout(() => setExpanded(false), EXPANDED_LINGER_MS);
    }
  }, []);

  /** Cancels navigation when the touch was a long press, not a tap. */
  const onClickCapture = useCallback((e: React.MouseEvent) => {
    if (longPressed.current) {
      e.preventDefault();
      e.stopPropagation();
      longPressed.current = false;
    }
  }, []);

  const moreActive = active ? MORE_ITEMS.includes(active.id) : false;

  return (
    <>
      <nav
        aria-label="Navegação"
        className="no-callout fixed inset-x-0 bottom-0 z-30 border-t bg-card pb-safe shadow-[0_-4px_16px_rgb(0_0_0/0.06)] md:hidden"
        onPointerDown={onPointerDown}
        onPointerUp={onPointerUp}
        onPointerCancel={onPointerUp}
        onPointerLeave={onPointerUp}
        onClickCapture={onClickCapture}
        onContextMenu={(e) => e.preventDefault()}
      >
        <ul className="mx-auto flex h-16 max-w-md items-stretch justify-between px-2">
          {BOTTOM_LEFT.map((id) => (
            <BarLink key={id} id={id} active={active?.id === id} expanded={expanded} />
          ))}
          <li className="flex flex-1 items-center justify-center">
            <motion.button
              type="button"
              aria-label="Adicionar"
              aria-expanded={addOpen}
              whileTap={{ scale: 0.92 }}
              onClick={() => {
                haptic();
                setAddOpen((o) => !o);
              }}
              className="relative z-50 grid size-13 -translate-y-3 place-items-center rounded-full bg-primary text-primary-foreground shadow-lg shadow-primary/30"
            >
              <motion.span animate={{ rotate: addOpen ? 45 : 0 }} transition={spring} className="grid">
                <Plus className="size-6" strokeWidth={2.5} />
              </motion.span>
            </motion.button>
          </li>
          {BOTTOM_RIGHT.map((id) => (
            <BarLink key={id} id={id} active={active?.id === id} expanded={expanded} />
          ))}
          <li className="flex flex-1">
            <BarButton
              icon={Ellipsis}
              label="Mais"
              active={moreActive}
              expanded={expanded}
              onClick={() => {
                haptic();
                setMoreOpen(true);
              }}
            />
          </li>
        </ul>
      </nav>
      <AddFan open={addOpen} onClose={() => setAddOpen(false)} />
      <MoreSheet open={moreOpen} onClose={() => setMoreOpen(false)} />
    </>
  );
}

/** Bottom bar entry that links to a screen. */
function BarLink({ id, active, expanded }: { id: string; active: boolean; expanded: boolean }) {
  const item = navItem(id);
  return (
    <li className="flex flex-1">
      <NavLink to={item.path} className="flex flex-1" aria-label={item.label} onClick={() => haptic()}>
        <BarContent icon={item.icon} label={item.shortLabel} active={active} expanded={expanded} />
      </NavLink>
    </li>
  );
}

/** Bottom bar entry that runs an action. */
function BarButton(props: { icon: LucideIcon; label: string; active: boolean; expanded: boolean; onClick: () => void }) {
  return (
    <button type="button" onClick={props.onClick} className="flex flex-1" aria-label={props.label}>
      <BarContent icon={props.icon} label={props.label} active={props.active} expanded={props.expanded} />
    </button>
  );
}

/** Icon with the sliding active pill and an animated label. */
function BarContent({
  icon: Icon,
  label,
  active,
  expanded,
}: {
  icon: LucideIcon;
  label: string;
  active: boolean;
  expanded: boolean;
}) {
  const showLabel = active || expanded;
  return (
    <span className="flex flex-1 flex-col items-center justify-center gap-0.5">
      <span className="relative grid h-8 w-12 place-items-center">
        {active && (
          <motion.span layoutId="bottom-active" transition={spring} className="absolute inset-0 rounded-full bg-primary/12" />
        )}
        <Icon className={cn("relative size-[22px] transition-colors", active ? "text-primary" : "text-muted-foreground")} />
      </span>
      <AnimatePresence initial={false}>
        {showLabel && (
          <motion.span
            key="label"
            initial={{ opacity: 0, height: 0, y: -4 }}
            animate={{ opacity: 1, height: "auto", y: 0 }}
            exit={{ opacity: 0, height: 0, y: -4 }}
            transition={fade}
            className={cn("text-[10px] font-medium leading-tight", active ? "text-primary" : "text-muted-foreground")}
          >
            {label}
          </motion.span>
        )}
      </AnimatePresence>
    </span>
  );
}
