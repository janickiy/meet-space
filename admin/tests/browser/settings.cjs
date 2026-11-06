const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const { serve, fixture, chromium } = require("./harness.cjs");
const directory = process.argv[2] || "dist";
const MiB = 1024 * 1024;
const item = (key, label, type, value, group, extra = {}) => ({
  key,
  label,
  type,
  value,
  group,
  editable: true,
  secret: false,
  configured: false,
  description: `Настройка: ${label.toLowerCase()}.`,
  ...extra,
});

(async () => {
  const server = await serve(directory);
  const browser = await chromium.launch({ channel: "chrome", headless: true });
  const report = {
    kind: "isolated-settings-browser-fixture",
    tests: {},
    javascriptErrors: [],
    completed: false,
  };
  const responseSecret = "synthetic-response-value-not-for-display";
  const replacement = "synthetic-new-smtp-value";
  const state = {
    revision: 7,
    items: [
      item(
        "attachmentMaxBytes",
        "Размер вложения",
        "number",
        20 * MiB,
        "limits",
        { min: 1024, max: 100 * MiB, step: 1 },
      ),
      item("mediaMaxPeers", "Участники конференции", "number", 20, "limits", {
        min: 1,
        max: 100,
        step: 1,
      }),
      item("mediaMaxRooms", "Количество комнат", "number", 100, "limits", {
        min: 1,
        max: 10000,
        step: 1,
      }),
      item(
        "recordingRetentionDays",
        "Срок хранения записей",
        "number",
        7,
        "storage",
        { min: 1, max: 3650, step: 1 },
      ),
      item("smtpEnabled", "Отправка почты", "boolean", false, "mail"),
      item(
        "emailVerificationRequired",
        "Подтверждение email",
        "boolean",
        false,
        "mail",
      ),
      item("smtpHost", "SMTP-сервер", "string", "", "mail"),
      item("smtpPort", "Порт SMTP", "number", 465, "mail", {
        min: 1,
        max: 65535,
        step: 1,
      }),
      item("smtpUsername", "Логин SMTP", "string", "", "mail"),
      item("smtpPassword", "Пароль SMTP", "string", responseSecret, "mail", {
        secret: true,
        configured: true,
      }),
      item("smtpFrom", "Отправитель", "string", "", "mail"),
      item("smtpTLSMode", "Шифрование", "string", "tls", "mail", {
        options: ["tls", "starttls"],
      }),
      item(
        "publicURL",
        "Адрес платформы",
        "string",
        "https://example.test",
        "general",
        { editable: false },
      ),
    ],
  };
  let calls = [],
    failNext = false,
    holdNext = false,
    release,
    held;
  const heldPromise = new Promise((resolve) => {
    held = resolve;
  });
  const gate = new Promise((resolve) => {
    release = resolve;
  });
  const intercept = async ({ endpoint, request, fulfill }) => {
    if (endpoint !== "/settings") return false;
    if (request.method() === "GET") {
      await fulfill(state);
      return true;
    }
    assert.equal(request.method(), "PATCH");
    assert(request.headers()["x-csrf-token"]);
    const body = request.postDataJSON();
    calls.push(body);
    if (holdNext) {
      held();
      await gate;
      try {
        await fulfill(state);
      } catch {
        /* Navigating away cancels the request. */
      }
      return true;
    }
    if (failNext) {
      failNext = false;
      await fulfill({ message: "Тестовый отказ сохранения" }, 503);
      return true;
    }
    if (body.revision !== state.revision) {
      await fulfill({ message: "Настройки изменились" }, 409);
      return true;
    }
    for (const [key, value] of Object.entries(body.changes)) {
      const setting = state.items.find((item) => item.key === key);
      assert(setting?.editable);
      if (setting.secret) {
        setting.value = null;
        setting.configured = true;
      } else setting.value = value;
    }
    state.revision++;
    await fulfill(state);
    return true;
  };
  try {
    const page = await browser.newPage({
      viewport: { width: 1440, height: 1000 },
    });
    await page.clock.install();
    page.on("pageerror", (error) => report.javascriptErrors.push(error.name));
    await fixture(page, { intercept });
    await page.goto(server.url + "/settings");
    await page.getByLabel("Размер вложения", { exact: true }).waitFor();
    assert.equal(
      await page.getByLabel("Размер вложения", { exact: true }).inputValue(),
      "20",
    );
    const save = () =>
      page.getByRole("button", { name: "Сохранить изменения", exact: true });
    const saved = () =>
      page.getByText("Изменения сохранены.", { exact: true }).waitFor();
    await page.getByLabel("Размер вложения", { exact: true }).fill("32");
    await page.getByLabel("Участники конференции", { exact: true }).fill("150");
    await save().click();
    await page
      .getByText("Максимальное значение: 100.", { exact: true })
      .waitFor();
    assert.equal(calls.length, 0);
    await page.getByLabel("Участники конференции", { exact: true }).fill("50");
    await page.getByRole("tab", { name: /^Хранилище/ }).click();
    await page.getByLabel("Срок хранения записей", { exact: true }).fill("14");
    await page.getByRole("tab", { name: /^Лимиты/ }).click();
    await save().click();
    await saved();
    assert.deepEqual(calls[0], {
      revision: 7,
      changes: { attachmentMaxBytes: 32 * MiB, mediaMaxPeers: 50 },
    });
    await page.getByRole("tab", { name: /^Хранилище/ }).click();
    assert.equal(
      await page
        .getByLabel("Срок хранения записей", { exact: true })
        .inputValue(),
      "14",
    );
    await save().click();
    await saved();
    assert.deepEqual(calls[1], {
      revision: 8,
      changes: { recordingRetentionDays: 14 },
    });
    report.tests.validationMiBAndIndependentGroups = true;

    await page.getByLabel("Срок хранения записей", { exact: true }).fill("15");
    failNext = true;
    await save().click();
    await page
      .getByText("Тестовый отказ сохранения", { exact: true })
      .waitFor();
    assert.equal(
      await page
        .getByLabel("Срок хранения записей", { exact: true })
        .inputValue(),
      "15",
    );
    assert.equal(
      await page.getByText("Изменения сохранены.", { exact: true }).count(),
      0,
    );
    assert.equal(await save().isEnabled(), true);
    await save().click();
    await saved();
    report.tests.failurePreservesDraftWithoutSuccess = true;

    await page.getByLabel("Срок хранения записей", { exact: true }).fill("16");
    state.revision++;
    state.items.find((item) => item.key === "recordingRetentionDays").value =
      30;
    await save().click();
    await page
      .getByRole("button", {
        name: "Обновить и проверить изменения",
        exact: true,
      })
      .waitFor();
    assert.equal(await save().isDisabled(), true);
    await page
      .getByRole("button", {
        name: "Обновить и проверить изменения",
        exact: true,
      })
      .click();
    await page.getByText(/Актуальные настройки загружены/).waitFor();
    assert.equal(
      await page
        .getByLabel("Срок хранения записей", { exact: true })
        .inputValue(),
      "16",
    );
    await page.getByText("Сохранено: 30", { exact: true }).waitFor();
    const refreshedRevision = state.revision;
    await save().click();
    await saved();
    assert.equal(calls.at(-1).revision, refreshedRevision);
    assert.deepEqual(calls.at(-1).changes, { recordingRetentionDays: 16 });
    report.tests.conflictReloadPreservesDraftAndUsesLatestRevision = true;

    await page.getByRole("tab", { name: /^Почта/ }).click();
    assert.equal(
      await page.getByLabel("Пароль SMTP", { exact: true }).inputValue(),
      "",
    );
    assert(!(await page.locator("body").innerText()).includes(responseSecret));
    await page
      .getByRole("switch", { name: "Подтверждение email", exact: true })
      .click();
    const countBeforeMail = calls.length;
    await save().click();
    await page
      .getByText("Для подтверждения email включите и настройте почту.", {
        exact: true,
      })
      .waitFor();
    assert.equal(calls.length, countBeforeMail);
    await page
      .getByRole("switch", { name: "Отправка почты", exact: true })
      .click();
    await page
      .getByLabel("SMTP-сервер", { exact: true })
      .fill("smtp.example.test");
    await page.getByLabel("Порт SMTP", { exact: true }).fill("587");
    await page.getByLabel("Логин SMTP", { exact: true }).fill("mailer");
    await page
      .getByLabel("Отправитель", { exact: true })
      .fill("MeetSpace <mail@example.test>");
    await page
      .getByLabel("Шифрование", { exact: true })
      .selectOption("starttls");
    await save().click();
    await saved();
    assert.equal(calls.at(-1).changes.smtpEnabled, true);
    assert.equal(calls.at(-1).changes.emailVerificationRequired, true);
    assert(!("smtpPassword" in calls.at(-1).changes));
    await page.getByLabel("Пароль SMTP", { exact: true }).fill(replacement);
    await save().click();
    await saved();
    assert.deepEqual(Object.keys(calls.at(-1).changes), ["smtpPassword"]);
    assert.equal(
      await page.getByLabel("Пароль SMTP", { exact: true }).inputValue(),
      "",
    );
    const storage = await page.evaluate(() =>
      JSON.stringify([
        Object.entries(localStorage),
        Object.entries(sessionStorage),
      ]),
    );
    assert(!storage.includes(replacement) && !storage.includes(responseSecret));
    report.tests.mailDependenciesAndWriteOnlyPassword = true;

    await page
      .getByLabel("SMTP-сервер", { exact: true })
      .fill("unsaved.example.test");
    await page
      .getByRole("button", { name: "Отменить изменения", exact: true })
      .click();
    assert.equal(
      await page.getByLabel("SMTP-сервер", { exact: true }).inputValue(),
      "smtp.example.test",
    );
    assert.equal(await save().isDisabled(), true);
    await fs.mkdir("test-results", { recursive: true });
    await page.clock.fastForward(6000);
    await page.evaluate(() => window.scrollTo(0, 0));
    await page.screenshot({
      path: "test-results/settings-desktop.png",
      fullPage: true,
    });
    report.tests.resetGroup = true;

    await page.clock.fastForward(6000);
    holdNext = true;
    await page.getByLabel("Пароль SMTP", { exact: true }).fill(replacement);
    await save().click();
    await heldPromise;
    await page.getByRole("link", { name: "Пользователи", exact: true }).click();
    await page
      .getByRole("heading", { name: "Пользователи", level: 1, exact: true })
      .waitFor();
    release();
    await page.waitForTimeout(100);
    assert.equal(await page.locator(".toast").count(), 0);
    await page.getByRole("link", { name: "Настройки", exact: true }).click();
    await page.getByRole("tab", { name: /^Почта/ }).click();
    assert.equal(
      await page.getByLabel("Пароль SMTP", { exact: true }).inputValue(),
      "",
    );
    report.tests.unmountCancelsSaveAndClearsSecretDraft = true;
    await page.close();

    const mobile = await browser.newPage({
      viewport: { width: 390, height: 844 },
      isMobile: true,
      hasTouch: true,
    });
    await fixture(mobile, { intercept });
    mobile.on("pageerror", (error) => report.javascriptErrors.push(error.name));
    await mobile.goto(server.url + "/settings");
    await mobile.getByRole("tab", { name: /^Почта/ }).click();
    assert.equal(
      await mobile.evaluate(() => document.documentElement.scrollWidth),
      390,
    );
    await mobile
      .getByRole("button", { name: "Светлая тема", exact: true })
      .click();
    await mobile.screenshot({
      path: "test-results/settings-mobile-light.png",
      fullPage: true,
    });
    report.tests.mobileAndLightTheme = true;
    await mobile.close();
    assert.deepEqual(report.javascriptErrors, []);
    report.completed = true;
    process.stdout.write(JSON.stringify(report) + "\n");
  } finally {
    release();
    await browser.close();
    await server.close();
  }
})();
