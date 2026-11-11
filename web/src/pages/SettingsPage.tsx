import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AnimatePresence, motion } from "motion/react";
import { AlertTriangle, CheckCircle2, ChevronRight, Download, HandCoins, HardDrive, Landmark, LogOut, Monitor, Moon, Sun, Tags } from "lucide-react";
import { Link } from "react-router";
import { toast } from "sonner";
import { api, unwrap, type Backup, type Theme } from "@/api/client";
import { useLogout, useMe, useUpdatePreferences } from "@/app/session";
import { Button } from "@/components/ui/button";
import { Card, CardDescription, CardTitle } from "@/components/ui/card";
import { Segmented } from "@/components/ui/segmented";
import { Switch } from "@/components/ui/switch";
import { formatBytes, formatDateTime, formatUptime } from "@/lib/format";
import { haptic } from "@/lib/haptics";
import { usePreferences } from "@/lib/preferences";
import { spring } from "@/lib/motion";

const HEALTH_KEY = ["system", "health"] as const;
const BACKUPS_KEY = ["system", "backups"] as const;

const KIND_LABEL: Record<Backup["kind"], string> = {
  daily: "Diário",
  startup: "Ao ligar",
  manual: "Manual",
};

/** Settings screen: preferences, system health, backups and export. */
export function SettingsPage() {
  return (
    <div className="flex flex-col gap-4">
      <h1 className="px-1 text-lg font-semibold md:hidden">Ajustes</h1>
      <div className="grid gap-4 lg:grid-cols-2">
        <RecordsCard />
        <AppearanceCard />
        <HealthCard />
        <BackupsCard />
        <DataCard />
      </div>
    </div>
  );
}

/** Links to the record screens kept in Settings (section 5.2). */
function RecordsCard() {
  const links = [
    { to: "/settings/accounts", label: "Contas bancárias", hint: "Saldos, extrato, ajustes e transferências", icon: Landmark },
    { to: "/settings/incomes", label: "Rendas", hint: "Salários, bolsa, 13º e confirmação do recebimento", icon: HandCoins },
    { to: "/settings/categories", label: "Categorias", hint: "Criar, renomear, mover e desativar", icon: Tags },
  ];
  return (
    <Card className="flex flex-col gap-2 lg:col-span-2">
      <CardTitle>Cadastros</CardTitle>
      <ul className="flex flex-col">
        {links.map((l) => (
          <li key={l.to}>
            <Link to={l.to} className="-mx-2 flex items-center gap-3 rounded-xl px-2 py-2.5 hover:bg-accent active:bg-accent">
              <span className="grid size-10 place-items-center rounded-xl bg-muted">
                <l.icon className="size-5 text-primary" />
              </span>
              <span className="flex-1">
                <span className="block text-sm font-medium">{l.label}</span>
                <span className="block text-xs text-muted-foreground">{l.hint}</span>
              </span>
              <ChevronRight className="size-4 text-muted-foreground" />
            </Link>
          </li>
        ))}
      </ul>
    </Card>
  );
}

/** Theme and haptic feedback preferences (RNF-14, RNF-25). */
function AppearanceCard() {
  const { theme, setTheme, haptics, setHaptics } = usePreferences();
  const save = useUpdatePreferences();
  return (
    <Card className="flex flex-col gap-5">
      <div>
        <CardTitle>Aparência</CardTitle>
        <CardDescription>Vale para este usuário em todos os aparelhos.</CardDescription>
      </div>
      <Segmented<Theme>
        ariaLabel="Tema"
        value={theme}
        options={[
          { value: "light", label: "Claro", icon: <Sun className="size-4" /> },
          { value: "dark", label: "Escuro", icon: <Moon className="size-4" /> },
          { value: "system", label: "Sistema", icon: <Monitor className="size-4" /> },
        ]}
        onChange={(value, e) => {
          setTheme(value, { x: e.clientX, y: e.clientY });
          save.mutate({ theme: value });
        }}
      />
      <label className="flex items-center justify-between gap-4">
        <span>
          <span className="block text-sm font-medium">Resposta tátil</span>
          <span className="block text-sm text-muted-foreground">Vibração leve nos toques do iPhone.</span>
        </span>
        <Switch
          checked={haptics}
          onCheckedChange={(value) => {
            setHaptics(value);
            if (value) haptic();
            save.mutate({ haptics: value });
          }}
        />
      </label>
    </Card>
  );
}

/** System health: version, uptime, database size and backup state (section 8.5). */
function HealthCard() {
  const health = useQuery({
    queryKey: HEALTH_KEY,
    queryFn: async () => unwrap(await api.GET("/system/health")),
    refetchInterval: 30_000,
  });
  const h = health.data;
  return (
    <Card className="flex flex-col gap-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <CardTitle>Saúde do sistema</CardTitle>
          <CardDescription>Atualiza sozinho a cada 30 segundos.</CardDescription>
        </div>
        {h && (
          <span
            className={`flex shrink-0 items-center gap-1.5 whitespace-nowrap rounded-full px-2.5 py-1 text-xs font-medium ${
              h.backupError ? "bg-destructive/10 text-destructive" : "bg-success/10 text-success"
            }`}
          >
            {h.backupError ? <AlertTriangle className="size-3.5" /> : <CheckCircle2 className="size-3.5" />}
            {h.backupError ? "Atenção" : "Tudo certo"}
          </span>
        )}
      </div>
      {health.isPending && <p className="text-sm text-muted-foreground">Carregando...</p>}
      {health.isError && <p className="text-sm text-destructive">Não foi possível ler a saúde do sistema.</p>}
      {h && (
        <dl className="grid grid-cols-2 gap-x-4 gap-y-3 text-sm">
          <Item label="Último backup" value={h.lastBackup ? formatDateTime(h.lastBackup.createdAt) : "Nenhum ainda"} />
          <Item label="Próximo backup" value={formatDateTime(h.nextBackupAt)} />
          <Item label="Ligado há" value={formatUptime(h.uptimeSeconds)} />
          <Item label="Banco de dados" value={formatBytes(h.dbSizeBytes)} />
          <Item label="Versão" value={h.version} />
          <Item label="Cópias mantidas" value={`${h.backupKeep}`} />
          {h.backupError && (
            <div className="col-span-2 rounded-xl bg-destructive/10 px-3 py-2 text-destructive">
              Falha no último backup{h.backupErrorAt ? ` em ${formatDateTime(h.backupErrorAt)}` : ""}: {h.backupError}
            </div>
          )}
        </dl>
      )}
    </Card>
  );
}

/** One label and value of the health grid. */
function Item({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="font-medium">{value}</dd>
    </div>
  );
}

/** Backup list and the manual backup button (RNF-05, RNF-07). */
function BackupsCard() {
  const qc = useQueryClient();
  const backups = useQuery({ queryKey: BACKUPS_KEY, queryFn: async () => unwrap(await api.GET("/system/backups")) });
  const create = useMutation({
    mutationFn: async () => unwrap(await api.POST("/system/backups")),
    onSuccess: (b) => {
      haptic();
      toast.success("Backup feito", { description: b.name });
      void qc.invalidateQueries({ queryKey: ["system"] });
    },
    onError: (err) => toast.error("O backup falhou", { description: err.message }),
  });
  return (
    <Card className="flex flex-col gap-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <CardTitle>Backups</CardTitle>
          <CardDescription>Cópia diária automática e cópia ao ligar quando a do dia faltou.</CardDescription>
        </div>
        <Button size="sm" onClick={() => create.mutate()} disabled={create.isPending}>
          <HardDrive />
          {create.isPending ? "Fazendo..." : "Fazer agora"}
        </Button>
      </div>
      <ul className="flex max-h-72 flex-col overflow-y-auto">
        <AnimatePresence initial={false}>
          {(backups.data ?? []).map((b) => (
            <motion.li
              key={b.name}
              layout
              initial={{ opacity: 0, y: -8 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0 }}
              transition={spring}
              className="flex items-center justify-between gap-3 border-b py-2.5 text-sm last:border-b-0"
            >
              <span>
                <span className="block font-medium">{formatDateTime(b.createdAt)}</span>
                <span className="block text-xs text-muted-foreground">{KIND_LABEL[b.kind]}</span>
              </span>
              <span className="text-muted-foreground">{formatBytes(b.sizeBytes)}</span>
            </motion.li>
          ))}
        </AnimatePresence>
        {backups.data?.length === 0 && <li className="py-2 text-sm text-muted-foreground">Nenhum backup ainda.</li>}
      </ul>
    </Card>
  );
}

/** Data export and the session (RNF-18). */
function DataCard() {
  const me = useMe();
  const logout = useLogout();
  return (
    <Card className="flex flex-col gap-4">
      <div>
        <CardTitle>Dados e conta</CardTitle>
        <CardDescription>Exporta todas as tabelas em CSV, com valores em centavos.</CardDescription>
      </div>
      <div className="flex flex-wrap gap-2">
        <Button variant="outline" onClick={() => window.location.assign("/api/system/export")}>
          <Download />
          Exportar CSV
        </Button>
        <Button variant="ghost" onClick={() => logout.mutate()}>
          <LogOut />
          Trocar de usuário{me ? ` (${me.name})` : ""}
        </Button>
      </div>
    </Card>
  );
}
