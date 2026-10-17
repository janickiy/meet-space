import { spawn } from "node:child_process";
import { access, mkdtemp, writeFile, rm } from "node:fs/promises";
import { constants } from "node:fs";
import { join, dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { tmpdir } from "node:os";
import { createServer } from "vite";

// Uses an isolated headless browser and synthetic PCM, never real capture devices.
// Run: node scripts/check-microphone-gain-browser.mjs [result.json]
// Set CHROME_BIN when Chrome/Chromium is installed outside the standard paths.
const frontendRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const candidates = process.env.CHROME_BIN
  ? [process.env.CHROME_BIN]
  : [
      "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
      "/usr/bin/google-chrome",
      "/usr/bin/chromium",
      "/usr/bin/chromium-browser",
    ];
let chromePath;
for (const candidate of candidates) {
  try {
    await access(candidate, constants.X_OK);
    chromePath = candidate;
    break;
  } catch {}
}
if (!chromePath)
  throw new Error("Chrome/Chromium was not found. Set CHROME_BIN.");
const profile = await mkdtemp(join(tmpdir(), "meetspace-mic-browser-"));
let chrome, browserSocket, pageSocket, server;
const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

async function connect(url) {
  const socket = new WebSocket(url);
  await new Promise((resolve, reject) => {
    const timeout = setTimeout(() => {
      socket.close();
      reject(new Error("CDP connection timeout"));
    }, 10000);
    socket.onopen = () => {
      clearTimeout(timeout);
      resolve();
    };
    socket.onerror = (error) => {
      clearTimeout(timeout);
      reject(error);
    };
  });
  return socket;
}
function cdp(socket) {
  let id = 0;
  const pending = new Map();
  socket.addEventListener("message", (e) => {
    const message = JSON.parse(e.data),
      request = pending.get(message.id);
    if (!request) return;
    pending.delete(message.id);
    clearTimeout(request.timeout);
    if (message.error) request.reject(new Error(JSON.stringify(message.error)));
    else request.resolve(message.result);
  });
  socket.addEventListener("close", () => {
    for (const request of pending.values()) {
      clearTimeout(request.timeout);
      request.reject(new Error("CDP target closed"));
    }
    pending.clear();
  });
  return (method, params = {}, duration = 20000) =>
    new Promise((resolve, reject) => {
      const next = ++id,
        timeout = setTimeout(() => {
          pending.delete(next);
          reject(new Error(`CDP timeout: ${method}`));
        }, duration);
      pending.set(next, { resolve, reject, timeout });
      socket.send(JSON.stringify({ id: next, method, params }));
    });
}

try {
  server = await createServer({
    root: frontendRoot,
    configFile: false,
    cacheDir: join(profile, "vite-cache"),
    clearScreen: false,
    logLevel: "error",
    server: { host: "127.0.0.1", port: 0, strictPort: false, hmr: false },
  });
  await server.listen();
  const port = server.httpServer.address().port;
  chrome = spawn(
    chromePath,
    [
      "--headless=new",
      "--remote-debugging-address=127.0.0.1",
      "--remote-debugging-port=0",
      `--user-data-dir=${profile}`,
      "--autoplay-policy=no-user-gesture-required",
      "--no-first-run",
      "--no-default-browser-check",
      "--disable-background-networking",
      "--disable-component-update",
      "--disable-sync",
      "--disable-renderer-backgrounding",
      "--disable-background-timer-throttling",
      "--disable-backgrounding-occluded-windows",
      "--disable-features=MediaRouter",
      "about:blank",
    ],
    { stdio: ["ignore", "ignore", "pipe"] },
  );
  const ws = await new Promise((resolve, reject) => {
    let stderr = "";
    const timer = setTimeout(
      () => reject(new Error(`Chrome startup timeout: ${stderr.slice(-1500)}`)),
      15000,
    );
    chrome.stderr.on("data", (chunk) => {
      stderr = (stderr + chunk).slice(-16384);
      const match = stderr.match(/DevTools listening on (ws:\/\/[^\s]+)/);
      if (match) {
        clearTimeout(timer);
        resolve(match[1]);
      }
    });
    chrome.on("error", (error) => {
      clearTimeout(timer);
      reject(error);
    });
    chrome.on("exit", (code) => {
      clearTimeout(timer);
      reject(new Error(`Chrome exited ${code}: ${stderr.slice(-1500)}`));
    });
  });
  browserSocket = await connect(ws);
  const browser = cdp(browserSocket);
  const { targetId } = await browser("Target.createTarget", {
    url: "about:blank",
  });
  pageSocket = await connect(
    ws.replace(/\/devtools\/browser\/[^/]+$/, `/devtools/page/${targetId}`),
  );
  const page = cdp(pageSocket);
  await page("Runtime.enable");
  await page("Page.enable");
  await page("Page.navigate", {
    url: `http://127.0.0.1:${port}/tests/microphone-gain-browser.html`,
  });
  const deadline = Date.now() + 30000;
  while (true) {
    let ready = false;
    try {
      const state = await page("Runtime.evaluate", {
        expression: "typeof window.micGainVerification !== 'undefined'",
        returnByValue: true,
      });
      ready = state.result?.value === true;
    } catch (error) {
      if (
        !/navigated|context was destroyed|Cannot find context/.test(
          String(error),
        )
      )
        throw error;
    }
    if (ready) break;
    if (Date.now() > deadline) throw new Error("Browser harness did not load");
    await delay(50);
  }
  const response = await page(
    "Runtime.evaluate",
    {
      expression: `window.micGainVerification`,
      awaitPromise: true,
      returnByValue: true,
      timeout: 45000,
    },
    50000,
  );
  if (response.exceptionDetails)
    throw new Error(JSON.stringify(response.exceptionDetails));
  const result = response.result.value;
  if (process.argv[2])
    await writeFile(
      resolve(process.argv[2]),
      JSON.stringify(result, null, 2) + "\n",
    );
  console.log(JSON.stringify(result, null, 2));
  process.exitCode = result.ok ? 0 : 1;
  await browser("Browser.close").catch(() => {});
} catch (error) {
  console.error(error.stack || String(error));
  process.exitCode = 1;
} finally {
  pageSocket?.close();
  browserSocket?.close();
  if (chrome && chrome.exitCode === null) {
    chrome.kill("SIGTERM");
    await Promise.race([
      new Promise((resolve) => chrome.once("exit", resolve)),
      delay(2000),
    ]);
    if (chrome.exitCode === null) chrome.kill("SIGKILL");
  }
  await server?.close();
  await rm(profile, {
    recursive: true,
    force: true,
    maxRetries: 5,
    retryDelay: 100,
  });
}
