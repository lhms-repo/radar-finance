import { AnimatePresence, motion } from "motion/react";
import { LogOut, Moon, PanelLeftClose, PanelLeftOpen, Sun } from "lucide-react";
import { useRef, useState } from "react";
import { NavLink, useLocation } from "react-router";
import { NAV_ITEMS, activeItem, type NavItem } from "@/app/nav";
import { GroupSwitcher } from "@/components/layout/GroupSwitcher";
import { Logo } from "@/components/layout/Logo";
import { useLogout, useMe } from "@/app/session";
import { usePreferences } from "@/lib/preferences";
import { fade, softSpring, spring } from "@/lib/motion";
import { cn, readStorage, writeStorage } from "@/lib/utils";

const COLLAPSED_KEY = "radar.sidebar.collapsed";

/** Desktop sidebar, open or collapsed, with sliding indicators (section 5.5). */
export function Sidebar() {
  const [collapsed, setCollapsed] = useState(() => readStorage(COLLAPSED_KEY) === "1");
  const location = useLocation();
  const me = useMe();
  const active = activeItem(location.pathname);
  const [hovered, setHovered] = useState<string | null>(null);
  const [tooltip, setTooltip] = useState<{ id: string; label: string; top: number } | null>(null);
  const navRef = useRef<HTMLElement>(null);

  /** Switches between the open and the collapsed sidebar. */
  const toggle = () => {
    setCollapsed((c) => {
      writeStorage(COLLAPSED_KEY, c ? "0" : "1");
      return !c;
    });
    setTooltip(null);
  };

  /** Records the hovered item and, when collapsed, where the tooltip should go. */
  const onEnter = (item: NavItem, el: HTMLElement) => {
    setHovered(item.id);
    if (collapsed && navRef.current) {
      const top = el.offsetTop + el.offsetHeight / 2;
      setTooltip({ id: item.id, label: item.label, top });
    }
  };

  // The settlement only exists in a shared group (RF-84).
  const shown = (i: NavItem) => !(i.sharedOnly && me?.group.personal);
  const groups: { title: string; items: NavItem[] }[] = [
    { title: "Menu", items: NAV_ITEMS.filter((i) => i.group === "menu" && shown(i)) },
    { title: "Ajustes", items: NAV_ITEMS.filter((i) => i.group === "settings") },
  ];

  return (
    <motion.aside
      initial={false}
      animate={{ width: collapsed ? 88 : 264 }}
      transition={softSpring}
      className="sticky top-0 hidden h-dvh shrink-0 p-3 pr-0 md:block"
      aria-label="Menu principal"
    >
      <div className="relative flex h-full flex-col rounded-2xl bg-sidebar p-3 shadow-card">
        <div className={cn("flex items-center gap-3 px-1 pb-4 pt-1", collapsed && "justify-center px-0")}>
          <Logo className="size-9 shrink-0" />
          {!collapsed && (
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-semibold">Radar</p>
              <p className="truncate text-xs text-muted-foreground">{me?.group.name ?? "Finanças em grupo"}</p>
            </div>
          )}
        </div>

        <nav
          ref={navRef}
          className="relative flex-1 overflow-visible"
          onMouseLeave={() => {
            setHovered(null);
            setTooltip(null);
          }}
        >
          {groups.map((group) => (
            <div key={group.title} className="mb-4">
              {!collapsed && <p className="mb-1 px-3 text-xs text-muted-foreground">{group.title}</p>}
              <ul className="flex flex-col gap-0.5">
                {group.items.map((item) => {
                  const isActive = active?.id === item.id;
                  const Icon = item.icon;
                  return (
                    <li key={item.id}>
                      <NavLink
                        to={item.path}
                        aria-label={collapsed ? item.label : undefined}
                        onMouseEnter={(e) => onEnter(item, e.currentTarget)}
                        onFocus={(e) => onEnter(item, e.currentTarget)}
                        className={cn(
                          "relative flex h-10 items-center gap-3 rounded-xl px-3 text-sm outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring/50",
                          collapsed && "justify-center px-0",
                          isActive ? "font-semibold text-foreground" : "text-muted-foreground hover:text-foreground",
                        )}
                      >
                        {hovered === item.id && !isActive && (
                          <motion.span
                            layoutId="sidebar-hover"
                            transition={spring}
                            className="absolute inset-0 rounded-xl bg-sidebar-active/60"
                          />
                        )}
                        {isActive && (
                          <motion.span
                            layoutId="sidebar-active"
                            transition={spring}
                            className="absolute inset-0 rounded-xl bg-sidebar-active"
                          />
                        )}
                        <Icon className={cn("relative size-[18px] shrink-0", isActive && "text-primary")} />
                        {!collapsed && <span className="relative truncate">{item.label}</span>}
                      </NavLink>
                    </li>
                  );
                })}
              </ul>
            </div>
          ))}

          {collapsed && <FluidTooltip tooltip={tooltip} />}
        </nav>

        <SidebarFooter collapsed={collapsed} onToggle={toggle} />
      </div>
    </motion.aside>
  );
}

/**
 * Single tooltip that slides and morphs between icons in the collapsed sidebar,
 * inspired by the Fluid Tooltip Animation reference.
 */
function FluidTooltip({ tooltip }: { tooltip: { id: string; label: string; top: number } | null }) {
  return (
    <AnimatePresence>
      {tooltip && (
        <motion.div
          key="fluid-tooltip"
          className="pointer-events-none absolute left-full z-50 ml-4"
          initial={{ opacity: 0, top: tooltip.top, scale: 0.92, x: -6 }}
          animate={{ opacity: 1, top: tooltip.top, scale: 1, x: 0 }}
          exit={{ opacity: 0, scale: 0.92, x: -6, transition: fade }}
          transition={softSpring}
          style={{ translateY: "-50%" }}
        >
          <motion.div
            layout
            transition={softSpring}
            className="relative overflow-hidden whitespace-nowrap rounded-lg bg-foreground px-3 py-1.5 text-xs font-medium text-background shadow-lg"
          >
            <AnimatePresence mode="popLayout" initial={false}>
              <motion.span
                key={tooltip.id}
                className="block"
                initial={{ opacity: 0, y: 8, filter: "blur(4px)" }}
                animate={{ opacity: 1, y: 0, filter: "blur(0px)" }}
                exit={{ opacity: 0, y: -8, filter: "blur(4px)" }}
                transition={fade}
              >
                {tooltip.label}
              </motion.span>
            </AnimatePresence>
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}

/** Theme switch, collapse button and the logged in user (section 5.1). */
function SidebarFooter({ collapsed, onToggle }: { collapsed: boolean; onToggle: () => void }) {
  const { resolvedTheme, setTheme } = usePreferences();
  const me = useMe();
  const logout = useLogout();

  return (
    <div className="flex flex-col gap-2 border-t pt-3">
      <div className={cn("flex items-center gap-2", collapsed ? "flex-col" : "justify-between px-1")}>
        {!collapsed && <span className="text-sm text-muted-foreground">Tema</span>}
        <div className={cn("flex rounded-full bg-muted p-0.5", collapsed && "flex-col")}>
          {(["light", "dark"] as const).map((t) => {
            const Icon = t === "light" ? Sun : Moon;
            const isActive = resolvedTheme === t;
            return (
              <button
                key={t}
                type="button"
                aria-label={t === "light" ? "Tema claro" : "Tema escuro"}
                aria-pressed={isActive}
                onClick={(e) => {
                  const r = e.currentTarget.getBoundingClientRect();
                  setTheme(t, { x: r.left + r.width / 2, y: r.top + r.height / 2 });
                }}
                className={cn(
                  "relative grid size-7 place-items-center rounded-full transition-colors",
                  isActive ? "text-foreground" : "text-muted-foreground hover:text-foreground",
                )}
              >
                {isActive && (
                  <motion.span layoutId="theme-pill" transition={spring} className="absolute inset-0 rounded-full bg-card shadow-sm" />
                )}
                <Icon className="relative size-3.5" />
              </button>
            );
          })}
        </div>
      </div>

      <button
        type="button"
        onClick={onToggle}
        className={cn(
          "flex h-9 items-center gap-3 rounded-xl px-3 text-sm text-muted-foreground transition-colors hover:bg-sidebar-active hover:text-foreground",
          collapsed && "justify-center px-0",
        )}
        aria-label={collapsed ? "Abrir menu" : "Recolher menu"}
      >
        {collapsed ? <PanelLeftOpen className="size-[18px]" /> : <PanelLeftClose className="size-[18px]" />}
        {!collapsed && <span>Recolher menu</span>}
      </button>

      {!collapsed && <GroupSwitcher className="px-1" />}

      {me && (
        <div className={cn("flex items-center gap-3 rounded-xl bg-muted/60 p-2", collapsed && "flex-col p-1.5")}>
          <span className="grid size-8 shrink-0 place-items-center rounded-full bg-primary text-xs font-semibold text-primary-foreground">
            {me.name.charAt(0).toUpperCase()}
          </span>
          {!collapsed && (
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium">{me.name}</p>
              <p className="truncate text-xs text-muted-foreground">@{me.login}</p>
            </div>
          )}
          <button
            type="button"
            onClick={() => logout.mutate()}
            aria-label="Trocar de usuário"
            title="Trocar de usuário"
            className="grid size-8 place-items-center rounded-lg text-muted-foreground transition-colors hover:bg-card hover:text-foreground"
          >
            <LogOut className="size-4" />
          </button>
        </div>
      )}
    </div>
  );
}
