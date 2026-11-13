import { zodResolver } from "@hookform/resolvers/zod";
import { ChevronLeft, KeyRound, Plus, ShieldCheck, UserPlus, Users } from "lucide-react";
import { useState } from "react";
import { Controller, useForm } from "react-hook-form";
import { Link } from "react-router";
import { z } from "zod";
import { useMe, useSwitchGroup } from "@/app/session";
import { Button } from "@/components/ui/button";
import { Card, CardDescription, CardTitle } from "@/components/ui/card";
import { Dialog, DialogActions } from "@/components/ui/dialog";
import { Field, FieldError } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { Switch } from "@/components/ui/switch";
import {
  useAddMember,
  useCreateGroup,
  useCreatePerson,
  useGroup,
  useRenameGroup,
  useResetPin,
  useSetRole,
  useSetWeights,
  type GroupMember,
} from "@/features/groups/api";

/** A PIN of 4 to 6 numbers (RN-27). */
const pinField = z.string().regex(/^\d{4,6}$/, "O PIN tem de 4 a 6 números.");

/** Shows the people of the active group, or explains the personal space, which has only its owner (RF-80, RF-83). */
export function GroupPage() {
  const me = useMe();
  if (!me?.group.personal) return <GroupMembers />;
  return (
    <div className="flex flex-col gap-4">
      <Link to="/settings" className="flex items-center gap-0.5 text-sm text-primary">
        <ChevronLeft className="size-4" />
        Ajustes
      </Link>
      <Card className="flex flex-col gap-1">
        <CardTitle>Espaço Pessoal</CardTitle>
        <CardDescription>Aqui ficam só os seus lançamentos pessoais, que ninguém mais vê. Troque para um grupo para ver as pessoas dele.</CardDescription>
      </Card>
    </div>
  );
}

/** The active group, its people and, for administrators, their management (RF-79, RF-80). */
function GroupMembers() {
  const me = useMe();
  const group = useGroup();
  const isAdmin = me?.group.role === "admin";
  const [dialog, setDialog] = useState<"rename" | "person" | "existing" | "group" | "weights" | null>(null);
  const [pinFor, setPinFor] = useState<GroupMember | null>(null);
  const setRole = useSetRole();
  const members = group.data?.members ?? [];
  const admins = members.filter((m) => m.role === "admin").length;

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-0.5">
        <Link to="/settings" className="flex items-center gap-0.5 text-sm text-primary">
          <ChevronLeft className="size-4" />
          Ajustes
        </Link>
        <h1 className="px-1 text-lg font-semibold">Grupo e pessoas</h1>
      </div>

      <Card className="flex flex-col gap-3">
        <div className="flex items-start justify-between gap-3">
          <div>
            <CardTitle>{group.data?.name ?? "Grupo"}</CardTitle>
            <CardDescription>{isAdmin ? "Você administra este grupo." : "Só os administradores mudam o grupo e as pessoas."}</CardDescription>
          </div>
          {isAdmin && (
            <span className="flex gap-2">
              <Button variant="outline" size="sm" onClick={() => setDialog("weights")}>
                Pesos
              </Button>
              <Button variant="outline" size="sm" onClick={() => setDialog("rename")}>
                Renomear
              </Button>
            </span>
          )}
        </div>
      </Card>

      <Card className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <CardTitle>Pessoas</CardTitle>
          {isAdmin && (
            <div className="flex flex-wrap gap-2">
              <Button size="sm" onClick={() => setDialog("person")}>
                <UserPlus />
                Cadastrar pessoa
              </Button>
              <Button size="sm" variant="outline" onClick={() => setDialog("existing")}>
                <Plus />
                Já cadastrada
              </Button>
            </div>
          )}
        </div>
        <ul className="flex flex-col">
          {members.map((m) => (
            <li key={m.userId} className="flex flex-wrap items-center gap-3 border-b py-2.5 last:border-b-0">
              <span className="grid size-9 place-items-center rounded-full bg-muted text-sm font-semibold">{m.name.charAt(0).toUpperCase()}</span>
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm font-medium">{m.name}</span>
                <span className="block truncate text-xs text-muted-foreground">
                  @{m.login} · {m.role === "admin" ? "administrador" : "membro"} · peso {(m.weightBp / 100).toLocaleString("pt-BR")}%
                  {!m.hasPin && " · sem PIN"}
                </span>
              </span>
              {isAdmin && (
                <span className="flex gap-1">
                  <Button
                    variant="ghost"
                    size="sm"
                    aria-label={m.role === "admin" ? `Tornar ${m.name} membro` : `Tornar ${m.name} administrador`}
                    disabled={setRole.isPending || (m.role === "admin" && admins === 1)}
                    onClick={() => setRole.mutate({ userId: m.userId, role: m.role === "admin" ? "member" : "admin" })}
                  >
                    <ShieldCheck />
                    {m.role === "admin" ? "Tornar membro" : "Tornar admin"}
                  </Button>
                  <Button variant="ghost" size="sm" aria-label={`Redefinir PIN de ${m.name}`} onClick={() => setPinFor(m)}>
                    <KeyRound />
                    PIN
                  </Button>
                </span>
              )}
            </li>
          ))}
        </ul>
        <FieldError message={setRole.error?.message} />
      </Card>

      {isAdmin && (
        <Card className="flex flex-col gap-3">
          <div>
            <CardTitle>Outro grupo</CardTitle>
            <CardDescription>Para outra casa ou outro grupo de gastos. Você vira o administrador dele.</CardDescription>
          </div>
          <Button variant="outline" className="self-start" onClick={() => setDialog("group")}>
            <Users />
            Criar grupo
          </Button>
        </Card>
      )}

      <WeightsDialog key={`weights-${dialog === "weights"}`} open={dialog === "weights"} members={members} onClose={() => setDialog(null)} />
      <NameDialog
        key={`rename-${dialog === "rename"}`}
        open={dialog === "rename"}
        title="Renomear grupo"
        initial={group.data?.name ?? ""}
        mutation={useRenameGroup()}
        onClose={() => setDialog(null)}
      />
      <CreateGroupDialog key={`group-${dialog === "group"}`} open={dialog === "group"} onClose={() => setDialog(null)} />
      <PersonDialog key={`person-${dialog === "person"}`} open={dialog === "person"} onClose={() => setDialog(null)} />
      <ExistingDialog key={`existing-${dialog === "existing"}`} open={dialog === "existing"} onClose={() => setDialog(null)} />
      <PinDialog key={pinFor?.userId ?? 0} member={pinFor} onClose={() => setPinFor(null)} />
    </div>
  );
}

/** Asks for a group name and runs the rename. */
function NameDialog({
  open,
  title,
  initial,
  mutation,
  onClose,
}: {
  open: boolean;
  title: string;
  initial: string;
  mutation: ReturnType<typeof useRenameGroup>;
  onClose: () => void;
}) {
  const form = useForm<{ name: string }>({
    resolver: zodResolver(z.object({ name: z.string().trim().min(1, "Informe o nome do grupo.").max(40, "Use no máximo 40 caracteres.") })),
    defaultValues: { name: initial },
  });
  /** Clears the mutation and closes. */
  const close = () => {
    mutation.reset();
    onClose();
  };
  return (
    <Dialog open={open} onOpenChange={(o) => !o && close()} title={title}>
      <form onSubmit={form.handleSubmit((v) => mutation.mutate(v.name, { onSuccess: close }))} className="flex flex-col gap-4" noValidate>
        <Field label="Nome" htmlFor="group-name" error={form.formState.errors.name?.message}>
          <Input id="group-name" autoComplete="off" {...form.register("name")} />
        </Field>
        <FieldError message={mutation.error?.message} />
        <DialogActions>
          <Button type="button" variant="ghost" onClick={close}>
            Cancelar
          </Button>
          <Button type="submit" disabled={mutation.isPending}>
            Salvar
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}

/** Creates another group and offers to switch to it right away (RF-78, RF-81). */
function CreateGroupDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const create = useCreateGroup();
  const switchGroup = useSwitchGroup();
  const form = useForm<{ name: string }>({
    resolver: zodResolver(z.object({ name: z.string().trim().min(1, "Informe o nome do grupo.").max(40, "Use no máximo 40 caracteres.") })),
    defaultValues: { name: "" },
  });
  /** Clears the mutation and closes. */
  const close = () => {
    create.reset();
    onClose();
  };
  return (
    <Dialog open={open} onOpenChange={(o) => !o && close()} title="Criar grupo" description="Os dados de cada grupo ficam separados.">
      <form onSubmit={form.handleSubmit((v) => create.mutate(v.name))} className="flex flex-col gap-4" noValidate>
        {create.data ? (
          <p className="text-sm">O grupo {create.data.name} foi criado. Você pode ir para ele agora ou depois, pelo seletor de grupo.</p>
        ) : (
          <Field label="Nome" htmlFor="new-group-name" error={form.formState.errors.name?.message}>
            <Input id="new-group-name" autoComplete="off" {...form.register("name")} />
          </Field>
        )}
        <FieldError message={create.error?.message} />
        <DialogActions>
          <Button type="button" variant="ghost" onClick={close}>
            {create.data ? "Ficar aqui" : "Cancelar"}
          </Button>
          {create.data ? (
            <Button type="button" onClick={() => switchGroup.mutate(create.data.id)}>
              Ir para {create.data.name}
            </Button>
          ) : (
            <Button type="submit" disabled={create.isPending}>
              Criar
            </Button>
          )}
        </DialogActions>
      </form>
    </Dialog>
  );
}

/** Registers a new person with login and PIN in the group (RF-80, RN-27). */
function PersonDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const create = useCreatePerson();
  const schema = z.object({
    name: z.string().trim().min(1, "Informe o nome.").max(60, "Use no máximo 60 caracteres."),
    login: z.string().trim().min(1, "Informe o login.").max(64, "Use no máximo 64 caracteres."),
    pin: pinField,
    admin: z.boolean(),
  });
  const form = useForm<z.infer<typeof schema>>({ resolver: zodResolver(schema), defaultValues: { name: "", login: "", pin: "", admin: false } });
  const errors = form.formState.errors;
  /** Clears the mutation and closes. */
  const close = () => {
    create.reset();
    onClose();
  };
  return (
    <Dialog open={open} onOpenChange={(o) => !o && close()} title="Cadastrar pessoa" description="Ela entra com o login escolhido aqui e o PIN.">
      <form
        onSubmit={form.handleSubmit((v) => create.mutate({ name: v.name, login: v.login, pin: v.pin, role: v.admin ? "admin" : "member" }, { onSuccess: close }))}
        className="flex flex-col gap-4"
        noValidate
      >
        <Field label="Nome" htmlFor="person-name" error={errors.name?.message}>
          <Input id="person-name" autoComplete="off" {...form.register("name")} />
        </Field>
        <Field label="Login" htmlFor="person-login" error={errors.login?.message}>
          <Input id="person-login" autoComplete="off" autoCapitalize="none" {...form.register("login")} />
        </Field>
        <Field label="PIN" htmlFor="person-pin" error={errors.pin?.message}>
          <Input id="person-pin" type="password" inputMode="numeric" autoComplete="new-password" maxLength={6} {...form.register("pin")} />
        </Field>
        <label className="flex items-center justify-between gap-3 text-sm">
          Administrador do grupo
          <Controller control={form.control} name="admin" render={({ field }) => <Switch checked={field.value} onCheckedChange={field.onChange} aria-label="Administrador do grupo" />} />
        </label>
        <FieldError message={create.error?.message} />
        <DialogActions>
          <Button type="button" variant="ghost" onClick={close}>
            Cancelar
          </Button>
          <Button type="submit" disabled={create.isPending}>
            Cadastrar
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}

/** Adds a person already registered, by login, to the group (RF-80). */
function ExistingDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const add = useAddMember();
  const schema = z.object({ login: z.string().trim().min(1, "Informe o login."), pin: pinField, admin: z.boolean() });
  const form = useForm<z.infer<typeof schema>>({ resolver: zodResolver(schema), defaultValues: { login: "", pin: "", admin: false } });
  /** Clears the mutation and closes. */
  const close = () => {
    add.reset();
    onClose();
  };
  return (
    <Dialog
      open={open}
      onOpenChange={(o) => !o && close()}
      title="Adicionar pessoa já cadastrada"
      description="A própria pessoa digita o PIN dela para concordar. Ela entra no grupo com o login e o PIN que já usa."
    >
      <form onSubmit={form.handleSubmit((v) => add.mutate({ login: v.login, pin: v.pin, role: v.admin ? "admin" : "member" }, { onSuccess: close }))} className="flex flex-col gap-4" noValidate>
        <Field label="Login" htmlFor="existing-login" error={form.formState.errors.login?.message}>
          <Input id="existing-login" autoComplete="off" autoCapitalize="none" {...form.register("login")} />
        </Field>
        <Field label="PIN da pessoa" htmlFor="existing-pin" error={form.formState.errors.pin?.message}>
          <Input id="existing-pin" type="password" inputMode="numeric" autoComplete="off" maxLength={6} {...form.register("pin")} />
        </Field>
        <label className="flex items-center justify-between gap-3 text-sm">
          Administrador do grupo
          <Controller control={form.control} name="admin" render={({ field }) => <Switch checked={field.value} onCheckedChange={field.onChange} aria-label="Administrador do grupo" />} />
        </label>
        <FieldError message={add.error?.message} />
        <DialogActions>
          <Button type="button" variant="ghost" onClick={close}>
            Cancelar
          </Button>
          <Button type="submit" disabled={add.isPending}>
            Adicionar
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}

/** Sets a new PIN for a member; it also lifts a lock after too many wrong ones (RF-80, RN-27). */
function PinDialog({ member, onClose }: { member: GroupMember | null; onClose: () => void }) {
  const reset = useResetPin();
  const form = useForm<{ pin: string }>({ resolver: zodResolver(z.object({ pin: pinField })), defaultValues: { pin: "" } });
  /** Clears the mutation and closes. */
  const close = () => {
    reset.reset();
    onClose();
  };
  return (
    <Dialog open={member !== null} onOpenChange={(o) => !o && close()} title={`Novo PIN de ${member?.name ?? ""}`} description="O PIN antigo deixa de valer na hora.">
      <form onSubmit={form.handleSubmit((v) => member && reset.mutate({ userId: member.userId, pin: v.pin }, { onSuccess: close }))} className="flex flex-col gap-4" noValidate>
        <Field label="Novo PIN" htmlFor="reset-pin" error={form.formState.errors.pin?.message}>
          <Input id="reset-pin" type="password" inputMode="numeric" autoComplete="new-password" maxLength={6} {...form.register("pin")} />
        </Field>
        <FieldError message={reset.error?.message} />
        <DialogActions>
          <Button type="button" variant="ghost" onClick={close}>
            Cancelar
          </Button>
          <Button type="submit" disabled={reset.isPending}>
            Salvar PIN
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
}

/** Sets the weight of each member in the settlement, in percent adding up to 100, from today on (RF-79, RN-28). */
function WeightsDialog({ open, members, onClose }: { open: boolean; members: GroupMember[]; onClose: () => void }) {
  const save = useSetWeights();
  const [percent, setPercent] = useState<Record<number, string>>(() => Object.fromEntries(members.map((m) => [m.userId, String(m.weightBp / 100)])));
  /** Reads a percentage typed with comma or dot as basis points. */
  const bp = (v: string | undefined) => Math.round(Number((v ?? "").replace(",", ".")) * 100) || 0;
  const total = members.reduce((sum, m) => sum + bp(percent[m.userId]), 0);
  /** Clears the mutation state and closes. */
  const close = () => {
    save.reset();
    onClose();
  };
  return (
    <Dialog open={open} onOpenChange={(o) => !o && close()} title="Pesos no acerto" description="Quanto cada pessoa paga dos gastos do grupo. Vale a partir de hoje; gastos anteriores mantêm os pesos que tinham.">
      <form
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate(
            members.map((m) => ({ userId: m.userId, weightBp: bp(percent[m.userId]) })),
            { onSuccess: close },
          );
        }}
        className="flex flex-col gap-4"
        noValidate
      >
        {members.map((m) => (
          <Field key={m.userId} label={`${m.name} (%)`} htmlFor={`weight-${m.userId}`}>
            <Input
              id={`weight-${m.userId}`}
              inputMode="decimal"
              value={percent[m.userId] ?? ""}
              onChange={(e) => setPercent({ ...percent, [m.userId]: e.target.value })}
            />
          </Field>
        ))}
        <p className={cn("text-sm", total === 10000 ? "text-muted-foreground" : "text-destructive")}>Soma: {(total / 100).toLocaleString("pt-BR")}%</p>
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
