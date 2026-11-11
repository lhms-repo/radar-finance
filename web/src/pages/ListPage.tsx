import { AnimatePresence, motion } from "motion/react";
import { Check, ChevronLeft, Minus, Pencil, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { Dialog, DialogActions } from "@/components/ui/dialog";
import { Field, FieldError } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { NativeSelect } from "@/components/ui/native-select";
import {
  formatSizeMilli,
  parseSizeMilli,
  useAddListItem,
  useDeleteList,
  useProducts,
  useRemoveListItem,
  useRenameList,
  useSetPicked,
  useShoppingList,
  useUpdateListItem,
  type ListItem,
} from "@/features/market/api";
import { fade, spring } from "@/lib/motion";
import { cn } from "@/lib/utils";

/** Writes the quantity of a list item: units for packaged products, kg for products by weight. */
function quantityLabel(it: ListItem) {
  return it.product.byWeight ? `${formatSizeMilli(it.quantityMilli)} kg` : `${it.quantityMilli / 1000} un`;
}

/** A shopping list: add products, adjust quantities and mark what was picked, shared live by the couple (RF-71). */
export function ListPage() {
  const id = Number(useParams().id);
  const list = useShoppingList(id);
  const [renaming, setRenaming] = useState(false);
  const l = list.data;
  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-end justify-between gap-3">
        <div className="flex min-w-0 flex-col gap-0.5">
          <Link to="/market" className="flex items-center gap-0.5 text-sm text-primary">
            <ChevronLeft className="size-4" />
            Mercado
          </Link>
          <h1 className="truncate px-1 text-lg font-semibold">{l?.name ?? "Lista"}</h1>
        </div>
        {l && (
          <Button variant="ghost" size="icon" aria-label="Renomear ou excluir a lista" onClick={() => setRenaming(true)}>
            <Pencil />
          </Button>
        )}
      </div>
      {list.error && <p className="text-sm text-destructive">{list.error.message}</p>}
      {l && !l.done && <AddItem listId={l.id} />}
      {l && (
        <Card className="flex flex-col gap-1">
          <CardTitle>
            {l.pickedCount} de {l.itemCount} {l.itemCount === 1 ? "item pego" : "itens pegos"}
          </CardTitle>
          <ul className="mt-1 flex flex-col">
            <AnimatePresence initial={false}>
              {l.items.map((it) => (
                <ItemRow key={it.id} item={it} readOnly={l.done} />
              ))}
            </AnimatePresence>
          </ul>
          {l.items.length === 0 && <p className="py-6 text-center text-sm text-muted-foreground">Adicione os produtos da compra.</p>}
        </Card>
      )}
      {l && <RenameDialog open={renaming} id={l.id} name={l.name} onClose={() => setRenaming(false)} />}
    </div>
  );
}

/** Product picker and quantity: whole units, or kg with decimals for products by weight. */
function AddItem({ listId }: { listId: number }) {
  const products = useProducts();
  const add = useAddListItem();
  const [productId, setProductId] = useState(0);
  const [qty, setQty] = useState("1");
  const product = (products.data ?? []).find((p) => p.id === productId);
  const quantityMilli = product?.byWeight ? parseSizeMilli(qty) : Number.isInteger(Number(qty)) ? Number(qty) * 1000 : 0;
  return (
    <Card className="flex flex-col gap-2">
      <form
        className="grid grid-cols-[1fr_5.5rem] gap-2 sm:grid-cols-[1fr_7rem_auto] sm:items-end"
        onSubmit={(e) => {
          e.preventDefault();
          add.mutate({ listId, productId, quantityMilli }, { onSuccess: () => setQty("1") });
        }}
      >
        <Field label="Produto" htmlFor="item-product">
          <NativeSelect id="item-product" value={productId} onChange={(e) => setProductId(Number(e.target.value))}>
            <option value={0}>Escolha o produto</option>
            {(products.data ?? []).map((p) => (
              <option key={p.id} value={p.id}>
                {p.label}
              </option>
            ))}
          </NativeSelect>
        </Field>
        <Field label={product?.byWeight ? "Kg" : "Quantidade"} htmlFor="item-qty">
          <Input id="item-qty" inputMode={product?.byWeight ? "decimal" : "numeric"} value={qty} onChange={(e) => setQty(e.target.value)} />
        </Field>
        <Button type="submit" className="col-span-2 sm:col-span-1" disabled={add.isPending || !productId}>
          <Plus />
          Pôr na lista
        </Button>
      </form>
      <FieldError message={add.error?.message} />
      {products.data?.length === 0 && (
        <p className="text-sm text-muted-foreground">
          Cadastre produtos na aba{" "}
          <Link to="/market" className="text-primary">
            Produtos
          </Link>{" "}
          ou leia uma nota fiscal.
        </p>
      )}
    </Card>
  );
}

/** One item: tap to mark as picked, steppers for units and removal. */
function ItemRow({ item: it, readOnly }: { item: ListItem; readOnly: boolean }) {
  const pick = useSetPicked();
  const update = useUpdateListItem();
  const remove = useRemoveListItem();
  return (
    <motion.li
      layout
      initial={{ opacity: 0, height: 0 }}
      animate={{ opacity: 1, height: "auto" }}
      exit={{ opacity: 0, height: 0 }}
      transition={{ layout: spring, height: spring, opacity: fade }}
      className="overflow-hidden border-b last:border-b-0"
    >
      <div className="flex items-center gap-3 py-2.5">
        <button
          type="button"
          role="checkbox"
          aria-checked={it.picked}
          aria-label={`Peguei ${it.product.label}`}
          disabled={readOnly || pick.isPending}
          onClick={() => pick.mutate({ id: it.id, picked: !it.picked })}
          className={cn("grid size-7 shrink-0 place-items-center rounded-full border-2 transition-colors", it.picked ? "border-success bg-success text-white" : "border-muted-foreground/40")}
        >
          <AnimatePresence>
            {it.picked && (
              <motion.span initial={{ scale: 0 }} animate={{ scale: 1 }} exit={{ scale: 0 }} transition={spring}>
                <Check className="size-4" strokeWidth={3} />
              </motion.span>
            )}
          </AnimatePresence>
        </button>
        <span className="min-w-0 flex-1">
          <span className={cn("block truncate text-sm font-medium transition-colors", it.picked && "text-muted-foreground line-through")}>{it.product.label}</span>
          <span className="block text-xs text-muted-foreground">
            {quantityLabel(it)}
            {it.picked && it.pickedByName ? ` · pego por ${it.pickedByName.split(" ")[0]}` : ""}
          </span>
        </span>
        {!readOnly && !it.product.byWeight && (
          <span className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={`Menos ${it.product.label}`}
              disabled={it.quantityMilli <= 1000 || update.isPending}
              onClick={() => update.mutate({ id: it.id, quantityMilli: it.quantityMilli - 1000 })}
            >
              <Minus />
            </Button>
            <Button variant="ghost" size="icon-sm" aria-label={`Mais ${it.product.label}`} disabled={update.isPending} onClick={() => update.mutate({ id: it.id, quantityMilli: it.quantityMilli + 1000 })}>
              <Plus />
            </Button>
          </span>
        )}
        {!readOnly && (
          <Button variant="ghost" size="icon-sm" aria-label={`Tirar ${it.product.label}`} disabled={remove.isPending} onClick={() => remove.mutate(it.id)}>
            <Trash2 />
          </Button>
        )}
      </div>
    </motion.li>
  );
}

/** Renames or deletes the list. */
function RenameDialog({ open, id, name, onClose }: { open: boolean; id: number; name: string; onClose: () => void }) {
  const rename = useRenameList();
  const remove = useDeleteList();
  const navigate = useNavigate();
  const [draft, setDraft] = useState<string | null>(null);
  const value = draft ?? name;

  /** Clears the draft and closes. */
  const close = () => {
    rename.reset();
    remove.reset();
    setDraft(null);
    onClose();
  };

  return (
    <Dialog open={open} onOpenChange={(o) => !o && close()} title="Lista">
      <form
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault();
          rename.mutate({ id, name: value }, { onSuccess: close });
        }}
      >
        <Field label="Nome" htmlFor="list-name">
          <Input id="list-name" autoComplete="off" value={value} onChange={(e) => setDraft(e.target.value)} />
        </Field>
        <FieldError message={rename.error?.message ?? remove.error?.message} />
        <DialogActions className="sm:justify-between">
          <Button variant="ghost" className="text-destructive" disabled={remove.isPending} onClick={() => remove.mutate(id, { onSuccess: () => navigate("/market") })}>
            <Trash2 />
            Excluir lista
          </Button>
          <span className="flex flex-col-reverse gap-2 sm:flex-row">
            <Button variant="ghost" onClick={close}>
              Cancelar
            </Button>
            <Button type="submit" disabled={rename.isPending}>
              Salvar
            </Button>
          </span>
        </DialogActions>
      </form>
    </Dialog>
  );
}
