import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react";
import { currentMonth } from "./format";

type MonthState = {
  year: number;
  month: number;
  /** Direction of the last change: 1 forward, -1 backward, 0 none. */
  direction: 1 | -1 | 0;
  isCurrent: boolean;
  next: () => void;
  previous: () => void;
  goToCurrent: () => void;
  /** Jumps to a given month, sliding in the right direction. */
  goTo: (year: number, month: number) => void;
};

const MonthContext = createContext<MonthState | null>(null);

/** Shifts a year and month by delta months. */
function shift(year: number, month: number, delta: number) {
  const index = year * 12 + (month - 1) + delta;
  return { year: Math.floor(index / 12), month: (index % 12) + 1 };
}

/** Holds the month selected in the top bar, shared by every screen (section 5.1). */
export function MonthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState(() => ({ ...currentMonth(), direction: 0 as 1 | -1 | 0 }));

  const next = useCallback(() => setState((s) => ({ ...shift(s.year, s.month, 1), direction: 1 })), []);
  const previous = useCallback(() => setState((s) => ({ ...shift(s.year, s.month, -1), direction: -1 })), []);
  const goToCurrent = useCallback(
    () =>
      setState((s) => {
        const now = currentMonth();
        const cmp = now.year * 12 + now.month - (s.year * 12 + s.month);
        return { ...now, direction: cmp > 0 ? 1 : cmp < 0 ? -1 : 0 };
      }),
    [],
  );

  const goTo = useCallback(
    (year: number, month: number) =>
      setState((s) => {
        const cmp = year * 12 + month - (s.year * 12 + s.month);
        return { year, month, direction: cmp > 0 ? 1 : cmp < 0 ? -1 : 0 };
      }),
    [],
  );

  const value = useMemo(() => {
    const now = currentMonth();
    return {
      ...state,
      isCurrent: now.year === state.year && now.month === state.month,
      next,
      previous,
      goToCurrent,
      goTo,
    };
  }, [state, next, previous, goToCurrent, goTo]);
  return <MonthContext.Provider value={value}>{children}</MonthContext.Provider>;
}

/** Returns the selected month. */
export function useMonth() {
  const ctx = useContext(MonthContext);
  if (!ctx) throw new Error("useMonth must be used inside MonthProvider");
  return ctx;
}
