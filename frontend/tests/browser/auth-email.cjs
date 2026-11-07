const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { chromium, serve } = require("../../../admin/tests/browser/harness.cjs");

const TOKEN = "SyntheticResetTokenForBrowserFixture00000000";
const PASSWORD = "SyntheticResetPassword42";
const USER = {
  id: "00000000-0000-4000-8000-000000000001",
  email: "fixture@example.test",
  displayName: "Fixture",
  createdAt: "2026-11-06T10:00:00Z",
  updatedAt: "2026-11-06T10:00:00Z",
  emailVerificationRequired: true,
};

(async () => {
  const root = path.resolve(__dirname, "../..");
  const server = await serve(path.join(root, "dist"));
  const browser = await chromium.launch({ channel: "chrome", headless: true });
  const report = { tests: {}, javascriptErrors: [], completed: false };
  async function fixture(options = {}) {
    const page = await browser.newPage({
      viewport: { width: 1440, height: 1100 },
    });
    page.setDefaultTimeout(10000);
    const state = {
      reset: [],
      recovery: [],
      verify: [],
      login: 0,
      resend: [],
      requests: [],
      ...options,
    };
    page.on("pageerror", (error) =>
      report.javascriptErrors.push(error.message),
    );
    page.on("request", (request) => {
      state.requests.push(request.url());
      assert(
        !request.url().includes(TOKEN),
        "bearer token must never appear in a request URL",
      );
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
      if (endpoint === "/auth/providers")
        return send({
          mode: "local",
          registrationEnabled: true,
          passwordRecoveryEnabled: true,
          ldap: { enabled: false, label: "Корпоративный вход" },
        });
      if (endpoint === "/auth/refresh")
        return send({ message: "unauthorized" }, 401);
      if (endpoint === "/auth/password-recovery") {
        state.recovery.push(request.postDataJSON());
        if (state.holdRecovery) {
          state.pending = route;
          return;
        }
        return send(
          state.recoveryError
            ? { message: state.recoveryError }
            : { status: "success" },
          state.recoveryStatus || 200,
        );
      }
      if (endpoint === "/auth/reset-password") {
        assert.equal(request.method(), "POST");
        assert.equal(request.headers().authorization, undefined);
        state.reset.push(request.postDataJSON());
        return send(
          state.resetError
            ? { message: state.resetError }
            : { status: "success" },
          state.resetStatus || 200,
        );
      }
      if (endpoint === "/auth/register") return send({ user: USER }, 201);
      if (endpoint === "/auth/login") {
        state.login++;
        return send({ message: "email verification required" }, 403);
      }
      if (endpoint === "/auth/verify-email") {
        state.verify.push(request.postDataJSON());
        return send({ status: "success" });
      }
      if (endpoint === "/auth/verify-email/resend") {
        state.resend.push(request.postDataJSON());
        return send(
          state.resendError
            ? { message: state.resendError }
            : { status: "success" },
          state.resendStatus || 200,
        );
      }
      return send({ items: [], status: "success" });
    });
    return { page, state };
  }
  async function resetForm(
    fixture,
    password = PASSWORD,
    confirmation = password,
  ) {
    await fixture.page.locator("#reset-password").fill(password);
    await fixture.page.locator("#reset-confirmation").fill(confirmation);
    await fixture.page
      .getByRole("button", { name: "Сохранить новый пароль", exact: true })
      .click();
  }
  try {
    const recovery = await fixture();
    const recoveryStart = new Date("2026-11-08T00:00:00Z");
    await recovery.page.clock.setFixedTime(recoveryStart);
    await recovery.page.goto(server.url + "/login");
    await recovery.page.getByLabel("Email", { exact: true }).fill(USER.email);
    await recovery.page
      .getByRole("link", { name: "Забыли пароль?", exact: true })
      .click();
    await recovery.page.waitForURL(server.url + "/reset-password");
    await recovery.page
      .getByRole("heading", { name: "Восстановление пароля", exact: true })
      .waitFor();
    assert.equal(
      await recovery.page.getByLabel("Email", { exact: true }).inputValue(),
      USER.email,
    );
    assert.equal(recovery.state.recovery.length, 0);
    assert.equal(new URL(recovery.page.url()).search, "");
    assert.equal(new URL(recovery.page.url()).hash, "");
    await recovery.page
      .getByRole("button", { name: "Отправить ссылку", exact: true })
      .click();
    await recovery.page
      .getByRole("status")
      .filter({ hasText: "Ссылка действует 1 час" })
      .waitFor();
    assert.deepEqual(recovery.state.recovery, [{ email: USER.email }]);
    assert(recovery.state.requests.every((url) => !url.includes(USER.email)));
    report.tests.loginRecoveryNavigatesAndPrefillsEmailWithoutSending = true;
    report.tests.recoveryFormReportsQueuedEmail = true;

    const recoveryButton = recovery.page.getByRole("button", {
      name: "Отправить ссылку",
      exact: true,
    });
    await recovery.page
      .getByRole("timer")
      .filter({ hasText: "02:00" })
      .waitFor();
    assert.equal(await recoveryButton.isDisabled(), true);
    await recovery.page
      .locator("form")
      .evaluate((form) => form.requestSubmit());
    await recovery.page.waitForTimeout(50);
    assert.equal(recovery.state.recovery.length, 1);
    await recovery.page.clock.setFixedTime(new Date(+recoveryStart + 30_000));
    await recovery.page
      .getByRole("timer")
      .filter({ hasText: "01:30" })
      .waitFor();
    await recovery.page.reload();
    await recovery.page
      .getByRole("timer")
      .filter({ hasText: "01:30" })
      .waitFor();
    await recovery.page
      .getByLabel("Email", { exact: true })
      .fill("another@example.test");
    assert.equal(await recoveryButton.isDisabled(), true);
    await recovery.page.getByLabel("Email", { exact: true }).fill(USER.email);
    await recovery.page.clock.setFixedTime(new Date(+recoveryStart + 119_000));
    await recovery.page
      .getByRole("timer")
      .filter({ hasText: "00:01" })
      .waitFor();
    assert.equal(await recoveryButton.isDisabled(), true);
    await recovery.page.clock.setFixedTime(new Date(+recoveryStart + 120_000));
    await recovery.page.locator("button[type=submit]:enabled").waitFor();
    assert.equal(await recovery.page.getByRole("timer").count(), 0);
    await recoveryButton.click();
    await recovery.page
      .getByRole("timer")
      .filter({ hasText: "02:00" })
      .waitFor();
    assert.equal(recovery.state.recovery.length, 2);
    report.tests.recoveryWaitsTwoMinutesAndSurvivesReload = true;

    // Receiving a reset link must not be blocked by the email request pause.
    await recovery.page.goto(server.url + "/reset-password#token=" + TOKEN);
    const resetDuringPause = recovery.page.getByRole("button", {
      name: "Сохранить новый пароль",
      exact: true,
    });
    await resetDuringPause.waitFor();
    assert.equal(await resetDuringPause.isDisabled(), false);
    assert.equal(await recovery.page.getByRole("timer").count(), 0);
    await resetForm(recovery);
    await recovery.page
      .getByRole("heading", { name: "Пароль изменён", exact: true })
      .waitFor();
    assert.deepEqual(recovery.state.reset, [
      { token: TOKEN, password: PASSWORD },
    ]);
    report.tests.passwordResetIsAvailableDuringRecoveryPause = true;

    // Verification emails and recovery emails keep separate resend deadlines.
    await recovery.page.goto(server.url + "/verify-email");
    await recovery.page.getByLabel("Email", { exact: true }).fill(USER.email);
    const verificationAfterRecovery = recovery.page.getByRole("button", {
      name: "Отправить письмо повторно",
      exact: true,
    });
    assert.equal(await verificationAfterRecovery.isDisabled(), false);
    await verificationAfterRecovery.click();
    await recovery.page
      .getByRole("timer")
      .filter({ hasText: "02:00" })
      .waitFor();
    assert.equal(recovery.state.resend.length, 1);
    report.tests.recoveryAndVerificationPausesAreIndependent = true;

    const emptyRecovery = await fixture();
    await emptyRecovery.page.goto(server.url + "/login");
    await emptyRecovery.page
      .getByRole("link", { name: "Забыли пароль?", exact: true })
      .click();
    await emptyRecovery.page.waitForURL(server.url + "/reset-password");
    await emptyRecovery.page
      .getByRole("heading", { name: "Восстановление пароля", exact: true })
      .waitFor();
    assert.equal(
      await emptyRecovery.page
        .getByLabel("Email", { exact: true })
        .inputValue(),
      "",
    );
    await emptyRecovery.page
      .getByRole("button", { name: "Отправить ссылку", exact: true })
      .click();
    await emptyRecovery.page
      .getByRole("alert")
      .filter({ hasText: "Введите корректный email." })
      .waitFor();
    assert.equal(emptyRecovery.state.recovery.length, 0);
    assert.equal(await emptyRecovery.page.getByRole("timer").count(), 0);
    assert.equal(
      await emptyRecovery.page
        .getByRole("button", { name: "Отправить ссылку", exact: true })
        .isDisabled(),
      false,
    );
    report.tests.forgotPasswordNavigatesWithAnEmptyEmail = true;
    report.tests.invalidRecoveryEmailDoesNotStartPause = true;

    const blocked = await fixture({
      recoveryError: "account blocked",
      recoveryStatus: 403,
    });
    await blocked.page.goto(server.url + "/login");
    await blocked.page.getByLabel("Email", { exact: true }).fill(USER.email);
    await blocked.page
      .getByRole("link", { name: "Забыли пароль?", exact: true })
      .click();
    await blocked.page
      .getByRole("button", { name: "Отправить ссылку", exact: true })
      .click();
    await blocked.page
      .getByRole("alert")
      .filter({ hasText: "Аккаунт заблокирован." })
      .waitFor();
    report.tests.blockedRecoveryMessage = true;

    const stale = await fixture({ holdRecovery: true });
    await stale.page.goto(server.url + "/login");
    await stale.page.getByLabel("Email", { exact: true }).fill(USER.email);
    await stale.page
      .getByRole("link", { name: "Забыли пароль?", exact: true })
      .click();
    await stale.page
      .getByRole("button", { name: "Отправить ссылку", exact: true })
      .click();
    await stale.page
      .waitForResponse(() => false, { timeout: 100 })
      .catch(() => {});
    assert(stale.state.pending);
    await stale.page.getByRole("link", { name: "Войти", exact: true }).click();
    await stale.page.waitForURL(server.url + "/login");
    await stale.page
      .getByRole("heading", { name: "Вход в MeetSpace", exact: true })
      .waitFor();
    await stale.state.pending
      .fulfill({
        status: 200,
        contentType: "application/json",
        body: '{"status":"success"}',
      })
      .catch(() => {});
    assert.equal(
      await stale.page
        .getByRole("status")
        .filter({ hasText: "Ссылка действует 1 час" })
        .count(),
      0,
    );
    report.tests.leavingRecoveryCancelsStaleResult = true;

    const reset = await fixture();
    await reset.page.goto(server.url + "/reset-password#token=" + TOKEN);
    await reset.page
      .getByRole("heading", { name: "Новый пароль", exact: true })
      .waitFor();
    await reset.page.waitForURL((url) => !url.hash);
    assert.equal(reset.state.reset.length, 0);
    assert.equal(
      await reset.page.evaluate(() =>
        JSON.stringify([
          Object.entries(sessionStorage),
          Object.entries(localStorage),
        ]).includes("SyntheticResetToken"),
      ),
      false,
    );
    report.tests.linkIsInMemoryAndRequiresExplicitSubmit = true;
    await resetForm(reset, "short");
    await reset.page
      .getByText("Пароль должен содержать от 8 до 128 символов.", {
        exact: true,
      })
      .waitFor();
    assert.equal(reset.state.reset.length, 0);
    await resetForm(reset, PASSWORD, "DifferentPassword42");
    await reset.page
      .getByText("Пароли не совпадают.", { exact: true })
      .waitFor();
    assert.equal(reset.state.reset.length, 0);
    report.tests.passwordRulesAndConfirmation = true;
    await resetForm(reset);
    await reset.page
      .getByRole("heading", { name: "Пароль изменён", exact: true })
      .waitFor();
    assert.deepEqual(reset.state.reset, [{ token: TOKEN, password: PASSWORD }]);
    assert.equal(reset.state.login, 0);
    assert.equal(await reset.page.locator('input[type="password"]').count(), 0);
    assert.equal(
      await reset.page
        .getByRole("link", { name: "Войти с новым паролем" })
        .getAttribute("href"),
      "/login",
    );
    report.tests.successClearsPasswordAndOffersLoginWithoutAutologin = true;

    const expired = await fixture({
      resetStatus: 422,
      resetError: "invalid or expired password reset link",
    });
    await expired.page.goto(server.url + "/reset-password#token=" + TOKEN);
    await resetForm(expired);
    await expired.page
      .getByText(
        "Ссылка недействительна, уже использована или срок её действия истёк. Запросите новую ссылку.",
        { exact: true },
      )
      .waitFor();
    await expired.page
      .getByRole("button", { name: "Запросить новую ссылку", exact: true })
      .click();
    await expired.page.getByLabel("Email", { exact: true }).fill(USER.email);
    await expired.page
      .getByRole("button", { name: "Отправить ссылку", exact: true })
      .click();
    await expired.page
      .getByRole("status")
      .filter({ hasText: "Ссылка действует 1 час" })
      .waitFor();
    assert.deepEqual(expired.state.recovery, [{ email: USER.email }]);
    report.tests.expiredLinkAllowsNewRecoveryEmail = true;

    const blockedReset = await fixture({
      resetStatus: 403,
      resetError: "account blocked",
    });
    await blockedReset.page.goto(server.url + "/reset-password#token=" + TOKEN);
    await resetForm(blockedReset);
    await blockedReset.page
      .getByText("Аккаунт заблокирован.", { exact: true })
      .waitFor();
    report.tests.blockedResetMessage = true;

    const mobile = await fixture({
      recoveryStatus: 503,
      recoveryError: "password recovery unavailable",
    });
    await mobile.page.setViewportSize({ width: 390, height: 844 });
    await mobile.page.goto(server.url + "/reset-password");
    await mobile.page.getByLabel("Email", { exact: true }).fill(USER.email);
    await mobile.page
      .getByRole("button", { name: "Отправить ссылку", exact: true })
      .click();
    await mobile.page
      .getByText(
        "Восстановление пароля недоступно. Обратитесь к администратору сервиса.",
        { exact: true },
      )
      .waitFor();
    assert(
      await mobile.page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    );
    report.tests.smtpUnavailableAndMobileLayout = true;
    assert.equal(await mobile.page.getByRole("timer").count(), 0);
    assert.equal(
      await mobile.page
        .getByRole("button", { name: "Отправить ссылку", exact: true })
        .isDisabled(),
      false,
    );
    report.tests.failedRecoveryDoesNotStartPause = true;

    const registration = await fixture();
    await registration.page.goto(server.url + "/register");
    await registration.page
      .getByLabel("Email", { exact: true })
      .fill(USER.email);
    await registration.page.locator("#password").fill(PASSWORD);
    await registration.page
      .getByRole("button", { name: "Зарегистрироваться", exact: true })
      .click();
    await registration.page
      .getByRole("heading", { name: "Проверьте почту", exact: true })
      .waitFor();
    assert.equal(
      await registration.page.locator("#verification-email").inputValue(),
      USER.email,
    );
    assert.equal(registration.state.login, 0);
    report.tests.registrationRequiresEmailVerificationBeforeLogin = true;

    const pendingLogin = await fixture();
    await pendingLogin.page.goto(server.url + "/login");
    await pendingLogin.page
      .getByLabel("Email", { exact: true })
      .fill(USER.email);
    await pendingLogin.page.locator("#password").fill(PASSWORD);
    await pendingLogin.page
      .getByRole("button", { name: "Войти", exact: true })
      .click();
    await pendingLogin.page
      .getByRole("link", {
        name: "Отправить письмо подтверждения повторно",
        exact: true,
      })
      .click();
    await pendingLogin.page
      .getByRole("button", { name: "Отправить письмо повторно", exact: true })
      .click();
    await pendingLogin.page
      .getByRole("status")
      .filter({ hasText: "письмо будет отправлено" })
      .waitFor();
    assert.deepEqual(pendingLogin.state.resend, [{ email: USER.email }]);
    report.tests.pendingLoginOffersVerificationResend = true;

    const verification = await fixture();
    await verification.page.goto(server.url + "/verify-email#token=" + TOKEN);
    await verification.page.waitForURL((url) => !url.hash);
    assert.equal(verification.state.verify.length, 0);
    await verification.page
      .getByRole("button", { name: "Подтвердить email", exact: true })
      .click();
    await verification.page
      .getByRole("heading", { name: "Email подтверждён", exact: true })
      .waitFor();
    assert.deepEqual(verification.state.verify, [{ token: TOKEN }]);
    assert.equal(verification.state.login, 0);
    report.tests.verificationLinkConfirmsAccountAndOffersLogin = true;

    const resend = await fixture();
    const resendStart = new Date("2026-11-08T00:00:00Z");
    await resend.page.clock.setFixedTime(resendStart);
    await resend.page.goto(server.url + "/verify-email");
    await resend.page.getByLabel("Email", { exact: true }).fill(USER.email);
    const resendButton = resend.page.getByRole("button", {
      name: "Отправить письмо повторно",
      exact: true,
    });
    await resendButton.click();
    await resend.page.getByRole("timer").filter({ hasText: "02:00" }).waitFor();
    assert.equal(await resendButton.isDisabled(), true);
    await resend.page.locator("form").evaluate((form) => form.requestSubmit());
    await resend.page.waitForTimeout(50);
    assert.equal(resend.state.resend.length, 1);

    await resend.page.clock.setFixedTime(new Date(+resendStart + 30_000));
    await resend.page.getByRole("timer").filter({ hasText: "01:30" }).waitFor();
    await resend.page.reload();
    await resend.page.getByRole("timer").filter({ hasText: "01:30" }).waitFor();
    await resend.page
      .getByLabel("Email", { exact: true })
      .fill("another@example.test");
    assert.equal(await resendButton.isDisabled(), true);
    await resend.page.getByLabel("Email", { exact: true }).fill(USER.email);
    await resend.page.clock.setFixedTime(new Date(+resendStart + 119_000));
    await resend.page.getByRole("timer").filter({ hasText: "00:01" }).waitFor();
    assert.equal(await resendButton.isDisabled(), true);
    await resend.page.clock.setFixedTime(new Date(+resendStart + 120_000));
    await resend.page.locator("button[type=submit]:enabled").waitFor();
    assert.equal(await resend.page.getByRole("timer").count(), 0);
    await resendButton.click();
    await resend.page.getByRole("timer").filter({ hasText: "02:00" }).waitFor();
    assert.equal(resend.state.resend.length, 2);
    report.tests.verificationResendWaitsTwoMinutesAndSurvivesReload = true;

    // A resend pause must not delay confirmation using an already received link.
    await resend.page.goto(server.url + "/verify-email#token=" + TOKEN);
    const confirmDuringPause = resend.page.getByRole("button", {
      name: "Подтвердить email",
      exact: true,
    });
    await confirmDuringPause.waitFor();
    assert.equal(await confirmDuringPause.isDisabled(), false);
    await confirmDuringPause.click();
    await resend.page
      .getByRole("heading", { name: "Email подтверждён", exact: true })
      .waitFor();
    report.tests.verificationConfirmationIsAvailableDuringResendPause = true;

    const failedResend = await fixture({
      resendStatus: 503,
      resendError: "service unavailable",
    });
    await failedResend.page.goto(server.url + "/verify-email");
    await failedResend.page
      .getByLabel("Email", { exact: true })
      .fill(USER.email);
    await failedResend.page
      .getByRole("button", { name: "Отправить письмо повторно", exact: true })
      .click();
    await failedResend.page.getByRole("alert").waitFor();
    assert.equal(await failedResend.page.getByRole("timer").count(), 0);
    assert.equal(
      await failedResend.page
        .getByRole("button", { name: "Отправить письмо повторно", exact: true })
        .isDisabled(),
      false,
    );
    report.tests.failedVerificationResendDoesNotStartPause = true;

    assert.deepEqual(report.javascriptErrors, []);
    report.completed = true;
  } finally {
    await fs.mkdir(path.join(root, "test-results"), { recursive: true });
    await fs.writeFile(
      path.join(root, "test-results/auth-email.json"),
      JSON.stringify(report, null, 2),
    );
    console.log(JSON.stringify(report));
    await browser.close();
    await server.close();
  }
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
