// Copyright 2026 Alexander Yanitskiy
// Контакты автора: см. NOTICE в корне проекта.
// SPDX-License-Identifier: Apache-2.0

import { StrictMode } from "react";
import { createRoot, hydrateRoot } from "react-dom/client";
import { BrowserRouter, useLocation } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import "@fontsource/inter/latin-400.css";
import "@fontsource/inter/latin-500.css";
import "@fontsource/inter/latin-600.css";
import "@fontsource/inter/latin-700.css";
import "@fontsource/inter/cyrillic-400.css";
import "@fontsource/inter/cyrillic-500.css";
import "@fontsource/inter/cyrillic-600.css";
import "@fontsource/inter/cyrillic-700.css";
import "./styles.css";
import "./accessibility.css";
import "./tokens.css";
import "./workspace.css";
import "./appearance.css";
import { ApiError } from "./api";
import { AuthProvider } from "./auth";
import { AppearanceProvider } from "./appearance";
import { App } from "./App";
import { AppErrorBoundary } from "./components/AppErrorBoundary";
import { installClientTelemetry } from "./clientTelemetry";
import { installRecordingAudioUnlock } from "./recordingAnnouncement";
import { PublicPage } from "./landing/PublicPage";
import { PublicMetadata } from "./landing/PublicMetadata";
import { publicPath } from "./landing/metadata";
import { MARKETING_ONLY } from "./landing/application";

const stopTelemetry = installClientTelemetry();
const stopRecordingAudioUnlock = installRecordingAudioUnlock();
if (import.meta.hot) import.meta.hot.dispose(stopTelemetry);
if (import.meta.hot) import.meta.hot.dispose(stopRecordingAudioUnlock);

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 10000,
      /**
       * retry решает, допустим ли повтор запроса с учётом ошибки и числа отказов.
       *
       * @args
       *   - count — число уже выполненных попыток.
       *   - error — пойманная ошибка API или сети.
       *
       * @returns вычисленное значение: !( error instanceof ApiError && error.status >= 400 && error.status < 500 ) && count < 1.
       */
      retry: (count, error) =>
        !(
          error instanceof ApiError &&
          error.status >= 400 &&
          error.status < 500
        ) && count < 1,
    },
    mutations: { retry: false },
  },
});
/** Public marketing pages remain usable without account recovery requests. */
function ApplicationBootstrap() {
  const { pathname } = useLocation();
  const path = publicPath(pathname);
  if (path && (MARKETING_ONLY || path !== "/")) {
    return (
      <>
        <PublicMetadata />
        <PublicPage path={path} />
      </>
    );
  }
  return (
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        <AppearanceProvider>
          <App />
        </AppearanceProvider>
      </AuthProvider>
    </QueryClientProvider>
  );
}

const application = (
  <StrictMode>
    <AppErrorBoundary>
      <BrowserRouter>
        <ApplicationBootstrap />
      </BrowserRouter>
    </AppErrorBoundary>
  </StrictMode>
);
const container = document.getElementById("root")!;
if (
  container.dataset.prerenderPath &&
  (MARKETING_ONLY || container.dataset.prerenderPath !== "/") &&
  container.dataset.prerenderPath === publicPath(window.location.pathname)
) {
  hydrateRoot(container, application);
} else {
  createRoot(container).render(application);
}
