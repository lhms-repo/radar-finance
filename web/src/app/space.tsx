import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ArrowRightLeft } from "lucide-react";
import { useId, useState } from "react";
import { toast } from "sonner";
import { api, unwrap } from "@/api/client";
import { useMe } from "@/app/session";
import { Button } from "@/components/ui/button";
import { Field, FieldError } from "@/components/ui/field";
import { NativeSelect } from "@/components/ui/native-select";
import { haptic } from "@/lib/haptics";

/** What can be personal or of a group (RF-83). */
export type SpaceKind = "entry" | "purchase" | "bill" | "financing" | "income" | "savings";

/** Optional space a new item goes to, instead of the active one (RF-83). */
export type SpaceTarget = { space?: number };

/** One space of the user: her personal one or a group. */
export type Space = { id: number; name: string; personal: boolean };

/** Header that creates the item in another space; nothing for the active one. */
export function spaceHeader(space?: number): { "X-Radar-Space"?: number } {
  return space ? { "X-Radar-Space": space } : {};
}

/** Lists the spaces of the user, her personal one first, and the active one. */
export function useSpaces(): { spaces: Space[]; active: number } {
  const me = useMe();
  if (!me) return { spaces: [], active: 0 };
  return { spaces: me.groups.map((g) => ({ id: g.id, name: g.name, personal: g.personal })), active: me.group.id };
}

/** Keeps the space a new item goes to, starting at the active one. */
export function useSpaceChoice() {
  const { active } = useSpaces();
  const [chosen, setChosen] = useState<number | undefined>(undefined);
  const space = chosen ?? active;
  return { space, setSpace: setChosen, target: space || undefined };
}

/** Names a space for the person: Pessoal for the personal one, the group name otherwise. */
function spaceLabel(s: Space) {
  return s.personal ? "Pessoal (só você vê)" : s.name;
}

/** Words for the active space in screen texts: the group name, or the person in the personal space. */
export function useSpaceWords() {
  const me = useMe();
  const personal = me?.group.personal ?? false;
  const name = me?.group.name ?? "";
  return {
    personal,
    name,
    /** Share of a total that stays with the space, as in "parte de Casa" or "sua parte". */
    part: personal ? "sua parte" : `parte de ${name}`,
    /** Share of an invoice without third parties: the full invoice of the personal space mixes every group. */
    invoicePart: personal ? "sem terceiros" : `parte de ${name}`,
  };
}

/** Upper cases the first letter of a text. */
export function capitalize(s: string) {
  return s.charAt(0).toUpperCase() + s.slice(1);
}

/** Chooses where a new item goes: the personal space or a group of the person (RF-83). */
export function SpaceField({ value, onChange }: { value: number; onChange: (space: number) => void }) {
  const { spaces } = useSpaces();
  const id = useId();
  if (spaces.length < 2) return null;
  return (
    <Field label="Onde" htmlFor={id}>
      <NativeSelect id={id} value={value} onChange={(e) => onChange(Number(e.target.value))}>
        {spaces.map((s) => (
          <option key={s.id} value={s.id}>
            {spaceLabel(s)}
          </option>
        ))}
      </NativeSelect>
    </Field>
  );
}

/** Moves an item, with its entries, to another space of the user (RF-83). */
export function useMoveToSpace() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ kind, itemId, space }: { kind: SpaceKind; itemId: number; space: number }) => {
      const res = await api.POST("/spaces/{id}/moves", { params: { path: { id: space } }, body: { kind, itemId } });
      if (!res.response.ok) unwrap({ ...res, data: undefined });
    },
    onSuccess: () => {
      haptic();
      toast.success("Item movido");
      void qc.invalidateQueries();
    },
  });
}

/** Offers to move an item being edited to another space of the user; the item leaves this screen after it (RF-83). */
export function MoveToSpace({ kind, itemId, onMoved }: { kind: SpaceKind; itemId: number; onMoved: () => void }) {
  const { spaces, active } = useSpaces();
  const move = useMoveToSpace();
  const id = useId();
  const others = spaces.filter((s) => s.id !== active);
  const [target, setTarget] = useState<number | undefined>(undefined);
  if (others.length === 0) return null;
  const chosen = target ?? others[0]?.id ?? 0;
  return (
    <div className="flex flex-col gap-1.5 rounded-xl border border-dashed p-3">
      <label htmlFor={id} className="text-sm font-medium">
        Mover para outro espaço
      </label>
      <div className="flex gap-2">
        <div className="min-w-0 flex-1">
          <NativeSelect id={id} value={chosen} onChange={(e) => setTarget(Number(e.target.value))}>
            {others.map((s) => (
              <option key={s.id} value={s.id}>
                {spaceLabel(s)}
              </option>
            ))}
          </NativeSelect>
        </div>
        <Button type="button" variant="outline" disabled={move.isPending} onClick={() => move.mutate({ kind, itemId, space: chosen }, { onSuccess: onMoved })}>
          <ArrowRightLeft />
          Mover
        </Button>
      </div>
      <FieldError message={move.error?.message} />
    </div>
  );
}
