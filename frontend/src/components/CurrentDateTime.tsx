import { useEffect, useState } from "react";

const dateFormat = new Intl.DateTimeFormat("ru-RU", {
  day: "numeric",
  month: "long",
  year: "numeric",
});
const timeFormat = new Intl.DateTimeFormat("ru-RU", {
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hour12: false,
});

/** Показывает текущие дату и время браузера, обновляя только часы каждую секунду. */
export function CurrentDateTime() {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const timer = window.setInterval(() => setNow(new Date()), 1000);
    return () => window.clearInterval(timer);
  }, []);

  return (
    <time
      className="dashboard-current-time"
      dateTime={now.toISOString()}
      title="Текущие дата и время"
    >
      <span>{dateFormat.format(now)}</span>
      <span aria-hidden="true">·</span>
      <span>{timeFormat.format(now)}</span>
    </time>
  );
}
