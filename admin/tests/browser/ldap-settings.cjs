const assert = require("node:assert/strict");
const path = require("node:path");
const { chromium, serve, fixture } = require("./harness.cjs");
const item = (key, type, value, extra = {}) => ({
  key,
  label: key,
  type,
  value,
  group: "ldap",
  editable: true,
  ...extra,
});

(async () => {
  const server = await serve(path.join(__dirname, "../../dist"));
  const browser = await chromium.launch({ channel: "chrome", headless: true });
  const state = {
    revision: 7,
    items: [
      item("ldapEnabled", "boolean", true),
      item("ldapMode", "string", "ad", { options: ["ad", "generic"] }),
      item("ldapServerURL", "string", "ldaps://directory.example.test:636"),
      item("ldapBaseDN", "string", "dc=example,dc=test"),
      item("ldapUsersDN", "string", "ou=people,dc=example,dc=test"),
      item("ldapBindDN", "string", "uid=reader,dc=example,dc=test"),
      item(
        "ldapBindPassword",
        "string",
        "synthetic-response-must-be-scrubbed",
        { secret: true, configured: true },
      ),
      item("ldapLoginAttribute", "string", "sAMAccountName"),
      item("ldapIDAttribute", "string", "objectGUID"),
      item("ldapEmailAttribute", "string", "mail"),
      item("ldapNameAttribute", "string", "displayName"),
      item("ldapGroupMembershipAttribute", "string", "memberOf"),
      item("ldapNestedGroups", "boolean", false),
      item(
        "ldapUserFilter",
        "string",
        "(&(objectClass=user)(!(objectClass=computer)))",
      ),
      item("ldapCA", "string", ""),
      {
        ...item("publicURL", "string", "https://example.test"),
        group: "general",
        editable: false,
      },
    ],
  };
  const calls = [];
  let statusReads = 0;
  let queued = false;
  const page = await browser.newPage({
    viewport: { width: 1440, height: 1100 },
  });
  const errors = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.clock.install();
  await fixture(page, {
    intercept: async ({ endpoint, request, fulfill }) => {
      if (endpoint === "/settings") {
        if (request.method() === "PATCH") {
          assert(request.headers()["x-csrf-token"]);
          const body = request.postDataJSON();
          calls.push({ endpoint, body });
          assert.equal(body.revision, state.revision);
          for (const [key, value] of Object.entries(body.changes))
            state.items.find((item) => item.key === key).value = value;
          state.revision++;
        }
        await fulfill(state);
        return true;
      }
      if (endpoint === "/directory/status") {
        statusReads++;
        await fulfill({
          status: queued ? "queued" : "succeeded",
          queued,
          imported: 1,
          updated: 2,
          disabled: 0,
          total: 3,
        });
        return true;
      }
      if (["/directory/test", "/directory/sync"].includes(endpoint)) {
        assert(request.headers()["x-csrf-token"]);
        const body = request.postDataJSON();
        assert.deepEqual(body, { revision: state.revision });
        calls.push({ endpoint, body });
        if (endpoint === "/directory/sync") queued = true;
        await fulfill(
          endpoint === "/directory/test"
            ? {
                reachable: true,
                matchedUsers: 3,
                message: "Подключение установлено.",
              }
            : { status: "queued" },
        );
        return true;
      }
      return false;
    },
  });
  try {
    await page.goto(server.url + "/settings");
    await page.getByRole("tab", { name: /^LDAP/ }).click();
    const testButton = page.getByRole("button", {
      name: "Проверить подключение",
      exact: true,
    });
    await testButton.waitFor();
    assert.equal(
      await page.getByLabel("ldapBindPassword", { exact: true }).inputValue(),
      "",
    );
    assert.equal(
      await page
        .getByLabel("ldapCA", { exact: true })
        .evaluate((el) => el.tagName),
      "TEXTAREA",
    );
    assert.equal(
      (await page.locator("body").innerText()).includes(
        "synthetic-response-must-be-scrubbed",
      ),
      false,
    );
    await page.getByLabel("ldapMode", { exact: true }).selectOption("generic");
    assert.equal(
      await page.getByLabel("ldapLoginAttribute", { exact: true }).inputValue(),
      "uid",
    );
    assert.equal(
      await page.getByLabel("ldapIDAttribute", { exact: true }).inputValue(),
      "entryUUID",
    );
    assert.equal(await testButton.isDisabled(), true);
    await page
      .getByRole("button", { name: "Сохранить изменения", exact: true })
      .click();
    await page.getByText("Изменения сохранены.", { exact: true }).waitFor();
    assert.equal(calls[0].body.changes.ldapMode, "generic");
    assert.equal(calls[0].body.changes.ldapBindPassword, undefined);
    const modeField = page.getByLabel("ldapEnabled", { exact: true });
    assert.deepEqual(await modeField.locator("option").allTextContents(), [
      "Локальные аккаунты",
      "Только LDAP",
    ]);
    assert.equal(await modeField.inputValue(), "true");
    await modeField.selectOption("false");
    await page.getByText("Сохранено: Только LDAP", { exact: true }).waitFor();
    await page
      .getByRole("button", { name: "Сохранить изменения", exact: true })
      .click();
    await page.waitForFunction(() =>
      document.body.textContent.includes("Все изменения сохранены"),
    );
    assert.equal(calls.at(-1).body.changes.ldapEnabled, false);
    await modeField.selectOption("true");
    await page
      .getByText("Сохранено: Локальные аккаунты", { exact: true })
      .waitFor();
    await page
      .getByRole("button", { name: "Сохранить изменения", exact: true })
      .click();
    await page.waitForFunction(() =>
      document.body.textContent.includes("Все изменения сохранены"),
    );
    assert.equal(calls.at(-1).body.changes.ldapEnabled, true);
    await testButton.click();
    await page.getByText(/Проверено записей: 3/).waitFor();
    await page
      .getByRole("button", { name: "Синхронизировать сейчас", exact: true })
      .click();
    await page
      .getByRole("button", { name: "Синхронизация…", exact: true })
      .waitFor();
    await page.getByText("В очереди", { exact: true }).waitFor();
    const reads = statusReads;
    const refreshed = page.waitForResponse((response) =>
      response.url().endsWith("/directory/status"),
    );
    await page.clock.runFor(2100);
    await refreshed;
    assert(statusReads > reads, "queued work must refresh status");
    await page.getByRole("tab", { name: /^Основные/ }).click();
    const hiddenReads = statusReads;
    await page.clock.runFor(6000);
    assert.equal(statusReads, hiddenReads, "closed LDAP tab must not poll");
    assert.deepEqual(errors, []);
    console.log(
      "LDAP admin browser: write-only secret, presets, save-before-actions, CSRF, queued polling and cleanup passed.",
    );
  } finally {
    await browser.close();
    await server.close();
  }
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
