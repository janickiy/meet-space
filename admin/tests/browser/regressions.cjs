const fs = require("node:fs/promises");
const assert = require("node:assert/strict");
const { serve, fixture, chromium } = require("./harness.cjs");
const [directory = "dist", reportPath, mode] = process.argv.slice(2);
const baseline = mode === "baseline";

(async () => {
  const server = await serve(directory);
  const browser = await chromium.launch({ channel: "chrome", headless: true });
  const result = { kind: "isolated-browser-fixture", baseline, tests: {} };
  try {
    const offline = await browser.newPage();
    await fixture(offline, {
      intercept: async ({ route, endpoint }) => {
        if (endpoint !== "/auth/logout") return false;
        await route.abort("failed");
        return true;
      },
    });
    await offline.goto(server.url + "/users");
    await offline
      .getByText("Пользователь аккаунта 1", { exact: true })
      .waitFor();
    await offline
      .getByRole("button", { name: "Мой профиль", exact: true })
      .click();
    await offline.getByRole("button", { name: "Выйти", exact: true }).click();
    await offline
      .getByText(/Локальные данные очищены|Не удалось связаться с сервером/)
      .waitFor();
    result.tests.privateVisibleAfterFailedLogout = await offline
      .locator("main")
      .isVisible();
    await offline.reload();
    await offline.getByRole("heading", { level: 1 }).first().waitFor();
    result.tests.privateVisibleAfterFailedLogoutReload = await offline
      .locator("main")
      .isVisible();
    if (!baseline) {
      assert.equal(result.tests.privateVisibleAfterFailedLogout, false);
      assert.equal(result.tests.privateVisibleAfterFailedLogoutReload, false);
    }
    await offline.close();

    const modalFocus = await browser.newPage();
    await fixture(modalFocus);
    await modalFocus.goto(server.url + "/users");
    await modalFocus
      .getByRole("button", { name: "Блокировать", exact: true })
      .click();
    await modalFocus
      .getByRole("button", { name: "Заблокировать", exact: true })
      .click();
    await modalFocus.getByRole("dialog").waitFor({ state: "hidden" });
    result.tests.modalRestoresTriggerFocusAfterMutation = await modalFocus
      .getByRole("button", { name: "Разблокировать", exact: true })
      .evaluate((element) => element === document.activeElement);
    if (!baseline)
      assert.equal(result.tests.modalRestoresTriggerFocusAfterMutation, true);
    await modalFocus.close();

    const accountSwitch = await browser.newPage();
    await accountSwitch.clock.install();
    let releaseUser;
    const userGate = new Promise((resolve) => {
      releaseUser = resolve;
    });
    const backend = await fixture(accountSwitch, {
      intercept: async ({ endpoint, identity, fulfill }) => {
        if (endpoint !== "/users" || identity !== 2) return false;
        await userGate;
        try {
          await fulfill({
            items: [
              {
                id: "new-user",
                name: "Пользователь аккаунта 2",
                status: "active",
                createdAt: "2026-11-05T10:00:00Z",
              },
            ],
            total: 1,
            page: 1,
            pageSize: 10,
          });
        } catch {
          /* The superseded request can be cancelled. */
        }
        return true;
      },
    });
    await accountSwitch.goto(server.url + "/users");
    await accountSwitch
      .getByText("Пользователь аккаунта 1", { exact: true })
      .waitFor();
    backend.setIdentity(2);
    await accountSwitch.clock.fastForward(61_000);
    await accountSwitch.evaluate(() =>
      window.dispatchEvent(new Event("visibilitychange")),
    );
    await accountSwitch
      .locator(".topbar-profile")
      .filter({ hasText: "audit_operator_2" })
      .waitFor();
    result.tests.previousAccountDataVisibleDuringSwitch = await accountSwitch
      .getByText("Пользователь аккаунта 1", { exact: true })
      .isVisible();
    releaseUser();
    await accountSwitch
      .getByText("Пользователь аккаунта 2", { exact: true })
      .waitFor();
    if (!baseline)
      assert.equal(result.tests.previousAccountDataVisibleDuringSwitch, false);
    await accountSwitch.close();

    const lateMe = await browser.newPage();
    await lateMe.clock.install();
    let meCalls = 0,
      releaseMe,
      meWaiting;
    const meGate = new Promise((resolve) => {
      releaseMe = resolve;
    });
    const waiting = new Promise((resolve) => {
      meWaiting = resolve;
    });
    await fixture(lateMe, {
      intercept: async ({ endpoint, fulfill, session }) => {
        if (endpoint !== "/auth/me" || ++meCalls !== 2) return false;
        const captured = session();
        meWaiting();
        await meGate;
        try {
          await fulfill(captured);
        } catch {
          /* Expected after explicit cancellation. */
        }
        return true;
      },
    });
    await lateMe.goto(server.url + "/users");
    await lateMe
      .getByText("Пользователь аккаунта 1", { exact: true })
      .waitFor();
    await lateMe.clock.fastForward(61_000);
    await lateMe.evaluate(() =>
      window.dispatchEvent(new Event("visibilitychange")),
    );
    await waiting;
    await lateMe
      .getByRole("button", { name: "Мой профиль", exact: true })
      .click();
    await lateMe.getByRole("button", { name: "Выйти", exact: true }).click();
    await lateMe
      .getByRole("heading", { name: "Вход в админку", exact: true })
      .waitFor();
    await lateMe.evaluate(() => {
      window.auditPrivateRestored = false;
      window.auditObserver = new MutationObserver(() => {
        if (document.querySelector("main")) window.auditPrivateRestored = true;
      });
      window.auditObserver.observe(document.body, {
        childList: true,
        subtree: true,
      });
    });
    releaseMe();
    await lateMe.waitForTimeout(250);
    result.tests.lateMeRestoredPrivateShell = await lateMe.evaluate(() => {
      window.auditObserver.disconnect();
      return window.auditPrivateRestored;
    });
    if (!baseline) assert.equal(result.tests.lateMeRestoredPrivateShell, false);
    await lateMe.close();

    const polling = await browser.newPage();
    await polling.clock.install();
    const pollBackend = await fixture(polling);
    await polling.goto(server.url);
    await polling
      .getByText("Изолированная проверка", { exact: true })
      .waitFor();
    await polling
      .getByRole("link", { name: "Пользователи", exact: true })
      .click();
    await polling
      .getByText("Пользователь аккаунта 1", { exact: true })
      .waitFor();
    const dashboardRequests = pollBackend.counts["/dashboard"];
    const conferenceRequests = pollBackend.counts["/conferences"];
    await polling.clock.fastForward(90_000);
    assert.equal(pollBackend.counts["/dashboard"], dashboardRequests);
    assert.equal(pollBackend.counts["/conferences"], conferenceRequests);
    await polling.getByRole("link", { name: "Система", exact: true }).click();
    await polling
      .getByRole("heading", { name: "Состояние системы", exact: true })
      .waitFor();
    await polling
      .getByRole("link", { name: "Пользователи", exact: true })
      .click();
    await polling
      .getByText("Пользователь аккаунта 1", { exact: true })
      .waitFor();
    const systemRequests = pollBackend.counts["/system"];
    await polling.clock.fastForward(90_000);
    assert.equal(pollBackend.counts["/system"], systemRequests);
    result.tests.pollingStopsAfterNavigation = true;
    await polling.close();

    const failures = await browser.newPage();
    let listStatus = 503;
    await fixture(failures, {
      intercept: async ({ endpoint, fulfill }) => {
        if (endpoint !== "/users") return false;
        await fulfill(
          listStatus === 200
            ? { items: [], total: 0, page: 1, pageSize: 10 }
            : { message: `Тестовый ответ ${listStatus}` },
          listStatus,
        );
        return true;
      },
    });
    await failures.goto(server.url + "/users");
    await failures.getByText("Тестовый ответ 503", { exact: true }).waitFor();
    listStatus = 403;
    await failures
      .getByRole("button", { name: "Повторить", exact: true })
      .click();
    await failures.getByText("Тестовый ответ 403", { exact: true }).waitFor();
    listStatus = 200;
    await failures
      .getByRole("button", { name: "Повторить", exact: true })
      .click();
    await failures
      .getByRole("heading", { name: "Ничего не найдено", exact: true })
      .waitFor();
    result.tests.errorRetryAndEmptyState = true;
    await failures.close();

    const mobile = await browser.newPage({
      viewport: { width: 390, height: 844 },
      isMobile: true,
      hasTouch: true,
    });
    await fixture(mobile);
    await mobile.goto(server.url);
    await mobile.getByText("Изолированная проверка", { exact: true }).waitFor();
    result.tests.mobileDocumentWidth = await mobile.evaluate(
      () => document.documentElement.scrollWidth,
    );
    if (!baseline) {
      assert.equal(result.tests.mobileDocumentWidth, 390);
      await mobile
        .getByRole("button", { name: "Мой профиль", exact: true })
        .click();
      await mobile.getByRole("button", { name: "Выйти", exact: true }).click();
      await mobile
        .getByRole("heading", { name: "Вход в админку", exact: true })
        .waitFor();
      assert.equal(
        await mobile.evaluate(() => document.documentElement.scrollWidth),
        390,
      );
    }
    await mobile.close();

    result.completed = true;
    if (reportPath)
      await fs.writeFile(reportPath, JSON.stringify(result, null, 2));
    process.stdout.write(JSON.stringify(result) + "\n");
  } finally {
    await browser.close();
    await server.close();
  }
})();
