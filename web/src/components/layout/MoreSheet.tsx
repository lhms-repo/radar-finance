import { AnimatePresence, motion } from "motion/react";
import { useMe } from "@/app/session";
import { ChevronRight } from "lucide-react";
import { useEffect } from "react";
import { NavLink } from "react-router";
import { MORE_ITEMS, navItem } from "@/app/nav";
import { useCloseOnBack } from "@/lib/back";
import { haptic } from "@/lib/haptics";
import { softSpring } from "@/lib/motion";

/** Bottom sheet with the screens that do not fit in the bar (section 5.5); back closes it first (RNF-26). */
export function MoreSheet({ open, onClose }: { open: boolean; onClose: () => void }) {
  const me = useMe();
  useCloseOnBack(open, onClose);
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  return (
    <AnimatePresence>
      {open && (
        <>
          <motion.div
            className="fixed inset-0 z-40 bg-black/35"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            onClick={onClose}
          />
          <motion.div
            role="dialog"
            aria-modal="true"
            aria-label="Mais telas"
            className="fixed inset-x-0 bottom-0 z-50 rounded-t-3xl bg-card px-4 pb-[calc(env(safe-area-inset-bottom)+1rem)] pt-2 shadow-2xl"
            initial={{ y: "100%" }}
            animate={{ y: 0 }}
            exit={{ y: "100%" }}
            transition={softSpring}
            drag="y"
            dragConstraints={{ top: 0, bottom: 0 }}
            dragElastic={{ top: 0, bottom: 0.6 }}
            onDragEnd={(_, info) => {
              if (info.offset.y > 80 || info.velocity.y > 500) onClose();
            }}
          >
            <div className="mx-auto mb-3 h-1.5 w-10 rounded-full bg-muted" />
            <ul className="flex flex-col">
              {MORE_ITEMS.filter((id) => !(navItem(id).sharedOnly && me?.group.personal)).map((id, i) => {
                const item = navItem(id);
                return (
                  <motion.li
                    key={id}
                    initial={{ opacity: 0, y: 12 }}
                    animate={{ opacity: 1, y: 0 }}
                    transition={{ ...softSpring, delay: 0.03 * i }}
                  >
                    {/* Replacing the step of the sheet, back returns to the screen it was opened on. */}
                    <NavLink
                      to={item.path}
                      replace
                      onClick={() => {
                        haptic();
                        onClose();
                      }}
                      className="flex items-center gap-3 rounded-2xl px-2 py-3 active:bg-accent"
                    >
                      <span className="grid size-10 place-items-center rounded-xl bg-muted">
                        <item.icon className="size-5 text-primary" />
                      </span>
                      <span className="flex-1 text-base">{item.label}</span>
                      <ChevronRight className="size-4 text-muted-foreground" />
                    </NavLink>
                  </motion.li>
                );
              })}
            </ul>
          </motion.div>
        </>
      )}
    </AnimatePresence>
  );
}
