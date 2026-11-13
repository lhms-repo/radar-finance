import { AnimatePresence, motion } from "motion/react";
import { ArrowDownLeft, ChevronDown, CreditCard, FileUp, Plus, ShoppingBag, Utensils, type LucideIcon } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { useQuickEntry, type QuickEntryPreset } from "@/features/quick-entry/QuickEntry";
import { useCloseOnBack } from "@/lib/back";
import { haptic } from "@/lib/haptics";
import { spring } from "@/lib/motion";
import { cn } from "@/lib/utils";

type Shortcut = { id: string; label: string; icon: LucideIcon; preset?: QuickEntryPreset; path?: string; phase?: number };

/** Shortcuts of the Add button (section 5.1); the ones without a preset arrive in later phases. */
export const SHORTCUTS: Shortcut[] = [
  { id: "card", label: "Compra no cartão", icon: CreditCard, preset: { nature: "expense", method: "credit" } },
  { id: "expense", label: "Despesa", icon: ShoppingBag, preset: { nature: "expense", method: "cash" } },
  { id: "income", label: "Receita", icon: ArrowDownLeft, preset: { nature: "income" } },
  { id: "alelo", label: "Compra no Alelo", icon: Utensils, preset: { nature: "expense", method: "alelo" } },
  { id: "csv", label: "Importar CSV", icon: FileUp, path: "/settings/import" },
];

/** Returns the action of a shortcut: the quick entry card, or a notice for later phases. */
function useOpenShortcut() {
  const { open } = useQuickEntry();
  const navigate = useNavigate();
  return (s: Shortcut) => {
    haptic();
    if (s.preset) open(s.preset);
    else if (s.path) navigate(s.path);
    else toast(s.label, { description: `Chega na fase ${s.phase}.` });
  };
}

/** Closes a popover when clicking outside or pressing Escape. */
function useDismiss(open: boolean, close: () => void, ref: React.RefObject<HTMLElement | null>) {
  useEffect(() => {
    if (!open) return;
    const onDown = (e: PointerEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) close();
    };
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && close();
    document.addEventListener("pointerdown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("pointerdown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [open, close, ref]);
}

/** Desktop split button: Add opens the card purchase, the arrow fans out the shortcuts (sections 5.6, 8.6). */
export function AddMenu() {
  const openShortcut = useOpenShortcut();
  const { open: openQuickEntry } = useQuickEntry();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useDismiss(open, () => setOpen(false), ref);
  useCloseOnBack(open, () => setOpen(false));
  return (
    <div ref={ref} className="relative flex">
      <button
        type="button"
        onClick={() => {
          haptic();
          setOpen(false);
          openQuickEntry({ nature: "expense", method: "credit" });
        }}
        className="flex h-10 items-center gap-1.5 rounded-l-xl bg-primary pl-4 pr-3 text-sm font-medium text-primary-foreground shadow-sm transition-[background-color,transform] hover:bg-primary/90 active:scale-[0.97]"
      >
        <Plus className="size-4" />
        Adicionar
      </button>
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        aria-haspopup="menu"
        aria-label="Atalhos"
        className="grid h-10 w-9 place-items-center rounded-r-xl border-l border-white/25 bg-primary text-primary-foreground shadow-sm transition-colors hover:bg-primary/90"
      >
        <motion.span animate={{ rotate: open ? 180 : 0 }} transition={spring} className="grid">
          <ChevronDown className="size-4" />
        </motion.span>
      </button>
      <AnimatePresence>
        {open && (
          <motion.ul
            role="menu"
            className="absolute right-0 top-12 z-50 w-56 origin-top-right rounded-2xl border bg-card p-1.5 shadow-xl"
            initial={{ opacity: 0, scale: 0.9, y: -6 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.95, y: -4, transition: { duration: 0.12 } }}
            transition={spring}
          >
            {SHORTCUTS.map((s, i) => (
              <motion.li
                key={s.id}
                initial={{ opacity: 0, y: -8, rotate: -4 }}
                animate={{ opacity: 1, y: 0, rotate: 0 }}
                transition={{ ...spring, delay: i * 0.035 }}
              >
                <button
                  type="button"
                  role="menuitem"
                  onClick={() => {
                    setOpen(false);
                    openShortcut(s);
                  }}
                  className="flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-sm transition-colors hover:bg-accent"
                >
                  <s.icon className="size-4 text-muted-foreground" />
                  {s.label}
                </button>
              </motion.li>
            ))}
          </motion.ul>
        )}
      </AnimatePresence>
    </div>
  );
}

/** Shortcuts fanned out above the Add button of the phone; back closes them first (RNF-26). */
export function AddFan({ open, onClose }: { open: boolean; onClose: () => void }) {
  const openShortcut = useOpenShortcut();
  useCloseOnBack(open, onClose);
  return (
    <AnimatePresence>
      {open && (
        <>
          <motion.div
            className="fixed inset-0 z-40 bg-black/60 backdrop-blur-sm"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            onClick={onClose}
          />
          <div className="pointer-events-none fixed inset-x-0 bottom-0 z-50 flex justify-center pb-[calc(env(safe-area-inset-bottom)+5.5rem)]">
            <div className="relative h-0 w-0">
              {SHORTCUTS.map((s, i) => {
                // Spread the five shortcuts on an arc from 160 to 20 degrees.
                const angle = ((160 - (i * 140) / (SHORTCUTS.length - 1)) * Math.PI) / 180;
                const radius = 140;
                const x = Math.cos(angle) * radius;
                const y = -Math.sin(angle) * radius;
                return (
                  <motion.button
                    key={s.id}
                    type="button"
                    className={cn(
                      "no-callout pointer-events-auto absolute flex w-[4.5rem] -translate-x-1/2 -translate-y-1/2 flex-col items-center gap-1",
                    )}
                    initial={{ x: 0, y: 0, scale: 0.3, opacity: 0 }}
                    animate={{ x, y, scale: 1, opacity: 1 }}
                    exit={{ x: 0, y: 0, scale: 0.3, opacity: 0, transition: { duration: 0.15 } }}
                    transition={{ ...spring, delay: i * 0.03 }}
                    onClick={() => {
                      onClose();
                      openShortcut(s);
                    }}
                  >
                    <span className="grid size-12 place-items-center rounded-full bg-card text-foreground shadow-lg">
                      <s.icon className="size-5" />
                    </span>
                    <span className="text-center text-[11px] font-medium leading-tight text-white drop-shadow">
                      {s.label}
                    </span>
                  </motion.button>
                );
              })}
            </div>
          </div>
        </>
      )}
    </AnimatePresence>
  );
}
