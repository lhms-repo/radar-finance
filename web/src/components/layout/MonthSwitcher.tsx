import { AnimatePresence, motion } from "motion/react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { useMonth } from "@/lib/month";
import { monthName } from "@/lib/format";
import { fade } from "@/lib/motion";

/** Month selector with arrows, shared by every screen (section 5.1). */
export function MonthSwitcher() {
  const { year, month, direction, isCurrent, next, previous, goToCurrent } = useMonth();
  const label = `${monthName(year, month)} ${year}`;
  return (
    <div className="flex min-w-0 items-center gap-1">
      <button
        type="button"
        onClick={previous}
        aria-label="Mês anterior"
        className="grid size-9 place-items-center rounded-xl text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
      >
        <ChevronLeft className="size-5" />
      </button>
      <button
        type="button"
        onClick={goToCurrent}
        aria-label={`Mês selecionado: ${label}`}
        data-month={label}
        title={isCurrent ? undefined : "Voltar para o mês atual"}
        className="relative h-9 min-w-0 overflow-hidden px-1 text-left"
      >
        <span className="sr-only" aria-live="polite">
          {label}
        </span>
        <AnimatePresence mode="popLayout" initial={false} custom={direction}>
          <motion.span
            key={`${year}-${month}`}
            custom={direction}
            initial={{ x: direction * 24, opacity: 0 }}
            animate={{ x: 0, opacity: 1 }}
            exit={{ x: direction * -24, opacity: 0 }}
            transition={fade}
            className="flex h-9 items-center whitespace-nowrap text-xl font-semibold tracking-tight md:text-2xl"
            aria-hidden="true"
          >
            {monthName(year, month)}
            <span className="ml-1.5 font-normal text-muted-foreground">{year}</span>
          </motion.span>
        </AnimatePresence>
      </button>
      <button
        type="button"
        onClick={next}
        aria-label="Próximo mês"
        className="grid size-9 place-items-center rounded-xl text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
      >
        <ChevronRight className="size-5" />
      </button>
    </div>
  );
}
