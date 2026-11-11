import { zodResolver } from "@hookform/resolvers/zod";
import { AnimatePresence, motion } from "motion/react";
import { DropdownMenu } from "radix-ui";
import { ChevronDown, ChevronLeft, Eye, EyeOff, FolderInput, MoreHorizontal, Pencil, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { useForm, useWatch } from "react-hook-form";
import { Link } from "react-router";
import { toast } from "sonner";
import { z } from "zod";
import { ApiRequestError } from "@/api/client";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Dialog, DialogActions } from "@/components/ui/dialog";
import { Input, Label } from "@/components/ui/input";
import {
  useCategories,
  useCreateCategory,
  useDeleteCategory,
  useMoveCategory,
  useUpdateCategory,
  type Category,
} from "@/features/categories/api";
import { CATEGORY_ICONS, CategoryIcon } from "@/features/categories/icons";
import { haptic } from "@/lib/haptics";
import { fade, spring } from "@/lib/motion";
import { cn } from "@/lib/utils";

/** Which dialog is open and for which category. */
type DialogState =
  | { kind: "create"; parent?: Category }
  | { kind: "edit"; category: Category }
  | { kind: "move"; category: Category }
  | { kind: "delete"; category: Category };

/** Category management screen in Settings (RF-50, RN-18). */
export function CategoriesPage() {
  const categories = useCategories(true);
  const [expanded, setExpanded] = useState<number | null>(null);
  const [dialog, setDialog] = useState<DialogState | null>(null);
  const toggle = useUpdateCategory();
  const tree = categories.data ?? [];

  /** Turns a category on or off, keeping its history (RN-18). */
  const setActive = (c: Category, active: boolean) =>
    toggle.mutate({ id: c.id, body: { active } }, { onError: (err) => toast.error(err.message) });

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between gap-3">
        <div className="flex flex-col gap-0.5">
          <Link to="/settings" className="flex items-center gap-0.5 text-sm text-primary">
            <ChevronLeft className="size-4" />
            Ajustes
          </Link>
          <h1 className="px-1 text-lg font-semibold">Categorias</h1>
        </div>
        <Button size="sm" onClick={() => setDialog({ kind: "create" })}>
          <Plus />
          Nova categoria
        </Button>
      </div>
      <p className="px-1 text-sm text-muted-foreground">
        Categorias desativadas somem das opções de lançamento, mas continuam no histórico.
      </p>

      {categories.isPending && <p className="px-1 text-sm text-muted-foreground">Carregando...</p>}
      {categories.isError && <p className="px-1 text-sm text-destructive">Não foi possível carregar as categorias.</p>}

      <Card className="p-2">
        <ul className="flex flex-col">
          {tree.map((c) => (
            <CategoryRow
              key={c.id}
              category={c}
              open={expanded === c.id}
              onToggle={() => {
                haptic();
                setExpanded(expanded === c.id ? null : c.id);
              }}
              onAction={(kind, target) => (kind === "toggle" ? setActive(target, !target.active) : setDialog({ kind, category: target }))}
              onAddChild={() => setDialog({ kind: "create", parent: c })}
            />
          ))}
        </ul>
      </Card>

      <CategoryFormDialog state={dialog?.kind === "create" || dialog?.kind === "edit" ? dialog : null} onClose={() => setDialog(null)} />
      <MoveDialog category={dialog?.kind === "move" ? dialog.category : null} tree={tree} onClose={() => setDialog(null)} />
      <DeleteDialog category={dialog?.kind === "delete" ? dialog.category : null} onClose={() => setDialog(null)} />
    </div>
  );
}

type Action = "edit" | "move" | "toggle" | "delete";

/** Top level category with its expandable subcategory list. */
function CategoryRow({
  category,
  open,
  onToggle,
  onAction,
  onAddChild,
}: {
  category: Category;
  open: boolean;
  onToggle: () => void;
  onAction: (kind: Action, target: Category) => void;
  onAddChild: () => void;
}) {
  const children = category.children ?? [];
  return (
    <li className="border-b last:border-b-0">
      <div className="flex items-center gap-1">
        <button
          type="button"
          onClick={onToggle}
          aria-expanded={open}
          className="flex min-w-0 flex-1 items-center gap-3 rounded-xl px-2 py-2.5 text-left active:bg-accent"
        >
          <span className={cn("grid size-10 shrink-0 place-items-center rounded-xl bg-primary/10", !category.active && "bg-muted")}>
            <CategoryIcon name={category.icon} className={cn("size-5 text-primary", !category.active && "text-muted-foreground")} />
          </span>
          <span className="min-w-0 flex-1">
            <span className={cn("block truncate font-medium", !category.active && "text-muted-foreground")}>{category.name}</span>
            <span className="block text-xs text-muted-foreground">
              {children.length === 1 ? "1 subcategoria" : `${children.length} subcategorias`}
            </span>
          </span>
          {!category.active && <InactiveBadge />}
          <motion.span animate={{ rotate: open ? 180 : 0 }} transition={spring}>
            <ChevronDown className="size-4 text-muted-foreground" />
          </motion.span>
        </button>
        <ActionsMenu category={category} canMove={children.length === 0} onAction={onAction} />
      </div>
      <AnimatePresence initial={false}>
        {open && (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ height: spring, opacity: fade }}
            className="overflow-hidden"
          >
            <ul className="mb-2 ml-7 flex flex-col border-l pl-3">
              {children.map((sub) => (
                <li key={sub.id} className="flex items-center gap-1">
                  <span className={cn("min-w-0 flex-1 truncate py-2 text-sm", !sub.active && "text-muted-foreground")}>{sub.name}</span>
                  {!sub.active && <InactiveBadge />}
                  <ActionsMenu category={sub} canMove onAction={onAction} />
                </li>
              ))}
              <li>
                <Button variant="ghost" size="sm" className="my-1 text-primary" onClick={onAddChild} disabled={!category.active}>
                  <Plus />
                  Adicionar subcategoria
                </Button>
              </li>
            </ul>
          </motion.div>
        )}
      </AnimatePresence>
    </li>
  );
}

/** Small pill that marks an inactive category. */
function InactiveBadge() {
  return <span className="shrink-0 rounded-full bg-muted px-2 py-0.5 text-xs text-muted-foreground">Desativada</span>;
}

/** Menu with the actions of one category. */
function ActionsMenu({
  category,
  canMove,
  onAction,
}: {
  category: Category;
  canMove: boolean;
  onAction: (kind: Action, target: Category) => void;
}) {
  const item = "flex cursor-pointer items-center gap-2 rounded-lg px-3 py-2 text-sm outline-none data-[highlighted]:bg-accent";
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label={`Ações de ${category.name}`}>
          <MoreHorizontal />
        </Button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          align="end"
          sideOffset={4}
          className="z-50 min-w-44 rounded-xl border bg-card p-1 shadow-lg data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95 motion-reduce:animate-none"
        >
          <DropdownMenu.Item className={item} onSelect={() => onAction("edit", category)}>
            <Pencil className="size-4" />
            Editar
          </DropdownMenu.Item>
          {canMove && (
            <DropdownMenu.Item className={item} onSelect={() => onAction("move", category)}>
              <FolderInput className="size-4" />
              Mover
            </DropdownMenu.Item>
          )}
          <DropdownMenu.Item className={item} onSelect={() => onAction("toggle", category)}>
            {category.active ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
            {category.active ? "Desativar" : "Reativar"}
          </DropdownMenu.Item>
          <DropdownMenu.Item className={cn(item, "text-destructive")} onSelect={() => onAction("delete", category)}>
            <Trash2 className="size-4" />
            Excluir
          </DropdownMenu.Item>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

const formSchema = z.object({
  name: z.string().trim().min(1, "Informe o nome.").max(60, "Use no máximo 60 caracteres."),
  icon: z.string(),
});
type FormValues = z.infer<typeof formSchema>;

/** Creates or edits a category; top level ones pick an icon (section 5.6). */
function CategoryFormDialog({
  state,
  onClose,
}: {
  state: Extract<DialogState, { kind: "create" | "edit" }> | null;
  onClose: () => void;
}) {
  const editing = state?.kind === "edit" ? state.category : undefined;
  const parent = state?.kind === "create" ? state.parent : undefined;
  const topLevel = editing ? editing.parentId === undefined : parent === undefined;
  const create = useCreateCategory();
  const update = useUpdateCategory();
  const mutation = editing ? update : create;
  const form = useForm<FormValues>({
    resolver: zodResolver(formSchema),
    values: { name: editing?.name ?? "", icon: editing?.icon ?? "" },
  });
  const icon = useWatch({ control: form.control, name: "icon" });

  /** Sends the form, requiring an icon for top level categories. */
  const submit = form.handleSubmit((v) => {
    if (topLevel && !v.icon) {
      form.setError("icon", { message: "Escolha um ícone." });
      return;
    }
    const done = { onSuccess: onClose };
    if (editing) update.mutate({ id: editing.id, body: { name: v.name, icon: v.icon } }, done);
    else create.mutate({ name: v.name, icon: v.icon || undefined, parentId: parent?.id }, done);
  });

  const title = editing ? "Editar categoria" : parent ? `Nova subcategoria em ${parent.name}` : "Nova categoria";
  return (
    <Dialog
      open={state !== null}
      onOpenChange={(o) => {
        if (!o) {
          mutation.reset();
          onClose();
        }
      }}
      title={title}
    >
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="category-name">Nome</Label>
          <Input id="category-name" autoComplete="off" aria-invalid={!!form.formState.errors.name} {...form.register("name")} />
          <FieldError message={form.formState.errors.name?.message} />
        </div>
        {topLevel && (
          <div className="flex flex-col gap-1.5">
            <Label>Ícone</Label>
            <IconPicker value={icon} onChange={(v) => form.setValue("icon", v, { shouldValidate: true })} />
            <FieldError message={form.formState.errors.icon?.message} />
          </div>
        )}
        <FieldError message={mutation.error?.message} />
        <DialogActions>
          <Button variant="ghost" onClick={onClose}>
            Cancelar
          </Button>
          <Button type="submit" disabled={mutation.isPending}>
            {mutation.isPending ? "Salvando..." : "Salvar"}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}

/** Grid of the icon library with a sliding highlight on the chosen one. */
function IconPicker({ value, onChange }: { value: string; onChange: (name: string) => void }) {
  return (
    <div role="radiogroup" aria-label="Ícone" className="grid max-h-52 grid-cols-7 gap-1 overflow-y-auto rounded-xl bg-muted p-1.5">
      {Object.entries(CATEGORY_ICONS).map(([name, Icon]) => {
        const active = value === name;
        return (
          <button
            key={name}
            type="button"
            role="radio"
            aria-checked={active}
            aria-label={name}
            onClick={() => {
              haptic();
              onChange(name);
            }}
            className="relative grid aspect-square place-items-center rounded-lg"
          >
            {active && <motion.span layoutId="icon-picker" transition={spring} className="absolute inset-0 rounded-lg bg-card shadow-sm" />}
            <Icon className={cn("relative size-5", active ? "text-primary" : "text-muted-foreground")} />
          </button>
        );
      })}
    </div>
  );
}

/** Moves a category to another parent or promotes a subcategory (RF-50). */
function MoveDialog({ category, tree, onClose }: { category: Category | null; tree: Category[]; onClose: () => void }) {
  const move = useMoveCategory();
  const [target, setTarget] = useState<number | "top" | null>(null);
  const [icon, setIcon] = useState("");
  const isSub = category?.parentId !== undefined;
  const options = tree.filter((c) => c.active && c.id !== category?.id && c.id !== category?.parentId);
  const needsIcon = target === "top" && !category?.icon;

  /** Clears the choice and closes. */
  const close = () => {
    setTarget(null);
    setIcon("");
    move.reset();
    onClose();
  };

  /** Sends the move. */
  const confirm = () => {
    if (!category || target === null) return;
    const body = target === "top" ? { icon: icon || undefined } : { parentId: target };
    move.mutate({ id: category.id, body }, { onSuccess: close });
  };

  const choice = (selected: boolean) =>
    cn("flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left text-sm transition-colors", selected ? "bg-primary/10 text-primary" : "hover:bg-accent");

  return (
    <Dialog open={category !== null} onOpenChange={(o) => !o && close()} title={`Mover ${category?.name ?? ""}`} description="O histórico de lançamentos vai junto.">
      <ul className="flex max-h-72 flex-col gap-0.5 overflow-y-auto">
        {isSub && (
          <li>
            <button type="button" className={choice(target === "top")} onClick={() => setTarget("top")}>
              <Plus className="size-4" />
              Tornar categoria principal
            </button>
          </li>
        )}
        {options.map((c) => (
          <li key={c.id}>
            <button type="button" className={choice(target === c.id)} onClick={() => setTarget(c.id)}>
              <CategoryIcon name={c.icon} className="size-4" />
              {c.name}
            </button>
          </li>
        ))}
      </ul>
      {needsIcon && (
        <div className="flex flex-col gap-1.5">
          <Label>Ícone da nova categoria principal</Label>
          <IconPicker value={icon} onChange={setIcon} />
        </div>
      )}
      <FieldError message={move.error?.message} />
      <DialogActions>
        <Button variant="ghost" onClick={close}>
          Cancelar
        </Button>
        <Button onClick={confirm} disabled={target === null || (needsIcon && !icon) || move.isPending}>
          {move.isPending ? "Movendo..." : "Mover"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}

/** Confirms a delete and offers to deactivate when the category is in use (RN-18). */
function DeleteDialog({ category, onClose }: { category: Category | null; onClose: () => void }) {
  const remove = useDeleteCategory();
  const deactivate = useUpdateCategory("Categoria desativada");
  const inUse = remove.error instanceof ApiRequestError && remove.error.code === "category_in_use";

  /** Clears errors and closes. */
  const close = () => {
    remove.reset();
    deactivate.reset();
    onClose();
  };

  return (
    <Dialog open={category !== null} onOpenChange={(o) => !o && close()} title={`Excluir ${category?.name ?? ""}?`}>
      <p className="text-sm text-muted-foreground">
        {remove.error ? remove.error.message : "A categoria some de vez. Isso só é possível enquanto ela não tem lançamentos."}
      </p>
      <DialogActions>
        <Button variant="ghost" onClick={close}>
          Cancelar
        </Button>
        {inUse && category?.active ? (
          <Button onClick={() => category && deactivate.mutate({ id: category.id, body: { active: false } }, { onSuccess: close })} disabled={deactivate.isPending}>
            <EyeOff />
            Desativar
          </Button>
        ) : (
          <Button variant="destructive" onClick={() => category && remove.mutate(category.id, { onSuccess: close })} disabled={remove.isPending || !!remove.error}>
            <Trash2 />
            Excluir
          </Button>
        )}
      </DialogActions>
    </Dialog>
  );
}

/** Red helper text under a field, animated in and out. */
function FieldError({ message }: { message?: string }) {
  return (
    <AnimatePresence initial={false}>
      {message && (
        <motion.p
          initial={{ opacity: 0, y: -4 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0 }}
          transition={fade}
          role="alert"
          className="text-sm text-destructive"
        >
          {message}
        </motion.p>
      )}
    </AnimatePresence>
  );
}
