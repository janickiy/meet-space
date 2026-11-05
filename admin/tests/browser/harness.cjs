const http = require("node:http");
const fs = require("node:fs/promises");
const path = require("node:path");
const crypto = require("node:crypto");
const { chromium } = require(
  process.env.PLAYWRIGHT_MODULE_PATH || "playwright",
);

async function serve(directory) {
  const root = path.resolve(directory);
  const server = http.createServer(async (request, response) => {
    try {
      const relative = decodeURIComponent(
        new URL(request.url, "http://localhost").pathname,
      );
      let file = path.join(root, relative);
      if (!file.startsWith(root + path.sep) && file !== root)
        throw new Error("Invalid path");
      if (!path.extname(relative)) file = path.join(root, "index.html");
      const content = await fs.readFile(file);
      const type =
        {
          ".html": "text/html",
          ".js": "text/javascript",
          ".css": "text/css",
          ".woff": "font/woff",
          ".woff2": "font/woff2",
        }[path.extname(file)] || "application/octet-stream";
      response.writeHead(200, {
        "Content-Type": type,
        "Cache-Control": "no-store",
      });
      response.end(content);
    } catch {
      response.writeHead(404);
      response.end();
    }
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  return {
    url: `http://127.0.0.1:${server.address().port}`,
    close: () => new Promise((resolve) => server.close(resolve)),
  };
}

async function fixture(page, options = {}) {
  let identity = 1,
    signedIn = options.signedIn !== false;
  let sequence = 0;
  const csrf = crypto.randomBytes(24).toString("hex");
  const counts = {},
    errors = [];
  const observed = { blocked: false };
  const timestamp = "2026-11-05T10:00:00Z";
  const administrator = () => ({
    id: `audit-administrator-${identity}`,
    username: `audit_operator_${identity}`,
    name: `Оператор проверки ${identity}`,
    status: "active",
    email: null,
    createdAt: timestamp,
    updatedAt: timestamp,
    lastLoginAt: timestamp,
  });
  const session = () => ({ administrator: administrator(), csrfToken: csrf });
  const user = () => ({
    id: "audit-user",
    name: `Пользователь аккаунта ${identity}`,
    email: null,
    status: observed.blocked ? "blocked" : "active",
    createdAt: timestamp,
  });
  const service = {
    name: "API",
    status: "healthy",
    latencyMs: 1,
    version: null,
    checkedAt: timestamp,
  };
  const list = (items, url) => ({
    items,
    total: items.length,
    page: Number(url.searchParams.get("page") || 1),
    pageSize: Number(url.searchParams.get("pageSize") || 10),
  });
  page.on("pageerror", (error) => errors.push(error.message));
  await page.route("**/api/v1/admin/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const endpoint = url.pathname.replace("/api/v1/admin", "");
    counts[endpoint] = (counts[endpoint] || 0) + 1;
    if (++sequence > 2500)
      throw new Error("Bounded frontend fixture request budget exceeded");
    const fulfill = (data, status = 200) =>
      route.fulfill({
        status,
        contentType: "application/json",
        body: status === 204 ? undefined : JSON.stringify(data),
      });
    if (
      await options.intercept?.({
        route,
        request,
        endpoint,
        fulfill,
        session,
        identity,
      })
    )
      return;
    if (endpoint === "/auth/login") {
      signedIn = true;
      return fulfill(session());
    }
    if (!signedIn)
      return fulfill(
        { error: "unauthorized", message: "Сессия завершена" },
        401,
      );
    if (endpoint === "/auth/me") return fulfill(session());
    if (endpoint === "/auth/logout") {
      signedIn = false;
      identity++;
      return fulfill(null, 204);
    }
    if (
      request.method() !== "GET" &&
      request.headers()["x-csrf-token"] !== csrf
    )
      return fulfill({ message: "CSRF отклонён" }, 403);
    if (endpoint === "/users/audit-user" && request.method() === "PATCH") {
      observed.blocked = request.postDataJSON().status === "blocked";
      return fulfill(user());
    }
    const items = {
      "/users": list([user()], url),
      "/administrators": list([administrator()], url),
      "/conferences": list(
        [
          {
            id: "audit-conference",
            title: "Изолированная проверка",
            ownerName: "Тестовый организатор",
            status: "active",
            createdAt: timestamp,
            startedAt: timestamp,
            endedAt: null,
            participantCount: 3,
            recordingStatus: "ready",
          },
        ],
        url,
      ),
      "/recordings": list(
        [
          {
            id: "audit-recording",
            title: "Тестовая запись",
            conferenceId: "audit-conference",
            conferenceTitle: "Изолированная проверка",
            status: "recording",
            durationSeconds: 30,
            sizeBytes: null,
            createdAt: timestamp,
            canDelete: false,
          },
        ],
        url,
      ),
      "/chats": {
        directChats: 3,
        groupChats: 2,
        conferenceChats: 1,
        messages: 10,
        attachments: 2,
      },
      "/files": {
        count: 4,
        bytes: 4096,
        capacityBytes: null,
        categories: [{ name: "Записи", count: 4, bytes: 4096 }],
        scope: "Изолированные тестовые ответы.",
      },
      "/system": {
        services: [service],
        resources: {
          cpuPercent: null,
          memoryUsedBytes: null,
          memoryTotalBytes: null,
          diskUsedBytes: null,
          diskTotalBytes: null,
        },
      },
      "/settings": {
        items: [
          {
            key: "fixture",
            label: "Параметр тестового стенда",
            value: true,
            editable: false,
            type: "boolean",
            group: "general",
            secret: false,
            configured: true,
            description: "Изолированная проверка.",
          },
        ],
      },
      "/audit": list(
        [
          {
            id: "audit-event",
            administratorUsername: administrator().username,
            action: "auth.login",
            objectType: "administrator",
            objectId: administrator().id,
            ip: "192.0.2.1",
            userAgent: "Browser test",
            metadata: {},
            createdAt: timestamp,
          },
        ],
        url,
      ),
      "/dashboard": {
        users: { total: 7, active: 6, blocked: 1, new: 2 },
        conferences: { total: 10, active: 1, completed: 9, participants: 3 },
        recordings: { total: 4, active: 1, bytes: 4096 },
        files: { count: 4, bytes: 4096, capacityBytes: null },
        services: [service],
        activity: Array.from({ length: 7 }, (_, i) => ({
          date: `2026-10-${25 + i}`,
          count: i,
        })),
        checkedAt: timestamp,
      },
    };
    return fulfill(items[endpoint] || {}, items[endpoint] ? 200 : 404);
  });
  return {
    counts,
    errors,
    administrator,
    setIdentity: (value) => {
      identity = value;
    },
    setSignedIn: (value) => {
      signedIn = value;
    },
  };
}

async function sample(page, cdp, label, startedAt) {
  // Let React commit the navigation before measuring detached nodes after GC.
  await page.evaluate(
    () =>
      new Promise((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(resolve)),
      ),
  );
  await cdp.send("HeapProfiler.collectGarbage");
  await cdp.send("HeapProfiler.collectGarbage");
  const perf = await cdp.send("Performance.getMetrics");
  const metric = Object.fromEntries(
    perf.metrics.map((item) => [item.name, item.value]),
  );
  const dom = await cdp.send("Memory.getDOMCounters");
  return {
    label,
    elapsedSeconds: Math.round((Date.now() - startedAt) / 1000),
    heapUsedBytes: metric.JSHeapUsedSize,
    heapTotalBytes: metric.JSHeapTotalSize,
    documents: dom.documents,
    nodes: dom.nodes,
    listeners: dom.jsEventListeners,
    visibleNodes: await page.locator("*").count(),
  };
}

module.exports = { serve, fixture, sample, chromium };
