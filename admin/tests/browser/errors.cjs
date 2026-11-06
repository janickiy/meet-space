const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const { serve, fixture, chromium } = require("./harness.cjs");
const now = new Date().toISOString();
const first = {
  id: "ec671d94-7c64-4c8f-9560-bb58ab088195",
  service: "api",
  instance: "synthetic-api",
  level: "ERROR",
  code: "http.server_error",
  message: "Не удалось обработать запрос",
  route: "/api/v1/synthetic/:id",
  requestId: "4617360d-47ac-44c0-9721-2d7b009bb3bc",
  release: "synthetic-release",
  firstSeen: now,
  lastSeen: now,
  count: 12,
};
const second = {
  ...first,
  id: "ea458574-c700-4e87-a249-790fce121b61",
  service: "frontend",
  level: "WARN",
  code: "uncaught_error",
  message: "Ошибка браузера",
  count: 2,
  browser: "chromium",
  stack: [{ file: "index-AbCd1234.js", line: 12, column: 34 }],
};
const settle = () => new Promise((r) => setTimeout(r, 30));
(async () => {
  const server = await serve(process.argv[2] || "dist");
  const browser = await chromium.launch({ channel: "chrome", headless: true });
  const report = { tests: {}, javascriptErrors: [], completed: false };
  const requests = [],
    telemetry = [];
  let releaseDetail, heldDetail;
  const detailHeld = new Promise((r) => (heldDetail = r));
  const detailReleased = new Promise((r) => (releaseDetail = r));
  const intercept = async ({ endpoint, request, fulfill }) => {
    const params = new URL(request.url()).searchParams;
    if (endpoint === "/errors/client") {
      telemetry.push(request.postDataJSON());
      await fulfill(null, 202);
      return true;
    }
    if (endpoint === "/errors") {
      requests.push(Object.fromEntries(params));
      const code = params.get("code");
      if (code === "fail") {
        await fulfill({ message: "Синтетический отказ" }, 503);
        return true;
      }
      const items =
        code === "empty"
          ? []
          : code === "hold"
            ? [{ ...first, id: "held" }]
            : code === "escape"
              ? [
                  {
                    ...first,
                    message: '<img src=x onerror="window.__injected=true">',
                  },
                ]
              : params.get("cursor") || params.get("service") === "frontend"
                ? [second]
                : [first];
      await fulfill({
        items,
        nextCursor:
          !params.get("cursor") && items.length ? "synthetic-next" : null,
        summary: { lastHour: 2, lastDay: 4, total: 6 },
        retentionDays: 14,
        maxRecords: 20000,
      });
      return true;
    }
    if (endpoint.startsWith("/errors/")) {
      if (endpoint === "/errors/held") {
        heldDetail();
        await detailReleased;
        await fulfill({
          ...first,
          message: "Старый ответ не должен появиться",
        });
        return true;
      }
      await fulfill(endpoint.endsWith(first.id) ? first : second);
      return true;
    }
    return false;
  };
  try {
    const page = await browser.newPage({
      viewport: { width: 1440, height: 1000 },
    });
    await page.clock.install();
    page.on("pageerror", (e) => report.javascriptErrors.push(e.name));
    await fixture(page, { intercept });
    await page.goto(server.url + "/errors");
    await page.getByRole("heading", { name: "Ошибки", exact: true }).waitFor();
    await page.getByText(first.message, { exact: true }).waitFor();
    assert.equal(
      await page.getByRole("link", { name: "Ошибки", exact: true }).count(),
      1,
    );
    assert.equal(
      await page.getByText("Групп ошибок за час", { exact: true }).count(),
      1,
    );
    assert.equal(await page.getByText(/Хранение 14 дней/).count(), 1);
    report.tests.listAndPolicy = true;
    await page.getByRole("button", { name: "Подробности ошибки" }).click();
    await page
      .getByRole("dialog")
      .getByText(first.requestId, { exact: true })
      .waitFor();
    await page.keyboard.press("Escape");
    assert.equal(await page.getByRole("dialog").count(), 0);
    report.tests.keyboardEscapeClosesDetails = true;
    await page.getByRole("button", { name: "Следующая страница" }).click();
    await page.getByText(second.message, { exact: true }).waitFor();
    assert.equal(requests.at(-1).cursor, "synthetic-next");
    await page.getByRole("button", { name: "Подробности ошибки" }).click();
    await page
      .getByRole("dialog")
      .getByText("index-AbCd1234.js:12:34", { exact: true })
      .waitFor();
    await page.getByRole("button", { name: "Закрыть", exact: true }).click();
    report.tests.cursorAndDetails = true;
    const beforeHistory = requests.length;
    await page.clock.fastForward(20_050);
    await settle();
    assert.equal(requests.length, beforeHistory);
    report.tests.historyDoesNotPoll = true;
    await page.getByLabel("Сервис", { exact: true }).selectOption("frontend");
    await page.getByLabel("Период", { exact: true }).selectOption("24h");
    await page.getByRole("button", { name: "Применить", exact: true }).click();
    await page.getByText(second.message, { exact: true }).waitFor();
    assert.equal(requests.at(-1).service, "frontend");
    assert.equal(requests.at(-1).cursor, undefined);
    report.tests.filtersResetCursor = true;
    const beforeAuto = requests.length;
    await page.clock.fastForward(10_050);
    await settle();
    assert(requests.length > beforeAuto);
    await page.getByLabel("Автообновление · 10 с", { exact: true }).uncheck();
    const paused = requests.length;
    await page.clock.fastForward(20_050);
    await settle();
    assert.equal(requests.length, paused);
    report.tests.autoRefreshAndPause = true;
    const beforeInvalid = requests.length;
    await page
      .getByLabel("ID последнего запроса", { exact: true })
      .fill("not-a-uuid");
    await page.getByRole("button", { name: "Применить", exact: true }).click();
    await page
      .getByText("ID запроса должен быть в формате UUID.", { exact: true })
      .waitFor();
    assert.equal(requests.length, beforeInvalid);
    await page
      .getByLabel("ID последнего запроса", { exact: true })
      .fill(first.requestId);
    await page.getByLabel("Период", { exact: true }).selectOption("custom");
    await page
      .getByLabel("Начало периода", { exact: true })
      .fill("2026-11-05T10:00");
    await page
      .getByLabel("Конец периода", { exact: true })
      .fill("2026-11-06T10:00");
    await page.getByRole("button", { name: "Применить", exact: true }).click();
    await settle();
    assert.equal(requests.at(-1).requestId, first.requestId);
    assert(
      requests.at(-1).from.endsWith("Z") && requests.at(-1).to.endsWith("Z"),
    );
    report.tests.explicitDatesAndRequestIdValidation = true;
    await page.getByRole("button", { name: "Сбросить", exact: true }).click();
    await page.getByText(first.message, { exact: true }).waitFor();
    await page.getByLabel("Код ошибки", { exact: true }).fill("empty");
    await page.getByRole("button", { name: "Применить", exact: true }).click();
    await page
      .getByRole("heading", { name: "Ошибок не найдено", exact: true })
      .waitFor();
    await page.getByLabel("Код ошибки", { exact: true }).fill("fail");
    await page.getByRole("button", { name: "Применить", exact: true }).click();
    await page.getByText("Синтетический отказ", { exact: true }).waitFor();
    await page.getByRole("button", { name: "Сбросить", exact: true }).click();
    await page.getByText(first.message, { exact: true }).waitFor();
    report.tests.emptyAndFailure = true;
    await page.getByLabel("Код ошибки", { exact: true }).fill("escape");
    await page.getByRole("button", { name: "Применить", exact: true }).click();
    await page
      .getByText('<img src=x onerror="window.__injected=true">', {
        exact: true,
      })
      .waitFor();
    assert.equal(await page.locator("main img").count(), 0);
    assert.equal(await page.evaluate(() => window.__injected), undefined);
    report.tests.messagesAreText = true;
    await page.getByRole("button", { name: "Сбросить", exact: true }).click();
    await page.getByText(first.message, { exact: true }).waitFor();
    await page.evaluate(() =>
      window.dispatchEvent(
        new ErrorEvent("error", {
          error: new Error("synthetic-private-value-not-for-log"),
        }),
      ),
    );
    await settle();
    assert.equal(telemetry.length, 1);
    assert.equal(telemetry[0].code, "uncaught_error");
    assert.equal(telemetry[0].route, "/errors");
    assert(!JSON.stringify(telemetry).includes("synthetic-private"));
    report.tests.safeTelemetry = true;
    await page.getByLabel("Код ошибки", { exact: true }).fill("hold");
    await page.getByRole("button", { name: "Применить", exact: true }).click();
    await page.getByRole("button", { name: "Подробности ошибки" }).click();
    await detailHeld;
    await page.getByRole("button", { name: "Закрыть", exact: true }).click();
    await page
      .getByRole("button", { name: "Мой профиль", exact: true })
      .click();
    await page.getByRole("button", { name: "Выйти", exact: true }).click();
    await page.getByRole("button", { name: "Войти", exact: true }).waitFor();
    releaseDetail();
    await settle();
    assert.equal(
      await page
        .getByText("Старый ответ не должен появиться", { exact: true })
        .count(),
      0,
    );
    report.tests.pendingDetailCannotSurviveLogout = true;
    const beforeSignedOut = telemetry.length;
    await page.evaluate(() =>
      window.dispatchEvent(
        new ErrorEvent("error", { error: new Error("synthetic-secret-login") }),
      ),
    );
    await page.clock.fastForward(20_000);
    await settle();
    assert.equal(telemetry.length, beforeSignedOut);
    report.tests.signedOutDoesNotReport = true;
    await page.close();
    await fs.mkdir("test-results/errors", { recursive: true });
    for (const width of [1440, 1217, 390])
      for (const theme of ["dark", "light"]) {
        const mobile = width === 390;
        const p = await browser.newPage({
          viewport: { width, height: mobile ? 844 : 1100 },
          deviceScaleFactor: width === 1217 ? 2 : 1,
          isMobile: mobile,
          hasTouch: mobile,
        });
        p.on("pageerror", (e) => report.javascriptErrors.push(e.name));
        await p.addInitScript(
          (theme) => localStorage.setItem("meetspace-admin-theme", theme),
          theme,
        );
        await fixture(p, { intercept });
        await p.goto(server.url + "/errors");
        await p.getByText(first.message, { exact: true }).waitFor();
        const layout = await p.evaluate(() => ({
          width: innerWidth,
          document: document.documentElement.scrollWidth,
          heights: [
            ...document.querySelectorAll(
              ".error-filter-grid input,.error-filter-grid select",
            ),
          ].map((e) => e.getBoundingClientRect().height),
        }));
        assert.equal(layout.width, layout.document);
        assert(layout.heights.every((h) => h === (mobile ? 44 : 40)));
        await p.screenshot({
          path: `test-results/errors/list-${width}-${theme}.png`,
          fullPage: !mobile,
        });
        if (mobile) {
          await p.locator(".error-filters").scrollIntoViewIfNeeded();
          await p.screenshot({
            path: `test-results/errors/filters-${width}-${theme}.png`,
            fullPage: false,
          });
        }
        await p.getByRole("button", { name: "Подробности ошибки" }).click();
        await p
          .getByRole("dialog")
          .getByText(first.requestId, { exact: true })
          .waitFor();
        await p.screenshot({
          path: `test-results/errors/detail-${width}-${theme}.png`,
          fullPage: !mobile,
        });
        await p.close();
      }
    report.tests.responsiveThemes = true;
    assert.equal(report.javascriptErrors.length, 0);
    report.completed = true;
    await fs.writeFile(
      "test-results/errors/report.json",
      JSON.stringify(report, null, 2) + "\n",
    );
    console.log(JSON.stringify(report));
  } finally {
    releaseDetail();
    await browser.close();
    await server.close();
  }
})().catch((e) => {
  console.error(e);
  process.exitCode = 1;
});
