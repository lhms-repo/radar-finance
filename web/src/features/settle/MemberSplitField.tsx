import { useId, useState } from "react";
import { useMe } from "@/app/session";
import { Field } from "@/components/ui/field";
import { MoneyInput } from "@/components/ui/money-input";
import { Money } from "@/components/ui/money";
import { Segmented } from "@/components/ui/segmented";
import { useGroup } from "@/features/groups/api";
import { useMemberSplit, type MemberSplit, type MemberSplitKind } from "@/features/settle/api";

type Mode = MemberSplit["mode"];

/** Options of the division between members (RN-28). */
const MODES: { value: Mode; label: string }[] = [
  { value: "weights", label: "Pelos pesos" },
  { value: "equal", label: "Iguais" },
  { value: "custom", label: "Valores" },
];

/** Chooses how the members of the group divide an expense: by the weights, equally between who took part, or by value (RN-28). */
export function MemberSplitField({
  value,
  onChange,
  space,
}: {
  value: MemberSplit | undefined;
  onChange: (split: MemberSplit | undefined) => void;
  /** Space chosen in the form; the field hides when the expense goes to another space. */
  space?: number;
}) {
  const me = useMe();
  const group = useGroup();
  const id = useId();
  const members = group.data?.members ?? [];
  // The personal space, another space chosen in the form and a group of one have nobody to divide with here.
  if (!me || me.group.personal || members.length < 2 || (space !== undefined && space !== me.group.id)) return null;
  const mode: Mode = value?.mode ?? "weights";
  const chosen = new Map((value?.members ?? []).map((m) => [m.personId, m.shareCents ?? 0]));
  const sum = [...chosen.values()].reduce((a, b) => a + b, 0);

  /** Switches the mode; equal starts with everyone, values start empty. */
  const setMode = (next: Mode) => {
    if (next === "weights") onChange(undefined);
    else if (next === "equal") onChange({ mode: "equal", members: members.map((m) => ({ personId: m.personId })) });
    else onChange({ mode: "custom", members: members.map((m) => ({ personId: m.personId, shareCents: 0 })) });
  };

  /** Turns a member in or out of the equal division. */
  const toggle = (personId: number) => {
    const next = chosen.has(personId) ? [...chosen.keys()].filter((p) => p !== personId) : [...chosen.keys(), personId];
    onChange({ mode: "equal", members: next.map((p) => ({ personId: p })) });
  };

  /** Sets the value of one member in the division by value. */
  const setShare = (personId: number, cents: number) => {
    onChange({ mode: "custom", members: members.map((m) => ({ personId: m.personId, shareCents: m.personId === personId ? cents : (chosen.get(m.personId) ?? 0) })) });
  };

  return (
    <Field label="Divisão entre os membros" htmlFor={id}>
      <Segmented ariaLabel="Divisão entre os membros" value={mode} options={MODES} onChange={(v) => setMode(v)} className="self-start" />
      {mode === "weights" && (
        <p id={id} className="text-xs text-muted-foreground">
          {members.map((m) => `${m.name} ${(m.weightBp / 100).toLocaleString("pt-BR")}%`).join(" · ")}
        </p>
      )}
      {mode === "equal" && (
        <div id={id} className="flex flex-wrap gap-2">
          {members.map((m) => (
            <label key={m.personId} className="flex items-center gap-2 rounded-xl border px-3 py-2 text-sm">
              <input type="checkbox" checked={chosen.has(m.personId)} onChange={() => toggle(m.personId)} className="size-4 accent-primary" />
              {m.name}
            </label>
          ))}
        </div>
      )}
      {mode === "custom" && (
        <div id={id} className="flex flex-col gap-2">
          {members.map((m) => (
            <div key={m.personId} className="grid grid-cols-[1fr_9rem] items-center gap-2">
              <label htmlFor={`${id}-${m.personId}`} className="truncate text-sm">
                {m.name}
              </label>
              <MoneyInput id={`${id}-${m.personId}`} value={chosen.get(m.personId) ?? 0} onChange={(c) => setShare(m.personId, c)} />
            </div>
          ))}
          <p className="text-xs text-muted-foreground">
            Soma dos valores: <Money cents={sum} />. Ela precisa ser a parte do grupo no gasto.
          </p>
        </div>
      )}
    </Field>
  );
}

/** Keeps the division chosen in a form; when editing, it starts from the one saved for the item (RN-28). */
export function useSplitState(kind: MemberSplitKind, id?: number) {
  const me = useMe();
  const saved = useMemberSplit(kind, id);
  // null is untouched: the form sends nothing and the server keeps what was saved.
  const [chosen, setChosen] = useState<MemberSplit | undefined | null>(null);
  const fromServer = saved.data && saved.data.mode !== "weights" ? saved.data : undefined;
  return {
    value: chosen === null ? fromServer : chosen,
    onChange: (split: MemberSplit | undefined) => setChosen(split),
    reset: () => setChosen(null),
    /** What to send for an expense going to space: the choice made here, or nothing to keep the saved one. */
    payload: (space?: number): MemberSplit | undefined => {
      if (chosen === null) return undefined;
      if (space !== undefined && space !== me?.group.id) return undefined;
      return chosen ?? { mode: "weights" };
    },
  };
}
