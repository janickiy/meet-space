import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import "@fontsource/inter/latin-400.css";
import "@fontsource/inter/latin-500.css";
import "@fontsource/inter/latin-600.css";
import "@fontsource/inter/latin-700.css";
import "@fontsource/inter/cyrillic-400.css";
import "@fontsource/inter/cyrillic-500.css";
import "@fontsource/inter/cyrillic-600.css";
import "@fontsource/inter/cyrillic-700.css";
import { APIError, api, hasAdministrativeSession } from "./api";
import { installAdminTelemetry, reportAdminError } from "./clientTelemetry";
import App from "./App";
import "./styles.css";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 15_000,
      retry: (failures, error) =>
        !(
          error instanceof APIError && [401, 403, 404].includes(error.status)
        ) && failures < 1,
      refetchOnWindowFocus: true,
    },
    mutations: { retry: false },
  },
});

const uninstallTelemetry = installAdminTelemetry({
  version: import.meta.env.VITE_BUILD_VERSION || "dev",
  origin: window.location.origin,
  pathname: () => window.location.pathname,
  userAgent: navigator.userAgent,
  isActive: hasAdministrativeSession,
  send: (event, signal) =>
    api("/errors/client", { method: "POST", body: event, signal }),
});
if (import.meta.hot) import.meta.hot.dispose(uninstallTelemetry);

ReactDOM.createRoot(document.getElementById("root")!, {
  onCaughtError: (error) => reportAdminError("react_render_error", error),
  onUncaughtError: (error) => reportAdminError("react_render_error", error),
}).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </QueryClientProvider>
  </React.StrictMode>,
);
