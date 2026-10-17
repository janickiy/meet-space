import assert from "node:assert/strict";
import { readFile, writeFile, mkdtemp } from "node:fs/promises";
import { rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

const root = new URL("../", import.meta.url);
const temporary = await mkdtemp(join(tmpdir(), "meetspace-microphone-"));
const compiled = pathToFileURL(`${temporary}/`);
process.once("exit", () => rmSync(temporary, { recursive: true, force: true }));
for (const file of ["media", "mediaDiagnostics", "microphoneGain"]) {
  const source = await readFile(new URL(`./src/${file}.ts`, root), "utf8");
  await writeFile(
    new URL(`./${file}.ts`, compiled),
    source
      .replace('from "./microphoneGain"', 'from "./microphoneGain.ts"')
      .replace('from "./mediaDiagnostics"', 'from "./mediaDiagnostics.ts"'),
  );
}

let id = 0;
class Track extends EventTarget {
  constructor(kind = "audio") {
    super();
    this.kind = kind;
    this.id = `track-${++id}`;
    this.readyState = "live";
    this.contentHint = "";
    this.onended = null;
    this.constraints = [];
  }
  stop() {
    this.readyState = "ended";
  }
  end() {
    this.stop();
    const event = new Event("ended");
    this.dispatchEvent(event);
    this.onended?.(event);
  }
  async applyConstraints(value) {
    this.constraints.push(value);
  }
  getSettings() {
    return this.kind === "audio" ? { channelCount: 1 } : {};
  }
}
class Stream {
  constructor(tracks = []) {
    this.tracks = [...tracks];
  }
  getTracks() {
    return [...this.tracks];
  }
  getAudioTracks() {
    return this.tracks.filter((t) => t.kind === "audio");
  }
  addTrack(t) {
    if (!this.tracks.includes(t)) this.tracks.push(t);
  }
  removeTrack(t) {
    this.tracks = this.tracks.filter((x) => x !== t);
  }
}
const contexts = [];
class Context extends EventTarget {
  static next = "running";
  constructor() {
    super();
    this.state = Context.next;
    Context.next = "running";
    this.nodes = [];
    contexts.push(this);
  }
  resume() {
    return this.state === "rejected"
      ? Promise.reject(new Error("blocked"))
      : this.state === "pending"
        ? new Promise(() => {})
        : Promise.resolve();
  }
  createNode(type) {
    const node = {
      type,
      disconnected: false,
      gain: { value: 1 },
      connect(to) {
        this.to = to;
      },
      disconnect() {
        this.disconnected = true;
      },
    };
    this.nodes.push(node);
    return node;
  }
  createMediaStreamSource(stream) {
    this.input = stream.getAudioTracks()[0];
    return this.createNode("source");
  }
  createGain() {
    return this.createNode("gain");
  }
  createMediaStreamDestination() {
    const node = this.createNode("destination");
    node.stream = new Stream([new Track()]);
    return node;
  }
  close() {
    this.state = "closed";
    this.dispatchEvent(new Event("statechange"));
    return Promise.resolve();
  }
  suspend() {
    this.state = "suspended";
    this.dispatchEvent(new Event("statechange"));
  }
}
class Peer {
  constructor() {
    this.transceivers = [];
  }
  addTransceiver(trackOrKind) {
    const kind =
      typeof trackOrKind === "string" ? trackOrKind : trackOrKind.kind;
    const sender = {
      track: typeof trackOrKind === "string" ? null : trackOrKind,
      replaceTrack: async (track) => {
        if (this.rejectNext) {
          this.rejectNext = false;
          throw new Error("replace failed");
        }
        sender.track = track;
      },
    };
    const item = {
      sender,
      receiver: { track: new Track(kind) },
      direction: "sendrecv",
    };
    this.transceivers.push(item);
    return item;
  }
  getTransceivers() {
    return this.transceivers;
  }
  close() {
    this.closed = true;
  }
}
globalThis.MediaStream = Stream;
globalThis.AudioContext = Context;
globalThis.MediaStreamAudioDestinationNode = class {
  constructor(context, options) {
    const node = context.createMediaStreamDestination();
    node.channelCount = options.channelCount;
    return node;
  }
};
globalThis.RTCPeerConnection = Peer;
globalThis.RTCRtpReceiver = { getCapabilities: () => undefined };
let capture;
Object.defineProperty(globalThis, "navigator", {
  value: {
    mediaDevices: { getUserMedia: (constraints) => capture(constraints) },
  },
  configurable: true,
});
const { ConferenceMediaClient } = await import(
  pathToFileURL(new URL("./media.ts", compiled).pathname)
);
let passed = 0;
async function test(name, check) {
  await check();
  passed++;
  console.log(`PASS ${name}`);
}
function client() {
  return new ConferenceMediaClient(
    () => `request-${++id}`,
    () => {},
  );
}
function assertClosed(context, raw, processed) {
  assert.equal(raw.readyState, "ended");
  assert.equal(processed?.readyState ?? "ended", "ended");
  assert.equal(context.state, "closed");
  assert(
    context.nodes
      .filter((n) => n.type !== "destination")
      .every((n) => n.disconnected),
  );
}
async function started(camera = false) {
  const raw = new Track();
  const video = camera ? new Track("video") : null;
  capture = async () => new Stream([raw, ...(video ? [video] : [])]);
  const instance = client();
  await instance.start(true, { cameraEnabled: camera });
  instance.createPeer([], 2);
  instance.offer = async () => {};
  return {
    instance,
    raw,
    video,
    processed: instance.sources.get("microphone").track,
    context: contexts.at(-1),
  };
}

await test("initial publication uses only processed microphone; camera untouched; complete stop", async () => {
  const { instance, raw, video, processed, context } = await started(true);
  assert.notEqual(processed, raw);
  assert.equal(video.readyState, "live");
  assert(!instance.local.getTracks().includes(raw));
  assert(instance.local.getTracks().includes(processed));
  assert.equal(context.nodes.find((n) => n.type === "gain").gain.value, 0.8);
  assert.equal(
    context.nodes.find((n) => n.type === "destination").channelCount,
    1,
  );
  assert.equal(instance.sources.get("camera").track, video);
  instance.stop();
  instance.stop();
  assertClosed(context, raw, processed);
  assert.equal(video.readyState, "ended");
});
await test("changing settings applies microphone constraints to raw capture", async () => {
  const { instance, raw } = await started();
  instance.configure({ noiseSuppression: false });
  assert.deepEqual(raw.constraints, [{ noiseSuppression: false }]);
  instance.stop();
});
await test("device switch replaces processed source and releases previous graph", async () => {
  const previous = await started();
  const raw = new Track();
  capture = async () => new Stream([raw]);
  await previous.instance.changeSource("microphone", true, "usb");
  const processed = previous.instance.sources.get("microphone").track;
  assert.notEqual(processed, raw);
  assert.notEqual(processed, previous.processed);
  assertClosed(previous.context, previous.raw, previous.processed);
  assert.equal(raw.readyState, "live");
  previous.instance.stop();
  assertClosed(contexts.at(-1), raw, processed);
});
await test("failed replace preserves previous live microphone and closes new graph", async () => {
  const previous = await started();
  const raw = new Track();
  capture = async () => new Stream([raw]);
  previous.instance.pc.rejectNext = true;
  await previous.instance.changeSource("microphone", true, "usb");
  assert.equal(
    previous.instance.sources.get("microphone").track,
    previous.processed,
  );
  assert.equal(previous.raw.readyState, "live");
  assertClosed(contexts.at(-1), raw);
  previous.instance.stop();
});
await test("mute and re-enable create a fresh attenuated microphone", async () => {
  const previous = await started();
  await previous.instance.changeSource("microphone", false);
  assert.equal(previous.instance.sources.get("microphone").track, null);
  assertClosed(previous.context, previous.raw, previous.processed);
  const raw = new Track();
  capture = async () => new Stream([raw]);
  await previous.instance.changeSource("microphone", true);
  const output = previous.instance.sources.get("microphone").track;
  assert.notEqual(output, raw);
  assert.equal(raw.readyState, "live");
  previous.instance.stop();
});
await test("moderator block releases microphone input and processed output", async () => {
  const previous = await started();
  previous.instance.setPolicy({ version: 1, microphoneBlocked: true });
  await previous.instance.queue;
  assert.equal(previous.instance.sources.get("microphone").track, null);
  assertClosed(previous.context, previous.raw, previous.processed);
  previous.instance.stop();
});
for (const event of ["raw-ended", "context-suspended"]) {
  await test(`${event} withdraws the publication and releases all capture resources`, async () => {
    const previous = await started();
    if (event === "raw-ended") previous.raw.end();
    else previous.context.suspend();
    await previous.instance.queue;
    assert.equal(previous.instance.sources.get("microphone").track, null);
    assertClosed(previous.context, previous.raw, previous.processed);
    assert.equal(previous.instance.snapshot().microphoneEnabled, false);
    previous.instance.stop();
  });
}
await test("stop during permission prompt closes prepared context and late returned tracks", async () => {
  let resolve;
  capture = () =>
    new Promise((done) => {
      resolve = done;
    });
  const instance = client();
  const pending = instance.start();
  const context = contexts.at(-1);
  instance.stop();
  const raw = new Track();
  const video = new Track("video");
  resolve(new Stream([raw, video]));
  await pending;
  assertClosed(context, raw);
  assert.equal(video.readyState, "ended");
  assert.equal(instance.snapshot().active, false);
});
await test("permission error releases prepared graph without publication", async () => {
  capture = async () => {
    throw new DOMException("denied", "NotAllowedError");
  };
  const instance = client();
  await instance.start();
  assert.equal(contexts.at(-1).state, "closed");
  assert.equal(instance.snapshot().active, false);
  assert.equal(instance.snapshot().microphoneEnabled, false);
});
await test("processing failure never publishes unattenuated capture", async () => {
  Context.next = "rejected";
  const raw = new Track();
  const video = new Track("video");
  capture = async () => new Stream([raw, video]);
  const instance = client();
  await instance.start();
  assertClosed(contexts.at(-1), raw);
  assert.equal(video.readyState, "ended");
  assert.equal(instance.snapshot().active, false);
  assert.equal(instance.snapshot().microphoneEnabled, false);
  assert.match(instance.snapshot().error, /подготовить звук/);
});
await test("device switch loses race to moderator block without leaking new microphone", async () => {
  const previous = await started();
  let resolve;
  capture = () =>
    new Promise((done) => {
      resolve = done;
    });
  const pending = previous.instance.changeSource("microphone", true, "usb");
  previous.instance.setPolicy({ version: 1, microphoneBlocked: true });
  const raw = new Track();
  resolve(new Stream([raw]));
  await pending;
  await previous.instance.queue;
  assert.equal(raw.readyState, "ended");
  assert.equal(contexts.at(-1).state, "closed");
  assert.equal(previous.instance.sources.get("microphone").track, null);
  previous.instance.stop();
});
await test("ending capture while signaling is unavailable still clears the local microphone", async () => {
  const previous = await started();
  previous.instance.ownPublished.set("publication", {
    id: "publication",
    source: "microphone",
  });
  previous.instance.send = () => {
    throw new Error("socket closed");
  };
  previous.raw.end();
  await previous.instance.queue;
  assert.equal(previous.instance.sources.get("microphone").track, null);
  assert.equal(previous.instance.snapshot().microphoneEnabled, false);
  assertClosed(previous.context, previous.raw, previous.processed);
  previous.instance.stop();
});
assert(contexts.every((c) => c.state === "closed"));
console.log(
  JSON.stringify({
    passed,
    contexts: contexts.length,
    allContextsClosed: true,
  }),
);
