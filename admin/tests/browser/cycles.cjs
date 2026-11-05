const fs = require("node:fs/promises");
const crypto = require("node:crypto");
const { serve, fixture, sample, chromium } = require("./harness.cjs");
const [directory, output] = process.argv.slice(2);
(async () => {
  const server = await serve(directory);
  const browser = await chromium.launch({ channel: "chrome", headless: true });
  const startedAt = Date.now();
  const report = {
    kind: "isolated-browser-fixture",
    samples: [],
    navigationCount: 100,
    modalCycles: 50,
    loginLogoutCycles: 10,
  };
  try {
    const page = await browser.newPage({
      viewport: { width: 1440, height: 1000 },
    });
    const backend = await fixture(page);
    const cdp = await page.context().newCDPSession(page);
    await cdp.send("Performance.enable");
    await page.goto(server.url);
    await page.getByText("Изолированная проверка", { exact: true }).waitFor();
    report.samples.push(await sample(page, cdp, "baseline", startedAt));
    const sections = [
      "Пользователи",
      "Администраторы",
      "Конференции",
      "Чаты",
      "Записи",
      "Файлы",
      "Система",
      "Настройки",
      "Журнал аудита",
      "Главная",
    ];
    for (let i = 0; i < 100; i++) {
      await page
        .getByRole("link", { name: sections[i % sections.length], exact: true })
        .click();
      const name =
        {
          Файлы: "Файлы и хранилище",
          Система: "Состояние системы",
          Главная: "Обзор платформы",
        }[sections[i % sections.length]] || sections[i % sections.length];
      await page
        .getByRole("heading", { name, exact: true, level: 1 })
        .waitFor();
      if ((i + 1) % 25 === 0)
        report.samples.push(
          await sample(page, cdp, `navigation-${i + 1}`, startedAt),
        );
    }
    for (let i = 0; i < 50; i++) {
      await page
        .getByRole("button", { name: "Мой профиль", exact: true })
        .click();
      await page.getByRole("dialog").waitFor();
      await page.getByRole("button", { name: "Закрыть", exact: true }).click();
    }
    report.samples.push(await sample(page, cdp, "modal-50", startedAt));
    const fixturePassword = crypto.randomBytes(18).toString("hex");
    for (let i = 0; i < 10; i++) {
      await page
        .getByRole("button", { name: "Мой профиль", exact: true })
        .click();
      await page.getByRole("button", { name: "Выйти", exact: true }).click();
      await page
        .getByRole("heading", { name: "Вход в админку", exact: true })
        .waitFor();
      await page
        .getByLabel("Логин администратора")
        .fill(backend.administrator().username);
      await page.getByLabel("Пароль", { exact: true }).fill(fixturePassword);
      await page.getByRole("button", { name: "Войти", exact: true }).click();
      await page.locator("main h1").waitFor();
    }
    report.samples.push(await sample(page, cdp, "login-logout-10", startedAt));
    await page.getByText("Изолированная проверка", { exact: true }).waitFor();
    await page.locator(".toast").waitFor({ state: "hidden" });
    report.samples.push(await sample(page, cdp, "stable-final", startedAt));
    report.javascriptErrors = backend.errors;
    report.requestCounts = backend.counts;
    report.completed = true;
    await fs.mkdir(require("node:path").dirname(output), { recursive: true });
    await fs.writeFile(output, JSON.stringify(report, null, 2));
    process.stdout.write(JSON.stringify(report) + "\n");
  } finally {
    await browser.close();
    await server.close();
  }
})();
