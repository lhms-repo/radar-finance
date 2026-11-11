import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MotionConfig } from "motion/react";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { createBrowserRouter, RouterProvider } from "react-router";
import { Toaster } from "sonner";
import { AppShell } from "@/components/layout/AppShell";
import { MonthProvider } from "@/lib/month";
import { PreferencesProvider, usePreferences } from "@/lib/preferences";
import { AccountDetailPage } from "@/pages/AccountDetailPage";
import { AccountsPage } from "@/pages/AccountsPage";
import { AleloPage } from "@/pages/AleloPage";
import { BillsPage } from "@/pages/BillsPage";
import { BudgetsPage } from "@/pages/BudgetsPage";
import { CardsPage } from "@/pages/CardsPage";
import { CashFlowPage } from "@/pages/CashFlowPage";
import { CategoriesPage } from "@/pages/CategoriesPage";
import { ImportPage } from "@/pages/ImportPage";
import { IncomesPage } from "@/pages/IncomesPage";
import { ReceivablesPage } from "@/pages/ReceivablesPage";
import { RulesPage } from "@/pages/RulesPage";
import { SavingsPage } from "@/pages/SavingsPage";
import { LoginPage } from "@/pages/LoginPage";
import { ListPage } from "@/pages/ListPage";
import { MarketPage } from "@/pages/MarketPage";
import { OverviewPage } from "@/pages/OverviewPage";
import { ReceiptPage } from "@/pages/ReceiptPage";
import { SettingsPage } from "@/pages/SettingsPage";
import "./index.css";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, refetchOnWindowFocus: true },
  },
});

const router = createBrowserRouter([
  { path: "/login", element: <LoginPage /> },
  {
    element: <AppShell />,
    children: [
      { index: true, element: <OverviewPage /> },
      { path: "cash-flow", element: <CashFlowPage /> },
      { path: "cards", element: <CardsPage /> },
      { path: "alelo", element: <AleloPage /> },
      { path: "bills", element: <BillsPage /> },
      { path: "savings", element: <SavingsPage /> },
      { path: "receivables", element: <ReceivablesPage /> },
      { path: "market", element: <MarketPage /> },
      { path: "market/receipt", element: <ReceiptPage /> },
      { path: "market/lists/:id", element: <ListPage /> },
      { path: "settings", element: <SettingsPage /> },
      { path: "settings/categories", element: <CategoriesPage /> },
      { path: "settings/accounts", element: <AccountsPage /> },
      { path: "settings/accounts/:id", element: <AccountDetailPage /> },
      { path: "settings/incomes", element: <IncomesPage /> },
      { path: "settings/budgets", element: <BudgetsPage /> },
      { path: "settings/import", element: <ImportPage /> },
      { path: "settings/rules", element: <RulesPage /> },
      { path: "*", element: <OverviewPage /> },
    ],
  },
]);

/** Toasts that follow the app theme. */
function ThemedToaster() {
  const { resolvedTheme } = usePreferences();
  return <Toaster theme={resolvedTheme} position="top-center" richColors closeButton />;
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    {/* reducedMotion="user" turns off transform and layout animations when the OS asks to. */}
    <MotionConfig reducedMotion="user">
      <QueryClientProvider client={queryClient}>
        <PreferencesProvider>
          <MonthProvider>
            <RouterProvider router={router} />
            <ThemedToaster />
          </MonthProvider>
        </PreferencesProvider>
      </QueryClientProvider>
    </MotionConfig>
  </StrictMode>,
);
