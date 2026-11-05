const fs = require("node:fs/promises");
const { serve, fixture, sample, chromium } = require("./harness.cjs");
const [directory, reportPath, secondsText = "1800"] = process.argv.slice(2);
const durationSeconds = Number(secondsText);
if (
  !directory ||
  !reportPath ||
  !Number.isInteger(durationSeconds) ||
  durationSeconds % 60 !== 0 ||
  durationSeconds < 60 ||
  durationSeconds > 3600
)
  throw new Error("Bounded 60–3600 second soak required");
(async () => {
  const server = await serve(directory);
  const browser = await chromium.launch({ channel: "chrome", headless: true });
  const startedAt = Date.now();
  const report = {
    kind: "isolated-browser-fixture",
    durationSeconds,
    samples: [],
    requestCounts: {},
    javascriptErrors: [],
    completed: false,
  };
  await fs.mkdir(require("node:path").dirname(reportPath), { recursive: true });
  try {
    const page = await browser.newPage({
      viewport: { width: 1440, height: 1000 },
    });
    const backend = await fixture(page);
    const cdp = await page.context().newCDPSession(page);
    await cdp.send("Performance.enable");
    await page.goto(server.url);
    await page.getByRole("heading", { name: "Обзор платформы" }).waitFor();
    await page.getByText("Изолированная проверка", { exact: true }).waitFor();
    for (let mark = 0; mark <= durationSeconds; mark += 60) {
      const target = startedAt + mark * 1000;
      while (Date.now() < target)
        await new Promise((resolve) =>
          setTimeout(resolve, Math.min(1000, target - Date.now())),
        );
      report.samples.push(
        await sample(page, cdp, `minute-${mark / 60}`, startedAt),
      );
      report.requestCounts = { ...backend.counts };
      report.javascriptErrors = [...backend.errors];
      await fs.writeFile(reportPath, JSON.stringify(report, null, 2));
      if (mark % 300 === 0)
        process.stdout.write(
          JSON.stringify({
            minute: mark / 60,
            heapUsedBytes: report.samples.at(-1).heapUsedBytes,
            nodes: report.samples.at(-1).nodes,
            listeners: report.samples.at(-1).listeners,
            requests: Object.values(backend.counts).reduce((a, b) => a + b, 0),
          }) + "\n",
        );
    }
    report.completed = true;
    report.actualDurationSeconds = Math.round((Date.now() - startedAt) / 1000);
    await fs.writeFile(reportPath, JSON.stringify(report, null, 2));
    process.stdout.write("SOAK_COMPLETED\n");
  } finally {
    await browser.close();
    await server.close();
  }
})();
