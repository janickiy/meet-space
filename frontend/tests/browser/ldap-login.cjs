const assert = require("node:assert/strict");
const path = require("node:path");
const { chromium, serve } = require("../../../admin/tests/browser/harness.cjs");

(async () => {
  const server = await serve(path.join(__dirname, "../../dist"));
  const browser = await chromium.launch({ channel: "chrome", headless: true });
  const errors = [];
  const requests = [];
  const user = {
    id: "00000000-0000-4000-8000-000000000001",
    email: "directory@example.test",
    displayName: "Directory User",
    authSource: "ldap",
    createdAt: "2026-11-07T10:00:00Z",
    updatedAt: "2026-11-07T10:00:00Z",
  };
  async function fixture(providerFailure = false, authenticated = false) {
    const page = await browser.newPage({
      viewport: { width: 1440, height: 1100 },
    });
    page.on("pageerror", (error) => errors.push(error.message));
    await page.route("**/api/v1/**", async (route) => {
      const request = route.request();
      const endpoint = new URL(request.url()).pathname.replace("/api/v1", "");
      const send = (data, status = 200) =>
        route.fulfill({
          status,
          contentType: "application/json",
          body: JSON.stringify(data),
        });
      if (endpoint === "/auth/providers")
        return send(
          providerFailure
            ? { message: "Unavailable" }
            : { ldap: { enabled: true, label: "Корпоративный вход" } },
          providerFailure ? 503 : 200,
        );
      if (["/auth/login", "/auth/ldap/login"].includes(endpoint)) {
        requests.push({ endpoint, body: request.postDataJSON() });
        return send({ message: "invalid credentials" }, 401);
      }
      if (endpoint === "/auth/refresh")
        return authenticated
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
    return page;
  }
  try {
    const page = await fixture();
    await page.goto(server.url + "/login");
    await page
      .getByRole("button", { name: "Корпоративный вход", exact: true })
      .click();
    assert.equal(
      await page
        .getByRole("button", { name: "Забыли пароль?", exact: true })
        .count(),
      0,
    );
    await page.getByLabel("Корпоративный логин", { exact: true }).fill("alice");
    await page.locator("#password").fill("SyntheticDirectoryPassword42");
    await page.getByRole("button", { name: "Войти", exact: true }).click();
    await page.getByRole("alert").waitFor();
    assert.deepEqual(requests, [
      {
        endpoint: "/auth/ldap/login",
        body: { login: "alice", password: "SyntheticDirectoryPassword42" },
      },
    ]);
    await page.getByRole("button", { name: "По email", exact: true }).click();
    assert.equal(await page.locator("#password").inputValue(), "");
    await page.getByLabel("Email", { exact: true }).fill("local@example.test");
    await page.locator("#password").fill("SyntheticLocalPassword42");
    await page.getByRole("button", { name: "Войти", exact: true }).click();
    await page.getByRole("alert").waitFor();
    assert.equal(requests.at(-1).endpoint, "/auth/login");
    const managed = await fixture(false, true);
    await managed.goto(server.url + "/app/settings");
    const name = managed.getByLabel("Имя для встреч", { exact: true });
    await name.waitFor();
    assert.equal(await name.getAttribute("readonly"), "");
    assert.equal(
      await managed
        .getByRole("button", { name: "Сохранить имя", exact: true })
        .count(),
      0,
    );
    await managed.getByRole("tab", { name: "Аккаунты", exact: true }).click();
    assert.equal(
      await managed
        .getByRole("button", { name: "Удалить текущий аккаунт", exact: true })
        .count(),
      0,
    );
    await managed
      .getByRole("button", { name: "Добавить аккаунт", exact: true })
      .click();
    await managed
      .getByRole("button", { name: "Корпоративный вход", exact: true })
      .click();
    await managed
      .getByLabel("Корпоративный логин", { exact: true })
      .fill("bob");
    await managed
      .getByLabel("Пароль", { exact: true })
      .fill("SyntheticSecondAccountPassword");
    await managed
      .getByRole("button", { name: "Войти и добавить", exact: true })
      .click();
    await managed.locator(".account-management [role=alert]").waitFor();
    assert.equal(requests.at(-1).endpoint, "/auth/ldap/login");
    await managed.getByText("Текущий", { exact: true }).waitFor();
    const fallback = await fixture(true);
    await fallback.goto(server.url + "/login");
    await fallback
      .getByLabel("Email", { exact: true })
      .fill("local@example.test");
    await fallback.locator("#password").fill("SyntheticLocalPassword42");
    await fallback.getByRole("button", { name: "Войти", exact: true }).click();
    await fallback.getByRole("alert").waitFor();
    assert.equal(requests.at(-1).endpoint, "/auth/login");
    assert.deepEqual(errors, []);
    console.log(
      "LDAP login browser: corporate login, no local fallback, local recovery and provider-discovery outage passed.",
    );
  } finally {
    await browser.close();
    await server.close();
  }
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
