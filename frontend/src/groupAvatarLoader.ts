import { api, ApiError } from "./api";

// Ограничьте объем данных, передаваемых из одного UI, восемью слотами для приватного контента
// на сервере.
let active = 0;
const waiting: Array<() => void> = [];
async function withSlot<T>(load: () => Promise<T>, signal: AbortSignal) {
  signal.throwIfAborted();
  await new Promise<void>((resolve, reject) => {
    const enter = () => {
      signal.removeEventListener("abort", cancel);
      active++;
      resolve();
    };
    const cancel = () => {
      const index = waiting.indexOf(enter);
      if (index >= 0) waiting.splice(index, 1);
      reject(signal.reason);
    };
    if (active < 6) enter();
    else {
      waiting.push(enter);
      signal.addEventListener("abort", cancel, { once: true });
    }
  });
  try {
    signal.throwIfAborted();
    return await load();
  } finally {
    active--;
    waiting.shift()?.();
  }
}

function pause(ms: number, signal: AbortSignal) {
  signal.throwIfAborted();
  return new Promise<void>((resolve, reject) => {
    const cancel = () => {
      clearTimeout(timer);
      reject(signal.reason);
    };
    const timer = setTimeout(() => {
      signal.removeEventListener("abort", cancel);
      resolve();
    }, ms);
    signal.addEventListener("abort", cancel, { once: true });
  });
}

/** Без байтового кэша: каждый подключенный аватар имеет собственный приватный объект URL и освобождает его. */
export async function loadGroupAvatar(id: string, signal: AbortSignal) {
  return loadPrivateAvatar(() => api.groupAvatar(id, signal), signal);
}

/** Share the bounded authenticated image queue across group and user photos. */
export async function loadPrivateAvatar(
  load: () => Promise<Blob>,
  signal: AbortSignal,
) {
  for (let attempt = 0; ; attempt++) {
    try {
      return await withSlot(load, signal);
    } catch (error) {
      if (!(error instanceof ApiError) || error.status !== 429 || attempt >= 2)
        throw error;
      await pause(500 * (attempt + 1), signal);
    }
  }
}
