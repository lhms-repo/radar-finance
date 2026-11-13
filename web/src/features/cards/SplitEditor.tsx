import { useQuery } from "@tanstack/react-query";
import { AnimatePresence, motion } from "motion/react";
import { Minus, Plus, Users, X } from "lucide-react";
import { useId } from "react";
import { api, unwrap } from "@/api/client";
import { useSpaceWords } from "@/app/space";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { MoneyInput } from "@/components/ui/money-input";
import { Segmented } from "@/components/ui/segmented";
import { Switch } from "@/components/ui/switch";
import { haptic } from "@/lib/haptics";
import { fade, spring } from "@/lib/motion";

/** Split as edited in the form; the server works out the shares (RF-24, RF-25). */
export type SplitValue = {
  enabled: boolean;
  mode: "equal" | "custom";
  coupleParts: number;
  parties: { name: string; shareCents: number }[];
};

/** Split turned off, with one empty person ready when it is turned on. */
export const EMPTY_SPLIT: SplitValue = { enabled: false, mode: "equal", coupleParts: 1, parties: [{ name: "", shareCents: 0 }] };

/** Converts the edited split to the API body, or undefined when turned off. */
export function splitBody(v: SplitValue) {
  if (!v.enabled) return undefined;
  return {
    mode: v.mode,
    coupleParts: v.mode === "equal" ? v.coupleParts : undefined,
    parties: v.parties.map((p) => ({ name: p.name.trim(), shareCents: v.mode === "custom" ? p.shareCents : undefined })),
  };
}

/** Loads the names already used in splits, for suggestions (RF-27). */
function useParties() {
  return useQuery({ queryKey: ["parties"], queryFn: async () => unwrap(await api.GET("/parties")) });
}

/** Turns the split on, picks equal parts or values and lists the people, suggesting known names (RF-24 to RF-27). */
export function SplitEditor({
  value,
  onChange,
  installmentsNote = "As cotas se repartem pelas parcelas.",
}: {
  value: SplitValue;
  onChange: (v: SplitValue) => void;
  installmentsNote?: string;
}) {
  const parties = useParties();
  const listId = useId();
  const words = useSpaceWords();
  const parts = words.personal ? "suas partes" : `partes de ${words.name}`;

  /** Replaces one person of the list. */
  const setParty = (i: number, p: Partial<SplitValue["parties"][number]>) =>
    onChange({ ...value, parties: value.parties.map((x, j) => (j === i ? { ...x, ...p } : x)) });

  return (
    <div className="flex flex-col gap-3 rounded-2xl border p-3">
      <label className="flex items-center justify-between gap-3 text-sm font-medium">
        <span className="flex items-center gap-2">
          <Users className="size-4 text-primary" />
          Dividir com outras pessoas
        </span>
        <Switch checked={value.enabled} onCheckedChange={(enabled) => onChange({ ...value, enabled })} />
      </label>
      <AnimatePresence initial={false}>
        {value.enabled && (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ height: spring, opacity: fade }}
            className="flex flex-col gap-3 overflow-hidden"
          >
            <Segmented<SplitValue["mode"]>
              ariaLabel="Como dividir"
              value={value.mode}
              options={[
                { value: "equal", label: "Partes iguais" },
                { value: "custom", label: "Valores" },
              ]}
              onChange={(mode) => onChange({ ...value, mode })}
              className="w-full"
            />
            {value.mode === "equal" && (
              <div className="flex items-center justify-between gap-3 text-sm">
                <span id={`${listId}-parts`}>{words.personal ? "Suas partes" : `Partes de ${words.name}`}</span>
                <div className="flex items-center gap-2" role="group" aria-labelledby={`${listId}-parts`}>
                  <Button variant="outline" size="icon-sm" aria-label={`Menos ${parts}`} disabled={value.coupleParts <= 0} onClick={() => onChange({ ...value, coupleParts: value.coupleParts - 1 })}>
                    <Minus />
                  </Button>
                  <span className="w-6 text-center font-semibold tabular-nums">{value.coupleParts}</span>
                  <Button variant="outline" size="icon-sm" aria-label={`Mais ${parts}`} disabled={value.coupleParts >= 9} onClick={() => onChange({ ...value, coupleParts: value.coupleParts + 1 })}>
                    <Plus />
                  </Button>
                </div>
              </div>
            )}
            <datalist id={listId}>
              {(parties.data ?? []).map((p) => (
                <option key={p.id} value={p.name} />
              ))}
            </datalist>
            <ul className="flex flex-col gap-2">
              {value.parties.map((p, i) => (
                <li key={i} className="flex items-center gap-2">
                  <Input
                    aria-label={`Pessoa ${i + 1}`}
                    placeholder="Nome"
                    list={listId}
                    autoComplete="off"
                    value={p.name}
                    onChange={(e) => setParty(i, { name: e.target.value })}
                    className="min-w-0 flex-1"
                  />
                  {value.mode === "custom" && (
                    <div className="w-36 shrink-0">
                      <MoneyInput ariaLabel={`Cota da pessoa ${i + 1}`} value={p.shareCents} onChange={(shareCents) => setParty(i, { shareCents })} />
                    </div>
                  )}
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={`Tirar a pessoa ${i + 1}`}
                    disabled={value.parties.length <= 1}
                    onClick={() => onChange({ ...value, parties: value.parties.filter((_, j) => j !== i) })}
                  >
                    <X />
                  </Button>
                </li>
              ))}
            </ul>
            <Button
              variant="ghost"
              size="sm"
              className="self-start text-primary"
              disabled={value.parties.length >= 10}
              onClick={() => {
                haptic();
                onChange({ ...value, parties: [...value.parties, { name: "", shareCents: 0 }] });
              }}
            >
              <Plus />
              Adicionar pessoa
            </Button>
            <p className="text-xs text-muted-foreground">
              {value.mode === "equal"
                ? `Cada pessoa paga uma parte; ${words.personal ? "você fica com as suas partes" : `${words.name} fica com as próprias partes`} e com a sobra de centavos.`
                : `${words.personal ? "Você fica" : `${words.name} fica`} com o que sobrar do total.`} {installmentsNote}
            </p>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}
