import { motion } from "motion/react";
import { Check, ChevronRight, ListChecks, Plus } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { FieldError } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useCreateList, useShoppingLists } from "@/features/market/api";
import { spring } from "@/lib/motion";
import { cn } from "@/lib/utils";

/** Shopping lists shared by the couple, open ones first, with progress (RF-71). */
export function ListsTab() {
  const lists = useShoppingLists();
  const create = useCreateList();
  const [name, setName] = useState("");
  return (
    <div className="flex flex-col gap-3">
      <form
        className="flex items-center gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          create.mutate(name, { onSuccess: () => setName("") });
        }}
      >
        <Input aria-label="Nome da nova lista" placeholder="Nova lista, como Compra do mês" value={name} onChange={(e) => setName(e.target.value)} />
        <Button type="submit" disabled={create.isPending || !name.trim()}>
          <Plus />
          Criar
        </Button>
      </form>
      <FieldError message={create.error?.message} />
      <Card className="p-2">
        <ul className="flex flex-col">
          {(lists.data ?? []).map((l) => {
            const share = l.itemCount ? l.pickedCount / l.itemCount : 0;
            return (
              <li key={l.id} className="border-b last:border-b-0">
                <Link to={`/market/lists/${l.id}`} className={cn("flex items-center gap-3 rounded-xl px-2 py-3 hover:bg-accent", l.done && "opacity-60")}>
                  <span className="grid size-9 shrink-0 place-items-center rounded-xl bg-primary/10 text-primary">{l.done ? <Check className="size-4" /> : <ListChecks className="size-4" />}</span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-medium">{l.name}</span>
                    <span className="block text-xs text-muted-foreground">
                      {l.done ? "Finalizada · " : ""}
                      {l.pickedCount} de {l.itemCount} {l.itemCount === 1 ? "item pego" : "itens pegos"}
                    </span>
                    {!l.done && l.itemCount > 0 && (
                      <span className="mt-1.5 block h-1 overflow-hidden rounded-full bg-muted" aria-hidden>
                        <motion.span className="block h-full rounded-full bg-primary" initial={false} animate={{ width: `${share * 100}%` }} transition={spring} />
                      </span>
                    )}
                  </span>
                  <ChevronRight className="size-4 text-muted-foreground" />
                </Link>
              </li>
            );
          })}
          {lists.data?.length === 0 && <li className="px-2 py-6 text-center text-sm text-muted-foreground">Crie uma lista e montem juntos a compra.</li>}
        </ul>
      </Card>
    </div>
  );
}
