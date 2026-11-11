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
import { CardsPage } from "@/pages/CardsPage";
import { CategoriesPage } from "@/pages/CategoriesPage";
import { IncomesPage } from "@/pages/IncomesPage";
import { LoginPage } from "@/pages/LoginPage";
import { PlaceholderPage } from "@/pages/PlaceholderPage";
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
      { index: true, element: <PlaceholderPage id="overview" /> },
      { path: "cash-flow", element: <PlaceholderPage id="cash-flow" /> },
      { path: "cards", element: <CardsPage /> },
      { path: "alelo", element: <PlaceholderPage id="alelo" /> },
      { path: "bills", element: <PlaceholderPage id="bills" /> },
      { path: "savings", element: <PlaceholderPage id="savings" /> },
      { path: "receivables", element: <PlaceholderPage id="receivables" /> },
      { path: "market", element: <PlaceholderPage id="market" /> },
      { path: "settings", element: <SettingsPage /> },
      { path: "settings/categories", element: <CategoriesPage /> },
      { path: "settings/accounts", element: <AccountsPage /> },
      { path: "settings/accounts/:id", element: <AccountDetailPage /> },
      { path: "settings/incomes", element: <IncomesPage /> },
      { path: "*", element: <PlaceholderPage id="overview" /> },
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
