const assert = require("node:assert/strict");
const path = require("node:path");
const { chromium, serve } = require("../../../admin/tests/browser/harness.cjs");

(async () => {
  const server = await serve(path.join(__dirname, "../../dist"));
  const browser = await chromium.launch({ channel: "chrome", headless: true });
  const errors = [];
  const user = {
    id: "00000000-0000-4000-8000-000000000001",
    email: "directory@example.test",
    displayName: "Directory User",
    authSource: "ldap",
    createdAt: "2026-11-07T10:00:00Z",
    updatedAt: "2026-11-07T10:00:00Z",
  };
  async function fixture(options = {}) {
    const state = {
      mode: "ldap",
      requests: [],
      providerFailure: false,
      ...options,
    };
    const page = await browser.newPage({
      viewport: { width: 1440, height: 1100 },
    });
    page.setDefaultTimeout(10000);
    page.on("pageerror", (error) => errors.push(error.message));
    let providerSeen;
    state.providerSeen = new Promise((resolve) => {
      providerSeen = resolve;
    });
    await page.route("**/api/v1/**", async (route) => {
      const request = route.request();
      const endpoint = new URL(request.url()).pathname.replace("/api/v1", "");
      const send = (data, status = 200) =>
        route.fulfill({
          status,
          contentType: "application/json",
          body: JSON.stringify(data),
        });
      if (endpoint === "/auth/providers") {
        providerSeen();
        const reply = () =>
          send(
            state.providerFailure
              ? { message: "Unavailable" }
              : {
                  mode: state.mode,
                  registrationEnabled: state.mode === "local",
                  passwordRecoveryEnabled: state.mode === "local",
                  ldap: {
                    enabled: state.mode === "ldap",
                    label: "Корпоративный вход",
                  },
                },
            state.providerFailure ? 503 : 200,
          );
        if (state.holdPolicy) {
          state.release = reply;
          return;
        }
        return reply();
      }
      if (
        [
          "/auth/login",
          "/auth/ldap/login",
          "/auth/register",
          "/auth/password-recovery",
          "/auth/reset-password",
          "/auth/verify-email",
          "/auth/verify-email/resend",
        ].includes(endpoint)
      ) {
        state.requests.push({ endpoint, body: request.postDataJSON() });
        return send({ message: "invalid credentials" }, 401);
      }
      if (endpoint === "/auth/refresh")
        return state.authenticated
          ? send({
              accessToken: "synthetic-directory-access-token",
              tokenType: "Bearer",
              expiresIn: 900,
              user,
            })
          : send({ message: "unauthorized" }, 401);
      if (endpoint === "/auth/me") return send({ user });
      if (endpoint === "/auth/accounts/list") return send({ items: [user] });
      return send({ items: [] });
    });
    return { page, state };
  }
  async function noLocalActions(page) {
    assert.equal(
      await page
        .getByRole("link", { name: "Создать аккаунт", exact: true })
        .count(),
      0,
    );
    assert.equal(
      await page
        .getByRole("button", { name: "Создать аккаунт", exact: true })
        .count(),
      0,
    );
    assert.equal(
      await page
        .getByRole("link", { name: "Забыли пароль?", exact: true })
        .count(),
      0,
    );
    assert.equal(
      await page
        .getByRole("button", { name: "Корпоративный вход", exact: true })
        .count(),
      0,
    );
    assert.equal(
      await page.getByRole("button", { name: "По email", exact: true }).count(),
      0,
    );
  }
  try {
    const ldap = await fixture();
    await ldap.page.goto(server.url + "/login");
    await ldap.page.getByLabel("Email", { exact: true }).fill(user.email);
    await noLocalActions(ldap.page);
    // LDAP passwords are not truncated to the local-account limit.
    const directoryPassword = "a".repeat(160);
    await ldap.page.locator("#password").fill(directoryPassword);
    await ldap.page.getByRole("button", { name: "Войти", exact: true }).click();
    await ldap.page.getByRole("alert").waitFor();
    assert.deepEqual(ldap.state.requests, [
      {
        endpoint: "/auth/login",
        body: { email: user.email, password: directoryPassword },
      },
    ]);
    for (const route of [
      "/register?next=%2Fcalendar",
      "/register/success",
      "/reset-password#token=SyntheticToken",
      "/verify-email#token=SyntheticToken",
    ]) {
      await ldap.page.goto(server.url + route);
      await ldap.page.waitForURL((url) => url.pathname === "/login");
      await ldap.page
        .getByRole("heading", { name: "Вход в MeetSpace" })
        .waitFor();
      await noLocalActions(ldap.page);
      assert.equal(
        ldap.state.requests.length,
        1,
        "prohibited forms must never issue requests",
      );
    }
    const managed = await fixture({ authenticated: true });
    await managed.page.goto(server.url + "/app/settings");
    const name = managed.page.getByLabel("Имя для встреч", { exact: true });
    await name.waitFor();
    assert.equal(await name.getAttribute("readonly"), "");
    assert.equal(
      await managed.page
        .getByRole("button", { name: "Сохранить имя", exact: true })
        .count(),
      0,
    );
    await managed.page
      .getByRole("tab", { name: "Аккаунты", exact: true })
      .click();
    assert.equal(
      await managed.page
        .getByRole("button", { name: "Удалить текущий аккаунт", exact: true })
        .count(),
      0,
    );
    await managed.page
      .getByRole("button", { name: "Добавить аккаунт", exact: true })
      .click();
    await managed.page
      .getByLabel("Email", { exact: true })
      .fill("second@example.test");
    await noLocalActions(managed.page);
    await managed.page
      .getByLabel("Пароль", { exact: true })
      .fill(directoryPassword);
    await managed.page
      .getByRole("button", { name: "Войти и добавить", exact: true })
      .click();
    await managed.page.locator(".account-management [role=alert]").waitFor();
    assert.deepEqual(managed.state.requests, [
      {
        endpoint: "/auth/login",
        body: { email: "second@example.test", password: directoryPassword },
      },
    ]);
    await managed.page.getByText("Текущий", { exact: true }).waitFor();
    const local = await fixture({ mode: "local" });
    await local.page.goto(server.url + "/login");
    await local.page
      .getByRole("link", { name: "Забыли пароль?", exact: true })
      .waitFor();
    assert(
      (await local.page
        .getByRole("link", { name: "Создать аккаунт", exact: true })
        .count()) > 0,
    );
    await local.page
      .getByLabel("Email", { exact: true })
      .fill("local@example.test");
    await local.page.locator("#password").fill("SyntheticLocalPassword");
    await local.page
      .getByRole("button", { name: "Войти", exact: true })
      .click();
    await local.page.getByRole("alert").waitFor();
    assert.equal(local.state.requests[0].endpoint, "/auth/login");
    const pending = await fixture({ holdPolicy: true });
    await pending.page.goto(server.url + "/login");
    await pending.state.providerSeen;
    await pending.page
      .getByRole("heading", { name: "Вход в MeetSpace" })
      .waitFor();
    await noLocalActions(pending.page);
    assert.equal(
      await pending.page
        .getByRole("button", { name: "Войти", exact: true })
        .isDisabled(),
      true,
    );
    await pending.state.release();
    await pending.page.getByLabel("Email", { exact: true }).fill(user.email);
    const failed = await fixture({ providerFailure: true });
    await failed.page.goto(server.url + "/login");
    await failed.page
      .getByText("Не удалось загрузить настройки входа.", { exact: true })
      .waitFor();
    await noLocalActions(failed.page);
    assert.equal(
      await failed.page
        .getByRole("button", { name: "Войти", exact: true })
        .isDisabled(),
      true,
    );
    failed.state.providerFailure = false;
    await failed.page
      .getByRole("button", { name: "Повторить", exact: true })
      .click();
    await failed.page.getByLabel("Email", { exact: true }).fill(user.email);
    assert.deepEqual(failed.state.requests, []);
    assert.deepEqual(errors, []);
    console.log(
      "Unified login browser: LDAP/local modes, unified email payload, protected routes, managed add-account, pending/error fail-closed and retry passed.",
    );
  } finally {
    await browser.close();
    await server.close();
  }
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
