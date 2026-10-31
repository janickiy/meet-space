import assert from "node:assert/strict";
import { registerHooks } from "node:module";
import { beforeEach, afterEach, test } from "node:test";

registerHooks({
  resolve(specifier, context, nextResolve) {
    if (/^\.\.?\//.test(specifier) && !/\.[a-z]+$/i.test(specifier))
      return nextResolve(`${specifier}.ts`, context);
    return nextResolve(specifier, context);
  },
  load(url, context, nextLoad) {
    if (url.endsWith(".wav"))
      return {
        format: "module",
        source: 'export default "test-tone.wav";',
        shortCircuit: true,
      };
    return nextLoad(url, context);
  },
});

const {
  readDevicePreferences,
  saveDevicePreferences,
  subscribeDevicePreferences,
} = await import("../src/prejoinDevices.ts");
const { MicrophoneGain } = await import("../src/microphoneGain.ts");
const { playDeviceTone } = await import("../src/deviceSound.ts");
const { createNotificationSound } = await import("../src/notificationSound.ts");

const originals = new Map();
const audios = [];
const contexts = [];
function replace(name, value) {
  originals.set(name, Object.getOwnPropertyDescriptor(globalThis, name));
  Object.defineProperty(globalThis, name, { value, configurable: true });
}

class Track extends EventTarget {
  kind = "audio";
  readyState = "live";
  getSettings() {
    return { channelCount: 1 };
  }
  stop() {
    this.readyState = "ended";
  }
}
class Stream {
  constructor(tracks) {
    this.tracks = tracks;
  }
  getAudioTracks() {
    return this.tracks;
  }
}
class AudioContextMock extends EventTarget {
  state = "running";
  gain = { gain: { value: 1 }, connect() {}, disconnect() {} };
  closed = 0;
  constructor() {
    super();
    contexts.push(this);
  }
  resume() {
    return Promise.resolve();
  }
  close() {
    this.closed++;
    this.state = "closed";
    return Promise.resolve();
  }
  createMediaStreamSource() {
    return { connect() {}, disconnect() {} };
  }
  createGain() {
    return this.gain;
  }
}
class AudioMock {
  volume = 1;
  paused = true;
  currentTime = 0;
  sinkId = "";
  loads = 0;
  constructor() {
    audios.push(this);
  }
  async play() {
    this.paused = false;
  }
  pause() {
    this.paused = true;
  }
  async setSinkId(id) {
    this.sinkId = id;
  }
  removeAttribute() {}
  load() {
    this.loads++;
  }
}

beforeEach(() => {
  audios.length = 0;
  contexts.length = 0;
  const storage = new Map();
  replace("window", new EventTarget());
  replace("localStorage", {
    getItem: (key) => storage.get(key) ?? null,
    setItem: (key, value) => storage.set(key, value),
  });
  replace("navigator", {});
  replace("Audio", AudioMock);
  replace("AudioContext", AudioContextMock);
  replace("MediaStream", Stream);
  replace(
    "MediaStreamAudioDestinationNode",
    class {
      stream = new Stream([new Track()]);
    },
  );
});
afterEach(() => {
  for (const [name, descriptor] of originals) {
    if (descriptor) Object.defineProperty(globalThis, name, descriptor);
    else delete globalThis[name];
  }
  originals.clear();
});

test("настройки без уровней получают 85/85/100%; изменения изолированы по аккаунту", () => {
  localStorage.setItem(
    "meet.devices.v1:alice",
    JSON.stringify({ noiseSuppression: false }),
  );
  const legacy = readDevicePreferences("alice");
  assert.deepEqual(
    [legacy.microphoneVolume, legacy.speakerVolume, legacy.signalVolume],
    [0.85, 0.85, 1],
  );
  const updates = [];
  const unsubscribe = subscribeDevicePreferences("alice", (value) =>
    updates.push(value),
  );
  saveDevicePreferences("bob", {
    ...readDevicePreferences("bob"),
    speakerVolume: 0.2,
  });
  assert.equal(updates.length, 0);
  saveDevicePreferences("alice", {
    ...legacy,
    microphoneVolume: 0,
    signalVolume: 0.4,
  });
  assert.equal(updates.length, 1);
  assert.equal(readDevicePreferences("alice").microphoneVolume, 0);
  assert.equal(readDevicePreferences("alice").noiseSuppression, false);
  assert.equal(readDevicePreferences("alice").signalVolume, 0.4);
  assert.equal(readDevicePreferences("bob").speakerVolume, 0.2);
  unsubscribe();
  saveDevicePreferences("alice", legacy);
  assert.equal(updates.length, 1);
});

test("раздельные сигналы объединяются без потери отключённого звука и пользовательских уровней", () => {
  for (const [stored, expected] of [
    [{ notificationVolume: 0.35, ringtoneVolume: 0.75 }, 1],
    [{ notificationVolume: 0, ringtoneVolume: 0.75 }, 0],
    [{ notificationVolume: 0.35, ringtoneVolume: 0.4 }, 0.4],
    [{ notificationVolume: 0.6, ringtoneVolume: 0.2 }, 0.2],
    [{ signalVolume: 0, notificationVolume: 0.6, ringtoneVolume: 0.7 }, 0],
  ]) {
    localStorage.setItem("meet.devices.v1:alice", JSON.stringify(stored));
    const preferences = readDevicePreferences("alice");
    assert.equal(preferences.signalVolume, expected);
    assert.equal(Object.hasOwn(preferences, "notificationVolume"), false);
    assert.equal(Object.hasOwn(preferences, "ringtoneVolume"), false);
    saveDevicePreferences("alice", preferences);
    assert.equal(readDevicePreferences("alice").signalVolume, expected);
  }
});

test("повреждённые уровни не попадают в AudioParam и HTMLMediaElement", () => {
  localStorage.setItem(
    "meet.devices.v1:alice",
    JSON.stringify({
      microphoneVolume: "80",
      speakerVolume: -10,
      signalVolume: 20,
    }),
  );
  const preferences = readDevicePreferences("alice");
  assert.equal(preferences.microphoneVolume, 0.85);
  assert.equal(preferences.speakerVolume, 0);
  assert.equal(preferences.signalVolume, 1);
  saveDevicePreferences("alice", {
    ...preferences,
    microphoneVolume: NaN,
    speakerVolume: Infinity,
  });
  assert.equal(readDevicePreferences("alice").microphoneVolume, 0.85);
  assert.equal(readDevicePreferences("alice").speakerVolume, 0.85);
});

test("уровень микрофона меняется на существующей дорожке и освобождает захват при закрытии", async () => {
  const input = new Track();
  const gain = new MicrophoneGain(0.85);
  const output = await gain.attach(input);
  assert.equal(contexts[0].gain.gain.value, 0.85);
  gain.setVolume(0);
  assert.equal(contexts[0].gain.gain.value, 0);
  gain.setVolume(0.61);
  assert.equal(contexts[0].gain.gain.value, 0.61);
  assert.equal(gain.outputTrack, output);
  assert.equal(input.readyState, "live");
  assert.equal(output.readyState, "live");
  assert.equal(contexts.length, 1);
  gain.dispose();
  gain.dispose();
  assert.equal(input.readyState, "ended");
  assert.equal(output.readyState, "ended");
  assert.equal(contexts[0].closed, 1);
});

test("проверочный сигнал меняет громкость на ходу и отменяется до позднего play", async () => {
  const stop = playDeviceTone("speakers", () => {}, 0.31);
  assert.equal(audios[0].volume, 0.31);
  stop.setVolume(0);
  assert.equal(audios[0].volume, 0);
  stop();
  stop();
  await new Promise(setImmediate);
  assert.equal(audios[0].paused, true);
  assert.equal(audios[0].loads, 1);
});

test("уведомления используют сохранённый уровень, обновляются и снимают подписку при закрытии", async () => {
  saveDevicePreferences("alice", {
    ...readDevicePreferences("alice"),
    signalVolume: 0.27,
  });
  const sound = createNotificationSound("alice");
  sound.notify("synthetic-notification");
  await new Promise(setImmediate);
  assert.equal(audios[0].volume, 0.27);
  assert.equal(audios[0].paused, false);
  saveDevicePreferences("bob", {
    ...readDevicePreferences("bob"),
    signalVolume: 0.9,
  });
  assert.equal(audios[0].volume, 0.27);
  saveDevicePreferences("alice", {
    ...readDevicePreferences("alice"),
    signalVolume: 0,
  });
  assert.equal(audios[0].volume, 0);
  sound.dispose();
  saveDevicePreferences("alice", {
    ...readDevicePreferences("alice"),
    signalVolume: 1,
  });
  assert.equal(audios[0].volume, 0);
  assert.equal(audios[0].paused, true);
});
