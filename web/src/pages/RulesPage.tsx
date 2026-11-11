import { ChevronLeft, Pencil, Plus, Trash2, Wand2 } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import { Button } from "@/components/ui/button";
import { Card, CardDescription } from "@/components/ui/card";
import { Dialog, DialogActions } from "@/components/ui/dialog";
import { Field, FieldError } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { CategorySelect } from "@/features/categories/CategorySelect";
import { useAliasRules, useDeleteRule, useSaveRule, type AliasRule } from "@/features/imports/api";

/** Alias rules: the original merchant of the CSV becomes a friendly name with a default category (RF-54, RN-19). */
export function RulesPage() {
  const rules = useAliasRules();
  const remove = useDeleteRule();
  const [editing, setEditing] = useState<AliasRule | "new" | null>(null);
  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between gap-3">
        <div className="flex flex-col gap-0.5">
          <Link to="/settings" className="flex items-center gap-0.5 text-sm text-primary">
            <ChevronLeft className="size-4" />
            Ajustes
          </Link>
          <h1 className="px-1 text-lg font-semibold">Regras de apelido</h1>
        </div>
        <Button size="sm" onClick={() => setEditing("new")}>
          <Plus />
          Nova regra
        </Button>
      </div>
      <CardDescription className="px-1">
        A regra casa pelo texto original do estabelecimento no CSV, sem diferença de maiúsculas. Com duas regras para o mesmo texto, vale a mais recente.
      </CardDescription>
      <Card className="p-2">
        <ul className="flex flex-col">
          {(rules.data ?? []).map((r) => (
            <li key={r.id} className="flex items-center gap-3 border-b px-2 py-2.5 last:border-b-0">
              <Wand2 className="size-4 shrink-0 text-primary" />
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm font-medium">{r.friendlyName}</span>
                <span className="block truncate text-xs text-muted-foreground">
                  {r.original}
                  {r.categoryName ? ` · ${r.categoryName}` : ""}
                </span>
              </span>
              <Button variant="ghost" size="icon-sm" aria-label={`Editar ${r.friendlyName}`} onClick={() => setEditing(r)}>
                <Pencil />
              </Button>
              <Button variant="ghost" size="icon-sm" aria-label={`Excluir ${r.friendlyName}`} disabled={remove.isPending} onClick={() => remove.mutate(r.id)}>
                <Trash2 />
              </Button>
            </li>
          ))}
          {rules.data?.length === 0 && <li className="px-2 py-6 text-center text-sm text-muted-foreground">As regras aparecem aqui quando você marca "Lembrar" numa importação.</li>}
        </ul>
      </Card>
      <RuleDialog state={editing} onClose={() => setEditing(null)} />
    </div>
  );
}

/** Creates or edits a rule. */
function RuleDialog({ state, onClose }: { state: AliasRule | "new" | null; onClose: () => void }) {
  const save = useSaveRule();
  const rule = state !== null && state !== "new" ? state : undefined;
  const [original, setOriginal] = useState("");
  const [name, setName] = useState("");
  const [categoryId, setCategoryId] = useState(0);
  const [loaded, setLoaded] = useState<AliasRule | "new" | null>(null);
  // Loads the rule into the fields each time the dialog opens for another one.
  if (state !== loaded) {
    setLoaded(state);
    setOriginal(rule?.original ?? "");
    setName(rule?.friendlyName ?? "");
    setCategoryId(rule?.categoryId ?? 0);
  }

  /** Clears the mutation state and closes. */
  const close = () => {
    save.reset();
    onClose();
  };

  return (
    <Dialog open={state !== null} onOpenChange={(o) => !o && close()} title={rule ? "Editar regra" : "Nova regra"}>
      <form
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate({ id: rule?.id, body: { original, friendlyName: name, categoryId: categoryId || undefined } }, { onSuccess: close });
        }}
      >
        <Field label="Texto original no CSV" htmlFor="rule-original">
          <Input id="rule-original" autoComplete="off" placeholder="IFOOD *PIZZARIA" value={original} onChange={(e) => setOriginal(e.target.value)} />
        </Field>
        <Field label="Nome amigável" htmlFor="rule-name">
          <Input id="rule-name" autoComplete="off" value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
        <Field label="Categoria padrão" htmlFor="rule-category">
          <CategorySelect id="rule-category" placeholder="Sem categoria padrão" value={categoryId} onChange={(e) => setCategoryId(Number(e.target.value))} />
        </Field>
        <FieldError message={save.error?.message} />
        <DialogActions>
          <Button variant="ghost" onClick={close}>
            Cancelar
          </Button>
          <Button type="submit" disabled={save.isPending}>
            Salvar
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}
