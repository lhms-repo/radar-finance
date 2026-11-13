import {
  ArrowLeftRight,
  CalendarClock,
  Scale,
  CreditCard,
  HandCoins,
  LayoutGrid,
  PiggyBank,
  Settings,
  ShoppingCart,
  Utensils,
  type LucideIcon,
} from "lucide-react";

export type NavItem = {
  id: string;
  label: string;
  /** Shorter label for the bottom bar and the collapsed tooltip. */
  shortLabel: string;
  path: string;
  icon: LucideIcon;
  group: "menu" | "settings";
  /** Construction phase that delivers the screen (section 9). */
  phase: number;
  /** Only in a shared group: the personal space has nobody to settle with (RF-84). */
  sharedOnly?: boolean;
  description: string;
};

/** Every screen of section 5.2, in sidebar order. */
export const NAV_ITEMS: NavItem[] = [
  {
    id: "overview",
    label: "Visão geral",
    shortLabel: "Início",
    path: "/",
    icon: LayoutGrid,
    group: "menu",
    phase: 5,
    description: "Saldo previsto, contas a pagar, cartões, caixinhas e a evolução dos gastos num só lugar.",
  },
  {
    id: "cash-flow",
    label: "Fluxo de caixa",
    shortLabel: "Fluxo",
    path: "/cash-flow",
    icon: ArrowLeftRight,
    group: "menu",
    phase: 5,
    description: "Entradas e saídas do mês, saldos das contas e o gráfico de evolução dos gastos.",
  },
  {
    id: "cards",
    label: "Cartões",
    shortLabel: "Cartões",
    path: "/cards",
    icon: CreditCard,
    group: "menu",
    phase: 2,
    description: "Fatura detalhada, parcelas, recorrências e compras divididas de cada cartão.",
  },
  {
    id: "alelo",
    label: "Alelo",
    shortLabel: "Alelo",
    path: "/alelo",
    icon: Utensils,
    group: "menu",
    phase: 3,
    description: "Saldo acumulado e compras do mês de cada cartão Alelo.",
  },
  {
    id: "bills",
    label: "Contas fixas e financiamentos",
    shortLabel: "Contas",
    path: "/bills",
    icon: CalendarClock,
    group: "menu",
    phase: 3,
    description: "Contas do mês, calendário de vencimentos, parcelas restantes e saldo devedor.",
  },
  {
    id: "savings",
    label: "Caixinhas",
    shortLabel: "Caixinhas",
    path: "/savings",
    icon: PiggyBank,
    group: "menu",
    phase: 4,
    description: "Metas, progresso, aportes sugeridos e confirmação de depósito.",
  },
  {
    id: "receivables",
    label: "A receber",
    shortLabel: "A receber",
    path: "/receivables",
    icon: HandCoins,
    group: "menu",
    phase: 4,
    description: "Pendências de terceiros por pessoa, marcação de pago e mensagem de cobrança.",
  },
  {
    id: "settlement",
    label: "Acerto",
    shortLabel: "Acerto",
    path: "/settlement",
    icon: Scale,
    group: "menu",
    phase: 10,
    sharedOnly: true,
    description: "Quanto cada membro pagou, a cota de cada um e quem paga quem para zerar.",
  },
  {
    id: "market",
    label: "Mercado",
    shortLabel: "Mercado",
    path: "/market",
    icon: ShoppingCart,
    group: "menu",
    phase: 7,
    description: "Listas de compras, ranking de mercados e leitura da nota fiscal.",
  },
  {
    id: "settings",
    label: "Ajustes",
    shortLabel: "Ajustes",
    path: "/settings",
    icon: Settings,
    group: "settings",
    phase: 1,
    description: "Preferências, backup, saúde do sistema e exportação dos dados.",
  },
];

/** Items shown directly in the bottom bar of the phone, around the Add button (section 5.5). */
export const BOTTOM_LEFT = ["overview", "cards"];
export const BOTTOM_RIGHT = ["savings"];

/** Screens reached through the More sheet on the phone, in the order of section 5.5. */
export const MORE_ITEMS = ["cash-flow", "alelo", "market", "bills", "receivables", "settlement", "settings"];

/** Finds a navigation item by ID. */
export function navItem(id: string): NavItem {
  const item = NAV_ITEMS.find((i) => i.id === id);
  if (!item) throw new Error(`unknown nav item ${id}`);
  return item;
}

/** Returns the item whose path matches the current location. */
export function activeItem(pathname: string): NavItem | undefined {
  if (pathname === "/") return navItem("overview");
  return NAV_ITEMS.find((i) => i.path !== "/" && pathname.startsWith(i.path));
}
