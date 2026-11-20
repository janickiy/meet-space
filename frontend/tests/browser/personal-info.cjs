const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const zlib = require("node:zlib");
const { chromium, serve } = require("../../../admin/tests/browser/harness.cjs");

const actorId = "00000000-0000-4000-8000-000000000001";
const peerId = "00000000-0000-4000-8000-000000000002";
const memberId = "00000000-0000-4000-8000-000000000003";
const directId = "00000000-0000-4000-8000-000000000101";
const groupId = "00000000-0000-4000-8000-000000000102";
const stamp = "2026-11-16T16:00:00Z";
const user = {
  id: actorId,
  displayName: "Александр",
  email: "alex@example.test",
  authSource: "local",
  bio: "",
  createdAt: stamp,
  updatedAt: stamp,
};
const peer = {
  id: peerId,
  displayName: "Анна Смирнова",
  email: "anna@example.test",
  bio: "Дизайнер интерфейсов.\nЛюблю путешествия и хорошие книги.",
  createdAt: "2024-02-05T12:15:00Z",
};
const participant = {
  id: memberId,
  displayName: "Михаил Волков",
  email: "mikhail@example.test",
  bio: "",
};
const common = {
  createdAt: stamp,
  updatedAt: stamp,
  lastMessageAt: stamp,
  lastMessageId: null,
  preview: "Материалы проекта",
  unreadCount: 0,
  notificationsEnabled: true,
};
const direct = {
  ...common,
  id: directId,
  type: "direct",
  peer: { id: peerId, displayName: peer.displayName },
  historyClearedThrough: 0,
};
const group = {
  ...common,
  id: groupId,
  type: "group",
  name: "Демо · Команда продукта",
  description: "Обсуждаем идеи и делимся материалами проекта.",
  memberCount: 3,
  myRole: "owner",
  createdBy: actorId,
  avatarVersion: null,
  lastSender: peer,
};
const file = (id, filename, mimeType) => ({
  id,
  filename,
  mimeType,
  size: 12345,
  status: "attached",
});
const photo = file(
  "00000000-0000-4000-8000-000000000301",
  "Команда.png",
  "image/png",
);
const photo2 = file(
  "00000000-0000-4000-8000-000000000302",
  "Эскиз.png",
  "image/png",
);
const document = file(
  "00000000-0000-4000-8000-000000000303",
  "План проекта.pdf",
  "application/pdf",
);
const video = file(
  "00000000-0000-4000-8000-000000000304",
  "Демонстрация.mp4",
  "video/mp4",
);
const message = (scope, suffix, text, attachments = []) => ({
  id: `00000000-0000-4000-8000-0000000002${suffix}`,
  sequence: String(200 + Number(suffix)),
  conversationId: scope,
  senderId: peerId,
  senderName: peer.displayName,
  text,
  createdAt: stamp,
  updatedAt: stamp,
  deletedAt: null,
  version: 1,
  attachments,
});
// A deterministic test image verifies real browser decoding without personal photos.
function fixturePNG() {
  const width = 240,
    height = 160;
  const scanlines = Buffer.alloc((width * 4 + 1) * height);
  for (let y = 0; y < height; y++)
    for (let x = 0; x < width; x++) {
      const at = y * (width * 4 + 1) + 1 + x * 4;
      scanlines[at] = 65 + Math.floor(x / 2);
      scanlines[at + 1] = 95 + Math.floor(y / 2);
      scanlines[at + 2] = 200;
      scanlines[at + 3] = 255;
    }
  function chunk(type, body) {
    const bytes = Buffer.concat([Buffer.from(type), body]);
    let crc = 0xffffffff;
    for (const byte of bytes) {
      crc ^= byte;
      for (let bit = 0; bit < 8; bit++)
        crc = (crc >>> 1) ^ (crc & 1 ? 0xedb88320 : 0);
    }
    const header = Buffer.alloc(4),
      checksum = Buffer.alloc(4);
    header.writeUInt32BE(body.length);
    checksum.writeUInt32BE((crc ^ 0xffffffff) >>> 0);
    return Buffer.concat([header, bytes, checksum]);
  }
  const header = Buffer.alloc(13);
  header.writeUInt32BE(width, 0);
  header.writeUInt32BE(height, 4);
  header[8] = 8;
  header[9] = 6;
  return Buffer.concat([
    Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]),
    chunk("IHDR", header),
    chunk("IDAT", zlib.deflateSync(scanlines)),
    chunk("IEND", Buffer.alloc(0)),
  ]);
}
const png = fixturePNG();
photo.size = photo2.size = png.length;
const pdf = Buffer.from("%PDF-1.4\n% Synthetic fixture\n%%EOF\n");
document.size = pdf.length;

(async () => {
  const output = path.resolve(__dirname, "../../../tmp/personal-info-20261116");
  await fs.mkdir(output, { recursive: true });
  const server = await serve(path.join(__dirname, "../../dist"));
  const browser = await chromium.launch({ channel: "chrome", headless: true });
  const errors = [];
  const completed = [];
  async function fixture(options = {}) {
    const state = {
      profileError: 0,
      materialsError: 0,
      requests: [],
      role: "owner",
      peerCreatedAt: peer.createdAt,
      ...options,
    };
    const page = await browser.newPage({
      viewport: options.viewport || { width: 1440, height: 1100 },
      timezoneId: "Europe/Moscow",
    });
    page.setDefaultTimeout(10000);
    await page.emulateMedia({ reducedMotion: "reduce" });
    page.on("pageerror", (error) => errors.push(error.message));
    await page.context().route(/\.svg(?:\?.*)?$/, async (route) => {
      const filename = new URL(route.request().url()).pathname.slice(1);
      const body = await fs.readFile(
        path.join(__dirname, "../../dist", filename),
      );
      return route.fulfill({ status: 200, contentType: "image/svg+xml", body });
    });
    await page.context().route("**/api/v1/**", async (route) => {
      const request = route.request();
      const url = new URL(request.url());
      const endpoint = url.pathname.replace("/api/v1", "");
      state.requests.push({
        endpoint,
        method: request.method(),
        query: url.search,
      });
      const send = (body, status = 200) =>
        route.fulfill({
          status,
          contentType: "application/json",
          body: JSON.stringify(body),
        });
      if (endpoint === "/auth/refresh")
        return send({
          accessToken: "synthetic-profile-token",
          tokenType: "Bearer",
          expiresIn: 900,
          user,
        });
      if (endpoint === "/auth/me") return send({ user });
      if (endpoint === "/auth/providers")
        return send({
          mode: "local",
          registrationEnabled: true,
          passwordRecoveryEnabled: true,
          ldap: { enabled: false },
        });
      if (endpoint === "/auth/accounts/list") return send({ items: [user] });
      if (endpoint.includes("/avatar/content"))
        return route.fulfill({
          status: 200,
          contentType: "image/png",
          headers: { "content-length": String(png.length) },
          body: png,
        });
      if (endpoint === "/conversations")
        return send({
          items: [direct, { ...group, myRole: state.role }],
          nextCursor: null,
          unreadCount: 0,
        });
      if (endpoint === `/conversations/${directId}`)
        return send({ status: "success", item: direct });
      if (endpoint === `/conversations/${groupId}`)
        return send({
          status: "success",
          item: { ...group, myRole: state.role },
        });
      if (endpoint.endsWith("/peer-presence"))
        return send({
          status: "success",
          item: { conversationId: directId, peerId, online: true },
        });
      if (endpoint.endsWith("/members"))
        return send({
          items: [
            { ...user, role: state.role, online: true },
            { ...peer, role: "member", online: true },
            { ...participant, role: "member", online: false },
          ],
        });
      if (endpoint.includes("/profiles/")) {
        if (state.profileError)
          return send({ message: "Профиль недоступен" }, state.profileError);
        const id = endpoint.split("/").at(-1);
        return send({
          status: "success",
          item:
            id === peerId
              ? {
                  ...peer,
                  bio: state.emptyBio ? "" : peer.bio,
                  createdAt: state.peerCreatedAt,
                }
              : id === actorId
                ? user
                : participant,
        });
      }
      const scope = endpoint.split("/")[2];
      const baseMessage = message(scope, "01", "Материалы проекта");
      if (endpoint.endsWith("/materials")) {
        if (state.materialsError)
          return send(
            { message: "Материалы недоступны" },
            state.materialsError,
          );
        const kind = url.searchParams.get("kind");
        if (kind === "media") {
          return url.searchParams.has("before")
            ? send({
                items: [
                  {
                    message: message(scope, "02", "Эскизы", [photo2]),
                    attachment: photo2,
                  },
                ],
                nextCursor: null,
              })
            : send({
                items: [
                  {
                    message: message(scope, "03", "Фото команды", [photo]),
                    attachment: photo,
                  },
                  {
                    message: message(scope, "04", "Видео", [video]),
                    attachment: video,
                  },
                ],
                nextCursor: "synthetic-media-page-2",
              });
        }
        if (kind === "file")
          return send({
            items: [
              {
                message: message(scope, "05", "План", [document]),
                attachment: document,
              },
            ],
            nextCursor: null,
          });
        return send({
          items: [
            {
              message: { ...baseMessage, text: "https://example.test/project" },
              url: "https://example.test/project",
            },
          ],
          nextCursor: null,
        });
      }
      if (endpoint.endsWith("/download"))
        return send({
          url: `/api/v1/conversations/${scope}/attachments/${endpoint.split("/").at(-2)}/content`,
          expiresAt: "2099-01-01T00:00:00Z",
          authenticated: scope === groupId,
        });
      if (endpoint.includes("/attachments/") && endpoint.endsWith("/content")) {
        if (scope === groupId)
          assert.equal(
            request.headers().authorization,
            "Bearer synthetic-profile-token",
          );
        const isPDF = endpoint.includes(document.id);
        const body = isPDF ? pdf : png;
        return route.fulfill({
          status: 200,
          contentType: isPDF ? "application/pdf" : "image/png",
          headers: { "content-length": String(body.length) },
          body,
        });
      }
      if (endpoint.endsWith("/messages") || endpoint.endsWith("/context"))
        return send({
          items: [
            endpoint.endsWith("/context")
              ? {
                  ...baseMessage,
                  id: endpoint.split("/").at(-2),
                  text: "Фото команды",
                }
              : baseMessage,
          ],
          nextCursor: null,
          unreadCount: 0,
          lastReadMessageId: baseMessage.id,
        });
      if (endpoint.endsWith("/pins")) return send({ items: [] });
      if (endpoint.endsWith("/chat/read") || endpoint.endsWith("/read"))
        return send({
          status: "success",
          item: { lastReadMessageId: baseMessage.id, unreadCount: 0 },
        });
      if (endpoint === "/conversations/direct")
        return send({ status: "success", item: direct });
      if (endpoint.includes("/calls")) return send({ item: null });
      if (endpoint.includes("ticket"))
        return send({ message: "Offline fixture" }, 503);
      return send({ items: [], nextCursor: null, unreadCount: 0 });
    });
    return { page, state };
  }
  async function noOverflow(page) {
    assert(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth + 1,
      ),
      "page must not overflow horizontally",
    );
    assert(
      await page
        .locator(".personal-content")
        .evaluate((node) => node.scrollWidth <= node.clientWidth + 1),
      "content must not overflow horizontally",
    );
  }
  try {
    const desktop = await fixture();
    await desktop.page.goto(`${server.url}/personal/${directId}?filter=all`);
    const composer = desktop.page.getByRole("textbox", {
      name: "Сообщение",
      exact: true,
    });
    await composer.fill("Черновик останется после просмотра профиля");
    await desktop.page
      .getByRole("button", {
        name: `Информация о пользователе: ${peer.displayName}`,
        exact: true,
      })
      .click();
    await desktop.page
      .getByRole("heading", { name: "Информация о пользователе", exact: true })
      .waitFor();
    await desktop.page.getByText(peer.email, { exact: true }).waitFor();
    await desktop.page.locator(".conversation-profile-avatar img").waitFor();
    await desktop.page.waitForFunction(
      () =>
        document.querySelector(".conversation-profile-avatar img")
          ?.naturalWidth > 0,
    );

    assert.equal(
      await desktop.page.getByRole("dialog").count(),
      0,
      "profile is a page, not a dialog",
    );
    assert.equal(
      await desktop.page.locator(".conversation-profile-bio").innerText(),
      peer.bio,
    );
    const registration = desktop.page
      .locator(".conversation-profile-details div")
      .filter({
        has: desktop.page.getByText("Дата регистрации", { exact: true }),
      });
    assert.equal(
      await registration.locator("time").getAttribute("datetime"),
      peer.createdAt,
    );
    assert.equal(
      (await registration.locator("dd").innerText()).replace(/\s+/g, " "),
      "5 февраля 2024 г.",
    );
    assert(
      await desktop.page
        .getByRole("navigation", { name: "Переписки" })
        .isVisible(),
    );
    await desktop.page
      .getByRole("button", { name: `Открыть ${photo.filename}`, exact: true })
      .waitFor();
    await noOverflow(desktop.page);
    await desktop.page.screenshot({
      path: path.join(output, "direct-desktop.png"),
      fullPage: true,
      animations: "disabled",
    });
    completed.push(
      "direct profile shows peer registration date without time and keeps sidebar visible",
    );
    const firstPhoto = desktop.page.getByRole("button", {
      name: `Открыть ${photo.filename}`,
      exact: true,
    });
    await firstPhoto.scrollIntoViewIfNeeded();
    await firstPhoto.locator("img").waitFor();
    assert(
      await firstPhoto
        .locator("img")
        .evaluate((image) => image.naturalWidth > 0),
    );
    await desktop.page.screenshot({
      path: path.join(output, "direct-media-desktop.png"),
      fullPage: true,
      animations: "disabled",
    });
    await firstPhoto.click();
    const preview = desktop.page.getByRole("dialog", {
      name: photo.filename,
      exact: true,
    });
    await preview.locator("img").waitFor();
    assert(
      await preview.locator("img").evaluate((image) => image.naturalWidth > 0),
    );
    await desktop.page.screenshot({
      path: path.join(output, "media-preview.png"),
      fullPage: true,
      animations: "disabled",
    });
    await preview
      .getByRole("button", { name: "Показать в чате", exact: true })
      .click();
    await desktop.page.locator(".chat-message-focused").waitFor();
    assert.equal(
      await desktop.page
        .locator(".chat-message-focused")
        .getAttribute("data-testid"),
      `chat-message-${message(directId, "03", "").id}`,
    );
    assert.equal(
      await composer.inputValue(),
      "Черновик останется после просмотра профиля",
    );
    await desktop.page
      .getByRole("button", {
        name: `Информация о пользователе: ${peer.displayName}`,
        exact: true,
      })
      .click();
    completed.push(
      "avatar, photo thumbnail and full preview decode real PNG bytes",
      "show in chat opens the source message and preserves the draft",
    );
    await desktop.page
      .getByRole("button", { name: "Показать ещё", exact: true })
      .click();
    await desktop.page
      .getByRole("button", { name: `Открыть ${photo2.filename}`, exact: true })
      .waitFor();
    assert(
      desktop.state.requests.some((r) =>
        r.query.includes("before=synthetic-media-page-2"),
      ),
    );
    completed.push("media pagination loads older shared attachments");
    await desktop.page.getByRole("tab", { name: "Файлы", exact: true }).click();
    await desktop.page.getByText(document.filename, { exact: true }).waitFor();

    await desktop.page
      .getByRole("tab", { name: "Ссылки", exact: true })
      .click();
    assert.equal(
      await desktop.page
        .getByRole("link", {
          name: "https://example.test/project",
          exact: true,
        })
        .getAttribute("rel"),
      "noopener noreferrer",
    );
    await desktop.page
      .getByRole("tab", { name: "Ссылки", exact: true })
      .press("Home");
    assert.equal(
      await desktop.page
        .getByRole("tab", { name: "Медиа", exact: true })
        .getAttribute("aria-selected"),
      "true",
    );
    completed.push("file/link tabs and keyboard tab navigation");
    await desktop.page
      .getByRole("button", { name: "Назад к переписке", exact: true })
      .click();
    assert.equal(
      await composer.inputValue(),
      "Черновик останется после просмотра профиля",
    );
    assert.equal(new URL(desktop.page.url()).searchParams.get("view"), null);
    assert.equal(new URL(desktop.page.url()).searchParams.get("filter"), "all");
    completed.push("return keeps draft and conversation filter");
    await desktop.page
      .getByRole("button", {
        name: `Информация о пользователе: ${peer.displayName}`,
        exact: true,
      })
      .click();
    await desktop.page.goBack();
    assert.equal(
      await composer.inputValue(),
      "Черновик останется после просмотра профиля",
    );
    completed.push("browser back returns to same chat draft");
    await desktop.page.close();

    const noBio = await fixture({ emptyBio: true, peerCreatedAt: "invalid" });
    await noBio.page.goto(`${server.url}/personal/${directId}?view=info`);
    await noBio.page.getByText(peer.email, { exact: true }).waitFor();
    assert.equal(
      await noBio.page
        .locator(".conversation-profile-details dt")
        .filter({ hasText: "О себе" })
        .count(),
      0,
    );
    assert.equal(
      await noBio.page.getByText("Дата регистрации", { exact: true }).count(),
      0,
    );
    completed.push("empty bio and invalid registration date are omitted");
    await noBio.page.close();

    const owner = await fixture();
    await owner.page.goto(`${server.url}/personal/${groupId}?view=info`);
    await owner.page
      .getByRole("heading", { name: "Информация о группе", exact: true })
      .waitFor();
    await owner.page
      .getByRole("button", {
        name: `Информация о пользователе: ${participant.displayName}`,
        exact: true,
      })
      .waitFor();
    assert.equal(await owner.page.getByRole("dialog").count(), 0);
    await owner.page
      .getByRole("button", { name: "Добавить участников", exact: true })
      .waitFor();
    await noOverflow(owner.page);
    await owner.page.screenshot({
      path: path.join(output, "group-desktop.png"),
      fullPage: true,
      animations: "disabled",
    });
    const groupPhoto = owner.page.getByRole("button", {
      name: `Открыть ${photo.filename}`,
      exact: true,
    });
    await groupPhoto.scrollIntoViewIfNeeded();
    await groupPhoto.locator("img").waitFor();
    assert(
      await groupPhoto
        .locator("img")
        .evaluate((image) => image.naturalWidth > 0),
    );
    await owner.page.screenshot({
      path: path.join(output, "group-media-desktop.png"),
      fullPage: true,
      animations: "disabled",
    });
    await owner.page.getByRole("tab", { name: "Файлы", exact: true }).click();
    await owner.page.getByText(document.filename, { exact: true }).waitFor();
    const downloadEvent = owner.page.waitForEvent("download");
    await owner.page
      .getByRole("button", {
        name: `Скачать ${document.filename}`,
        exact: true,
      })
      .click();
    const download = await downloadEvent;
    assert.equal(download.suggestedFilename(), document.filename);
    assert(
      (await fs.readFile(await download.path())).equals(pdf),
      "downloaded fixture bytes match the PDF response",
    );
    completed.push("file download preserves filename and returned bytes");

    await owner.page.getByRole("tab", { name: "Медиа", exact: true }).click();

    await owner.page
      .getByRole("button", { name: "Настройки группы", exact: true })
      .click();
    await owner.page
      .getByRole("dialog", { name: "Настройки группы", exact: true })
      .waitFor();
    await owner.page.keyboard.press("Escape");
    assert.equal(await owner.page.getByRole("dialog").count(), 0);
    await owner.page
      .getByRole("button", {
        name: `Информация о пользователе: ${participant.displayName}`,
        exact: true,
      })
      .click();
    await owner.page.getByText(participant.email, { exact: true }).waitFor();
    assert.equal(await owner.page.getByRole("dialog").count(), 0);
    assert.equal(
      await owner.page.getByText("Дата регистрации", { exact: true }).count(),
      0,
      "profiles from older servers may omit the registration timestamp",
    );
    assert.equal(
      await owner.page
        .locator(".conversation-profile-details dt")
        .filter({ hasText: "О себе" })
        .count(),
      0,
    );
    await owner.page
      .getByRole("button", { name: "Назад к информации о группе", exact: true })
      .click();
    await owner.page
      .getByRole("heading", { name: "Информация о группе", exact: true })
      .waitFor();
    completed.push(
      "group page, management dialog and participant profile/back",
    );
    await owner.page
      .getByRole("button", {
        name: `Действия с перепиской: ${group.name}`,
        exact: true,
      })
      .click();
    await owner.page
      .getByRole("menuitem", { name: "Покинуть группу", exact: true })
      .click();
    await owner.page
      .getByRole("button", { name: "Выбрать нового владельца", exact: true })
      .click();
    await owner.page
      .getByRole("dialog")
      .getByRole("button", {
        name: `Информация о пользователе: ${participant.displayName}`,
        exact: true,
      })
      .click();
    await owner.page.getByText(participant.email, { exact: true }).waitFor();
    assert.equal(await owner.page.getByRole("dialog").count(), 0);
    completed.push(
      "participant profile opened from management also uses full page",
    );
    await owner.page.close();

    const member = await fixture({ role: "member" });
    await member.page.goto(`${server.url}/personal/${groupId}?view=info`);
    await member.page
      .getByRole("button", {
        name: `Информация о пользователе: ${participant.displayName}`,
        exact: true,
      })
      .waitFor();
    assert.equal(
      await member.page
        .getByRole("button", { name: "Добавить участников", exact: true })
        .count(),
      0,
    );
    assert.equal(
      await member.page
        .getByRole("button", { name: "Настройки группы", exact: true })
        .count(),
      0,
    );
    completed.push("ordinary group member sees no management actions");
    await member.page.close();

    const mobile = await fixture({ viewport: { width: 390, height: 844 } });
    await mobile.page.goto(`${server.url}/personal/${directId}?view=info`);
    await mobile.page.getByText(peer.email, { exact: true }).waitFor();
    await noOverflow(mobile.page);
    await mobile.page.screenshot({
      path: path.join(output, "direct-mobile.png"),
      fullPage: true,
      animations: "disabled",
    });
    await mobile.page.goto(`${server.url}/personal/${groupId}?view=info`);
    await mobile.page
      .getByRole("button", {
        name: `Информация о пользователе: ${participant.displayName}`,
        exact: true,
      })
      .waitFor();
    await noOverflow(mobile.page);
    await mobile.page.screenshot({
      path: path.join(output, "group-mobile.png"),
      fullPage: true,
      animations: "disabled",
    });
    completed.push("390px direct/group screens have no horizontal overflow");
    await mobile.page.close();

    const denied = await fixture({ profileError: 404 });
    await denied.page.goto(
      `${server.url}/personal/${groupId}?view=info&profile=${memberId}`,
    );
    await denied.page
      .getByRole("button", { name: "Повторить загрузку профиля", exact: true })
      .waitFor();
    assert.equal(
      await denied.page.locator(".conversation-profile-details").count(),
      0,
    );
    denied.state.profileError = 0;
    await denied.page
      .getByRole("button", { name: "Повторить загрузку профиля", exact: true })
      .click();
    await denied.page.getByText(participant.email, { exact: true }).waitFor();
    completed.push(
      "unavailable participant profile hides personal data and can retry",
    );
    await denied.page.close();
    const revoked = await fixture({ materialsError: 403 });
    await revoked.page.goto(`${server.url}/personal/${directId}?view=info`);
    await revoked.page.waitForURL((url) => url.pathname === "/personal");
    assert.equal(
      await revoked.page.locator(".personal-info-screen").count(),
      0,
    );
    assert.equal(
      await revoked.page.locator(".conversation-profile-details").count(),
      0,
    );
    completed.push("revoked conversation access closes profile and materials");
    await revoked.page.close();
    assert.deepEqual(errors, []);
    await fs.writeFile(
      path.join(output, "result.json"),
      JSON.stringify({ completed, errors }, null, 2),
    );
    console.log(
      JSON.stringify(
        { passed: completed.length, completed, screenshots: output },
        null,
        2,
      ),
    );
  } finally {
    await browser.close();
    await server.close();
  }
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
