import { createRequire } from "node:module";
import { mkdir, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import assert from "node:assert/strict";

// Run from any directory: node design/meetspace-android-20261010/verify.mjs
// Start the static server separately: python3 design/meetspace-android-20261010/serve.py
// The test uses only local prototype pages and demonstration fixture data.
const require = createRequire(
  new URL("../../frontend/package.json", import.meta.url),
);
const { chromium } = require("playwright");
const base = process.env.MEETSPACE_QA_URL || "http://127.0.0.1:8766";
const outDir = fileURLToPath(new URL(".", import.meta.url));
const screens = [
  "meetings",
  "messages",
  "thread",
  "calendar",
  "recordings",
  "recording",
  "prejoin",
  "call",
  "settings",
  "welcome",
  "connect",
  "login",
  "deploy",
  "fingerprint",
  "preflight",
  "install",
  "complete",
  "install-error",
];
const viewports = [
  { width: 360, height: 800 },
  { width: 390, height: 844 },
  { width: 834, height: 1112 },
  { width: 1194, height: 834 },
  { width: 320, height: 700 },
  { width: 600, height: 900 },
];
const themes = ["light", "dark"];
const report = {
  generatedAt: new Date().toISOString(),
  base,
  matrix: [],
  flows: [],
  screenshots: [],
  issues: [],
  relatedReport: "regression-report.json",
};
const browser = await chromium.launch({ headless: true });
await mkdir(new URL("previews/", import.meta.url), { recursive: true });
const url = (screen, theme = "light") =>
  `${base}/preview.html?screen=${screen}&theme=${theme}`;
const ready = async (page) => {
  await page.locator("#app > *").waitFor();
  await page.evaluate(() => document.fonts.ready);
};
const stateIs = async (page, screen) =>
  assert.equal(await page.locator("body").getAttribute("data-screen"), screen);

async function matrix(viewport) {
  const page = await browser.newPage({ viewport });
  let errors = [],
    missing = [];
  page.on("pageerror", (error) => errors.push(error.message));
  page.on("response", (response) => {
    if (response.status() >= 400)
      missing.push({ url: response.url(), status: response.status() });
  });
  page.on("requestfailed", (request) =>
    missing.push({ url: request.url(), failure: request.failure()?.errorText }),
  );
  for (const theme of themes)
    for (const screen of screens) {
      errors = [];
      missing = [];
      await page.goto(url(screen, theme), { waitUntil: "load" });
      await ready(page);
      const measure = await page.evaluate(() => ({
        screen: document.body.dataset.screen,
        theme: document.documentElement.dataset.theme,
        width: innerWidth,
        bodyWidth: document.body.scrollWidth,
        documentWidth: document.documentElement.scrollWidth,
        horizontalOverflow:
          Math.max(
            document.body.scrollWidth,
            document.documentElement.scrollWidth,
          ) >
          innerWidth + 1,
        brokenImages: [...document.images]
          .filter((img) => !img.complete || !img.naturalWidth)
          .map((img) => img.getAttribute("src")),
        empty: document.querySelector("#app").innerText.trim().length < 20,
        navigationValid:
          !document.querySelector(".tm-shell") ||
          (document.querySelectorAll(".bottom-nav button").length === 3 &&
            [...document.querySelectorAll(".bottom-nav button")]
              .map((b) => b.getAttribute("aria-label"))
              .join(",") === "Встречи,Чаты,Профиль"),
      }));
      const entry = {
        screen,
        theme,
        viewport,
        ...measure,
        errors: [...errors],
        missing: [...missing],
      };
      entry.pass =
        !entry.horizontalOverflow &&
        !entry.empty &&
        entry.navigationValid &&
        !errors.length &&
        !missing.length &&
        !measure.brokenImages.length &&
        measure.screen === screen &&
        measure.theme === theme;
      report.matrix.push(entry);
      if (!entry.pass) {
        report.issues.push(entry);
        console.error("MATRIX FAIL", JSON.stringify(entry));
      }
    }
  await page.close();
}

async function flow(name, viewport, steps) {
  const page = await browser.newPage({ viewport });
  page.setDefaultTimeout(10000);
  const errors = [];
  page.on("pageerror", (error) => errors.push(error.message));
  try {
    await steps(page);
    assert.deepEqual(errors, []);
    report.flows.push({ name, viewport, pass: true });
    console.log(`Flow passed: ${name} ${viewport.width}`);
  } catch (error) {
    const issue = { name, viewport, pass: false, error: error.message, errors };
    report.flows.push(issue);
    report.issues.push(issue);
    console.error("FLOW FAIL", name, error.message);
    await page.screenshot({
      path: `${outDir}previews/failure-${name}-${viewport.width}.png`,
      fullPage: true,
    });
  }
  await page.close();
}

try {
  for (let offset = 0; offset < viewports.length; offset += 2)
    await Promise.all(viewports.slice(offset, offset + 2).map(matrix));
  console.log(
    `Matrix: ${report.matrix.filter((x) => x.pass).length}/${report.matrix.length} passed`,
  );
  for (const viewport of [viewports[1], viewports[3]]) {
    await flow("three-tabs-profile", viewport, async (page) => {
      await page.goto(url("meetings"));
      assert.deepEqual(
        await page
          .locator(".bottom-nav button")
          .evaluateAll((items) =>
            items.map((item) => item.getAttribute("aria-label")),
          ),
        ["Встречи", "Чаты", "Профиль"],
      );
      await page.locator('.bottom-nav [data-go="messages"]').click();
      await page.locator('[data-filter="personal"]').click();
      assert.equal(await page.locator(".conversation:visible").count(), 2);
      await page.locator('[data-filter="unread"]').click();
      assert.equal(await page.locator(".conversation:visible").count(), 2);
      await page
        .getByRole("button", { name: "Скрыть приглашение", exact: true })
        .click();
      assert.equal(await page.locator(".chat-invite-note").count(), 0);
      await page.locator('.bottom-nav [data-go="settings"]').click();
      await stateIs(page, "settings");
      const openAccountSection = async (section) => {
        if (viewport.width < 780 && await page.locator('.account-back').isVisible())
          await page.locator('.account-back').click();
        await page.locator(`[data-action="accountSection"][data-section="${section}"]`).click();
      };
      await openAccountSection("profile");
      assert(await page.locator('#account-save-name').isDisabled());
      await page.locator('#account-name').fill("   ");
      assert(await page.locator('#account-save-name').isDisabled());
      await page.locator('#account-name').fill("Александр Тест");
      await page.locator('#account-save-name').click();
      assert.equal(await page.locator('#account-name').inputValue(), "Александр Тест");
      assert.equal(await page.locator('input[type="email"]').getAttribute('readonly'), "");
      assert.equal(await page.locator('.nav-profile-avatar').innerText(), "АТ");
      await openAccountSection("audio");
      await page.getByRole('switch', {name:'Подключаться с выключенным микрофоном', exact:true}).check();
      await page.getByRole('combobox', {name:'Микрофон', exact:true}).selectOption('headset');
      await openAccountSection("notifications");
      await page.getByLabel('Напоминания о встречах', {exact:true}).uncheck();
      await page.locator('#account-save-notifications').click();
      assert(await page.locator('#account-save-notifications').isDisabled());
      await openAccountSection("audio");
      assert.equal(await page.getByRole('combobox', {name:'Микрофон', exact:true}).inputValue(), 'headset');
      assert(await page.getByRole('switch', {name:'Подключаться с выключенным микрофоном', exact:true}).isChecked());
      await openAccountSection("notifications");
      assert(!(await page.getByLabel('Напоминания о встречах', {exact:true}).isChecked()));
      await openAccountSection("appearance");
      await page.getByRole('radio', {name:'Тёмная тема', exact:true}).check();
      assert.equal(await page.locator('html').getAttribute('data-theme'), 'dark');
      await page.getByRole('combobox', {name:'Размер текста', exact:true}).selectOption('200');
      assert.equal(await page.locator('.account-layout').evaluate(el => getComputedStyle(el).fontSize), '28px');
      await page.locator('.bottom-nav [data-go="meetings"]').click();
      await stateIs(page, "meetings");
    });
    await flow("meeting-filters", viewport, async (page) => {
      await page.goto(url("meetings"));
      const rows = page.locator(".cf-entry");
      assert.equal(await rows.count(), 4);
      await page
        .getByLabel("Найти встречу", { exact: true })
        .fill("  ДИЗАЙН  ");
      assert.equal(await rows.count(), 1);
      await page
        .getByLabel("Найти встречу", { exact: true })
        .fill("нет такой встречи");
      assert(
        await page
          .getByRole("heading", { name: "Встречи не найдены", exact: true })
          .isVisible(),
      );
      await page
        .getByRole("button", { name: "Очистить поиск", exact: true })
        .click();
      const role =
        viewport.width < 780
          ? async () => {
              await page
                .getByRole("button", { name: "Фильтры встреч", exact: true })
                .click();
              return page.getByRole("dialog");
            }
          : async () => page.locator("#desktop-meeting-filters");
      let filters = await role();
      await filters
        .getByLabel("Моё участие", { exact: true })
        .selectOption("participating");
      if (viewport.width < 780)
        await filters
          .getByRole("button", { name: "Применить", exact: true })
          .click();
      assert.equal(await rows.count(), 1);
      filters = await role();
      await filters
        .getByLabel("Моё участие", { exact: true })
        .selectOption("owned");
      await filters
        .getByLabel("Встречи с даты", { exact: true })
        .fill("2026-10-13");
      if (viewport.width < 780)
        await filters
          .getByRole("button", { name: "Применить", exact: true })
          .click();
      else
        await filters
          .getByLabel("Встречи с даты", { exact: true })
          .press("Tab");
      assert.equal(await rows.count(), 1);
      assert.match(await rows.innerText(), /Планы на неделю/);
      await page
        .getByRole("button", { name: "Сбросить фильтры", exact: true })
        .filter({ visible: true })
        .click();
      await page.getByRole("tab", { name: "Активные", exact: true }).click();
      assert.equal(await rows.count(), 1);
      await page
        .getByRole("button", { name: "Подключиться", exact: true })
        .click();
      assert.match(
        await page.locator(".prejoin-info").innerText(),
        /Максим Петров/,
      );
      await page
        .getByRole("button", { name: "Вернуться к встречам", exact: true })
        .click();
      await page.getByRole("tab", { name: "Завершённые", exact: true }).click();
      assert.equal(await rows.count(), 3);
      await page
        .getByRole("button", {
          name: "Подробнее о встрече Продукт. Следующая глава",
          exact: true,
        })
        .click();
      await page
        .getByRole("button", { name: "Смотреть запись", exact: true })
        .click();
      await stateIs(page, "recording");
    });
    await flow("meeting-planning", viewport, async (page) => {
      await page.goto(url("meetings"));
      await page.locator('[data-action="schedule"]').click();
      await page
        .getByLabel("Название встречи", { exact: true })
        .fill("Планирование <проекта>");
      await page.getByLabel("Дата", { exact: true }).fill("2026-11-03");
      await page.getByLabel("Время", { exact: true }).fill("14:30");
      await page
        .getByRole("combobox", { name: "Длительность", exact: true })
        .selectOption("45");
      await page.locator('input[name="waitingRoom"]').check();
      await page
        .getByRole("dialog")
        .getByRole("button", { name: "Запланировать", exact: true })
        .click();
      assert.match(
        await page.getByRole("dialog").innerText(),
        /3 ноября · 14:30 · 45 мин/,
      );
      await page
        .getByRole("button", { name: "К списку встреч", exact: true })
        .click();
      assert.equal(await page.locator(".cf-entry").count(), 5);
      assert.match(
        await page.locator(".cf-entry").first().innerText(),
        /Планирование <проекта>/,
      );
      await page.locator('[data-action="newMeeting"]').click();
      await page
        .getByLabel("Название встречи", { exact: true })
        .fill("Комната без даты");
      await page
        .getByRole("button", { name: "Создать встречу", exact: true })
        .click();
      await page
        .getByRole("button", { name: "Открыть встречу", exact: true })
        .click();
      assert.match(await page.locator(".prejoin-info").innerText(), /Без даты/);
      assert.match(
        await page.locator(".prejoin-info").innerText(),
        /Организатор: Вы/,
      );
      await page
        .getByRole("button", { name: "Вернуться к встречам", exact: true })
        .click();
      assert.equal(await page.locator(".cf-entry").count(), 6);
    });
    await flow("meeting", viewport, async (page) => {
      await page.goto(url("meetings"));
      await ready(page);
      await page.locator('[data-action="newMeeting"]').click();
      await page
        .locator('#meeting-form input[name="title"]')
        .fill("Обсуждение главной");
      await page
        .getByRole("button", { name: "Создать встречу", exact: true })
        .click();
      await page
        .getByRole("button", { name: "Открыть встречу", exact: true })
        .click();
      await stateIs(page, "prejoin");
      await page.getByLabel("Выключить микрофон", { exact: true }).click();
      assert.equal(
        await page
          .getByLabel("Включить микрофон", { exact: true })
          .getAttribute("aria-pressed"),
        "false",
      );
      await page
        .getByRole("button", { name: "Присоединиться", exact: false })
        .click();
      await stateIs(page, "call");
      await page.getByRole("button", { name: "Выйти", exact: true }).click();
      assert(await page.getByRole("dialog").isVisible());
      await page.getByRole("button", { name: "Остаться", exact: true }).click();
      await stateIs(page, "call");
      await page.getByRole("button", { name: "Выйти", exact: true }).click();
      await page
        .getByRole("button", { name: "Выйти из встречи", exact: true })
        .click();
      await stateIs(page, "meetings");
    });
    await flow("messages", viewport, async (page) => {
      await page.goto(url("messages"));
      await ready(page);
      await page
        .getByRole("button", { name: "Поиск чатов", exact: true })
        .click();
      await page.getByLabel("Найти чат", { exact: true }).fill("Анна");
      assert.equal(await page.locator(".conversation:visible").count(), 1);
      await page
        .getByLabel("Найти чат", { exact: true })
        .fill("Несуществующий чат");
      assert(await page.locator("#chat-empty").isVisible());
      await page.getByLabel("Найти чат", { exact: true }).fill("Анна");
      await page.locator(".conversation:visible").click();
      await stateIs(page, "thread");
      await page
        .getByLabel("Сообщение", { exact: true })
        .fill("Проверка макета — демо");
      await page
        .getByRole("button", { name: "Отправить сообщение", exact: true })
        .click();
      assert(
        await page
          .locator("#thread-log")
          .getByText("Проверка макета — демо", { exact: false })
          .isVisible(),
      );
      assert.equal(
        await page.getByLabel("Сообщение", { exact: true }).inputValue(),
        "",
      );
    });
    await flow("groups-in-all-chats", viewport, async (page) => {
      await page.goto(url("messages", "dark"));
      await ready(page);
      const rows = page.locator(".conversation:visible");
      assert.equal(await rows.count(), 5);
      assert.equal(await rows.locator(".group-avatar-badge").count(), 3);
      assert.match(
        await page.locator('.conversation[data-chat="team"]').innerText(),
        /Группа · 5 участников/,
      );
      assert.match(
        await page.locator('.conversation[data-chat="team"]').innerText(),
        /Мария:/,
      );
      await page.locator('[data-filter="personal"]').click();
      assert.equal(await rows.count(), 2);
      assert.equal(
        await page.locator('.conversation[data-kind="group"]').count(),
        0,
      );
      await page.locator('[data-filter="unread"]').click();
      assert.equal(await rows.count(), 2);
      assert.equal(
        await page.locator('.conversation[data-kind="group"]').count(),
        1,
      );
      await page.locator('[data-filter="all"]').click();
      await page.locator('.conversation[data-chat="dev"]').click();
      assert.match(
        await page.locator(".thread-header").innerText(),
        /Группа · 4 участника/,
      );
      await page
        .getByRole("button", { name: "Информация о чате", exact: true })
        .click();
      assert.equal(await page.locator(".group-info-member").count(), 4);
      assert.match(
        await page.locator(".modal").innerText(),
        /Максим Петров\s+Администратор/,
      );
      await page.getByRole("button", { name: "Закрыть", exact: true }).click();
      const back = async () => {
        if (viewport.width < 700)
          await page
            .getByRole("button", { name: "Назад к чатам", exact: true })
            .click();
        else await page.locator('.bottom-nav [data-go="messages"]').click();
      };
      await back();
      await page.locator('.conversation[data-chat="anna"]').click();
      await page
        .getByRole("button", { name: "Информация о чате", exact: true })
        .click();
      assert.equal(
        await page.locator("#modal-title").innerText(),
        "О контакте",
      );
      assert.equal(await page.locator(".group-info-member").count(), 0);
      await page.getByRole("button", { name: "Закрыть", exact: true }).click();
      await back();
      await page.locator('[data-filter="personal"]').click();
      await page
        .getByRole("button", { name: "Новый чат", exact: true })
        .click();
      assert.equal(
        await page.locator('.modal [data-action="newGroup"]').count(),
        1,
      );
      await page
        .getByRole("button", { name: "Создать группу", exact: true })
        .click();
      await page
        .getByLabel("Название группы", { exact: true })
        .fill("Проект <Альфа>");
      await page.locator('input[name="members"][value="anna"]').uncheck();
      await page.locator('input[name="members"][value="max"]').uncheck();
      await page
        .getByRole("button", { name: "Создать группу", exact: true })
        .click();
      assert(await page.locator("#group-members-error").isVisible());
      await page.locator('input[name="members"][value="anna"]').check();
      await page
        .getByRole("button", { name: "Создать группу", exact: true })
        .click();
      await stateIs(page, "thread");
      assert.equal(
        await page.locator(".thread-header h3").innerText(),
        "Проект <Альфа>",
      );
      assert.match(
        await page.locator(".thread-header").innerText(),
        /Группа · 2 участника/,
      );
      assert(await page.locator(".group-welcome").isVisible());
      assert.equal(await page.locator(".thread-log .chat-message").count(), 0);
      await page
        .getByRole("button", { name: "Информация о чате", exact: true })
        .click();
      assert.equal(await page.locator(".group-info-member").count(), 2);
      assert.match(
        await page.locator(".group-info-member").first().innerText(),
        /Вы · Администратор/,
      );
      await page.getByRole("button", { name: "Закрыть", exact: true }).click();
      await page
        .getByLabel("Сообщение", { exact: true })
        .fill("Привет, команда!");
      await page
        .getByRole("button", { name: "Отправить сообщение", exact: true })
        .click();
      assert.equal(await page.locator(".thread-log .chat-message").count(), 1);
      assert.equal(await page.locator(".group-welcome").count(), 0);
      await back();
      assert.equal(
        await page.locator('[data-filter="all"]').getAttribute("aria-pressed"),
        "true",
      );
      assert.match(await rows.first().innerText(), /Проект <Альфа>/);
      assert.match(await rows.first().innerText(), /Вы: Привет, команда!/);
      assert.equal(await rows.count(), 6);
      await page.locator('[data-filter="personal"]').click();
      assert.equal(await rows.count(), 2);
      assert.equal(
        await page.locator('.conversation[data-kind="group"]').count(),
        0,
      );
      await page.locator('[data-filter="all"]').click();
      await rows.first().click();
      await page
        .getByRole("button", { name: "Информация о чате", exact: true })
        .click();
      assert(await page.getByRole("dialog", { name: "О группе", exact: true }).isVisible());
      assert.match(await page.getByRole("dialog").innerText(), /Проект <Альфа>/);
    });
    await flow("calendar", viewport, async (page) => {
      await page.goto(url("calendar"));
      await ready(page);
      await page.locator('[data-day="13"]').click();
      assert.equal(
        await page.locator('[data-day="13"]').getAttribute("aria-pressed"),
        "true",
      );
      assert.equal(await page.locator(".event").count(), 0);
      await page.getByRole("button", { name: "Сегодня", exact: true }).click();
      assert.equal(await page.locator(".event").count(), 3);
      await page.locator(".event").first().click();
      assert(await page.getByRole("dialog").isVisible());
      await page.getByRole("button", { name: "Закрыть", exact: true }).click();
    });
    await flow("recordings", viewport, async (page) => {
      await page.goto(url("recordings"));
      await ready(page);
      const total = await page.locator(".recording-card:visible").count();
      await page.locator('[data-record-filter="team"]').click();
      assert((await page.locator(".recording-card:visible").count()) < total);
      assert((await page.locator(".recording-card:visible").count()) > 0);
      await page.getByLabel("Найти запись").fill("Несуществующая запись");
      assert(await page.locator("#record-empty").isVisible());
      await page.getByLabel("Найти запись").fill("");
      await page.locator(".recording-card:visible").first().click();
      await stateIs(page, "recording");
      await page
        .getByRole("button", {
          name: "Запустить демо проигрывателя",
          exact: true,
        })
        .click();
      assert(await page.locator("#toast").isVisible());
    });
    await flow("connect-login", viewport, async (page) => {
      await page.goto(url("welcome"));
      await ready(page);
      await page.locator('[data-go="connect"]').click();
      await stateIs(page, "connect");
      await page
        .getByLabel("Адрес сервера", { exact: true })
        .fill("http://invalid.example");
      await page
        .getByRole("button", { name: "Проверить адрес", exact: false })
        .click();
      assert(await page.locator(".on-form-error").isVisible());
      await page
        .getByLabel("Адрес сервера", { exact: true })
        .fill("https://meet.example.org");
      await page
        .getByRole("button", { name: "Проверить адрес", exact: false })
        .click();
      assert(await page.locator("#on-connect-result").isVisible());
      await page
        .getByRole("button", { name: "Перейти ко входу", exact: false })
        .click();
      await stateIs(page, "login");
      await page.locator('input[name="password"]').fill("demo");
      await page
        .getByRole("button", { name: "Войти в MeetSpace", exact: false })
        .click();
      await stateIs(page, "meetings");
    });
    await flow("installer", viewport, async (page) => {
      await page.goto(url("welcome"));
      await ready(page);
      await page.locator('[data-go="deploy"]').click();
      await stateIs(page, "deploy");
      await page.locator('input[name="password"]').fill("demo");
      await page
        .getByRole("button", { name: "Продолжить", exact: false })
        .click();
      await stateIs(page, "fingerprint");
      assert(await page.locator("#on-trust-next").isDisabled());
      await page.locator("#on-trust").check();
      await page.locator("#on-trust-next").click();
      await stateIs(page, "preflight");
      assert(await page.locator("#on-install-start").isDisabled());
      await page.locator("#on-consent").check();
      await page.locator("#on-install-start").click();
      await stateIs(page, "install");
      await page.locator("#on-stage-next").click();
      await page.locator("#on-simulate-error").click();
      await stateIs(page, "install-error");
      await page.locator("#on-retry").click();
      await stateIs(page, "install");
      assert(
        (await page.locator(".on-install-percent").innerText()).includes("30"),
      );
      for (let i = 0; i < 4; i++) await page.locator("#on-stage-next").click();
      await stateIs(page, "complete");
      assert(
        (await page.locator("body").innerText()).includes(
          "Сервер не развёрнут",
        ),
      );
    });
  }
  await flow("gallery-sync", { width: 1440, height: 1100 }, async (page) => {
    await page.goto(`${base}/index.html?screen=meetings`);
    await page.frameLocator("#phone-preview").locator("#app > *").waitFor();
    await page.getByLabel("Выбрать экран макета").selectOption("prejoin");
    await page
      .frameLocator("#phone-preview")
      .locator('[data-action="enterCall"]')
      .click();
    await page.waitForFunction(
      () => document.querySelector("#screen-select").value === "call",
    );
    assert.equal(
      await page
        .frameLocator("#tablet-preview")
        .locator("body")
        .getAttribute("data-screen"),
      "call",
    );
    await page.getByLabel("Включить тёмную тему", { exact: true }).click();
    await page.waitForFunction(() =>
      [...document.querySelectorAll("iframe")].every(
        (frame) =>
          frame.contentDocument.documentElement.dataset.theme === "dark",
      ),
    );
    assert.equal(
      await page
        .frameLocator("#phone-preview")
        .locator("html")
        .getAttribute("data-theme"),
      "dark",
    );
    assert.equal(
      await page
        .frameLocator("#tablet-preview")
        .locator("html")
        .getAttribute("data-theme"),
      "dark",
    );
    await page.getByLabel("Выбрать экран макета").selectOption("settings");
    await page
      .frameLocator("#phone-preview")
      .locator('[data-action="accountSection"][data-section="appearance"]')
      .click();
    await page
      .frameLocator("#phone-preview")
      .getByRole("radio", { name: "Светлая тема", exact: true })
      .click();
    await page.waitForFunction(
      () =>
        document.body.dataset.theme === "light" &&
        [...document.querySelectorAll("iframe")].every(
          (frame) =>
            frame.contentDocument.documentElement.dataset.theme === "light",
        ),
    );
    await page.getByLabel("Повернуть планшет").click();
    assert.equal(
      await page.locator("#tablet-preview").getAttribute("width"),
      "834",
    );
    assert.equal(
      await page.locator("#tablet-preview").getAttribute("height"),
      "1194",
    );
  });
  const shots = [
    { name: "meetings-phone", screen: "meetings", viewport: viewports[1] },
    {
      name: "meetings-phone-dark",
      screen: "meetings",
      viewport: viewports[1],
      theme: "dark",
    },
    { name: "meetings-tablet", screen: "meetings", viewport: viewports[3] },
    {
      name: "meetings-tablet-portrait",
      screen: "meetings",
      viewport: viewports[2],
    },
    {
      name: "messages-phone-dark",
      screen: "messages",
      viewport: viewports[1],
      theme: "dark",
    },
    { name: "messages-tablet", screen: "messages", viewport: viewports[3] },
    {
      name: "profile-phone-dark",
      screen: "settings",
      viewport: viewports[1],
      theme: "dark",
    },
    {
      name: "three-tabs",
      url: `${base}/three-tabs.html?theme=light`,
      viewport: { width: 1400, height: 1200 },
    },
    {
      name: "gallery",
      url: `${base}/index.html?screen=meetings&theme=light&view=compare`,
      viewport: { width: 1600, height: 1200 },
    },
  ];
  for (const shot of shots) {
    const page = await browser.newPage({
      viewport: shot.viewport,
      deviceScaleFactor: 2,
    });
    await page.goto(shot.url || url(shot.screen, shot.theme), {
      waitUntil: "load",
    });
    if (shot.screen) await ready(page);
    else await page.evaluate(() => document.fonts.ready);
    await page.screenshot({
      path: `${outDir}previews/${shot.name}.png`,
      fullPage: false,
    });
    report.screenshots.push(`previews/${shot.name}.png`);
    await page.close();
  }
} catch (error) {
  report.issues.push({ fatal: true, error: error.message });
  console.error("QA RUN FAILED", error.message);
} finally {
  report.summary = {
    matrixPass: report.matrix.filter((x) => x.pass).length,
    matrixTotal: report.matrix.length,
    flowPass: report.flows.filter((x) => x.pass).length,
    flowTotal: report.flows.length,
    issueCount: report.issues.length,
  };
  await writeFile(
    `${outDir}qa-report.json`,
    JSON.stringify(report, null, 2) + "\n",
  );
  await browser.close();
  console.log(JSON.stringify(report.summary));
  if (report.issues.length) process.exitCode = 1;
}
