import assert from "node:assert/strict";
import { registerHooks } from "node:module";
import { beforeEach, afterEach, test } from "node:test";

// The application resolves extensionless TS imports through Vite; keep the test
// loader local to these modules, without adding a second compiler dependency.
registerHooks({
  resolve(specifier, context, nextResolve) {
    if (/^\.\.?\//.test(specifier) && !/\.[a-z]+$/i.test(specifier))
      return nextResolve(`${specifier}.ts`, context);
    return nextResolve(specifier, context);
  },
});
const { ConferenceMediaClient } = await import("../src/media.ts");
const original = new Map();
const tracks = [];
const peers = [];

class Track extends EventTarget {
  kind = "video";
  id = crypto.randomUUID();
  readyState = "live";
  enabled = true;
  onended = null;
  constructor() {
    super();
    tracks.push(this);
  }
  stop() {
    this.readyState = "ended";
  }
  async applyConstraints() {}
}
class Stream {
  constructor(tracks = []) {
    this.tracks = [...tracks];
  }
  getTracks() {
    return [...this.tracks];
  }
  getAudioTracks() {
    return this.tracks.filter((track) => track.kind === "audio");
  }
  getVideoTracks() {
    return this.tracks.filter((track) => track.kind === "video");
  }
  addTrack(track) {
    if (!this.tracks.includes(track)) this.tracks.push(track);
  }
  removeTrack(track) {
    this.tracks = this.tracks.filter((item) => item !== track);
  }
}
class Peer {
  signalingState = "stable";
  connectionState = "new";
  transceivers = [];
  closed = false;
  constructor() {
    peers.push(this);
  }
  addTransceiver(track, options) {
    const item = {
      mid: String(this.transceivers.length),
      direction: options.direction,
      sender: { replaceTrack: async () => {} },
      receiver: {
        track: { kind: typeof track === "string" ? track : track.kind },
      },
    };
    this.transceivers.push(item);
    return item;
  }
  getTransceivers() {
    return this.transceivers;
  }
  async createOffer() {
    return { type: "offer", sdp: "synthetic SDP" };
  }
  async setLocalDescription(description) {
    this.localDescription = description;
  }
  close() {
    this.closed = true;
    this.connectionState = "closed";
  }
}
function replaceGlobal(name, value) {
  original.set(name, Object.getOwnPropertyDescriptor(globalThis, name));
  Object.defineProperty(globalThis, name, { configurable: true, value });
}
beforeEach(() => {
  tracks.length = 0;
  peers.length = 0;
  replaceGlobal("MediaStream", Stream);
  replaceGlobal("RTCPeerConnection", Peer);
  replaceGlobal("RTCRtpReceiver", {});
  replaceGlobal("navigator", {
    mediaDevices: { getUserMedia: async () => new Stream([new Track()]) },
  });
});
afterEach(() => {
  for (const [name, descriptor] of original) {
    if (descriptor) Object.defineProperty(globalThis, name, descriptor);
    else delete globalThis[name];
  }
  original.clear();
});

function client() {
  const sent = [];
  return {
    media: new ConferenceMediaClient(
      (type, data) => {
        const id = crypto.randomUUID();
        sent.push({ id, type, data });
        return id;
      },
      () => {},
    ),
    sent,
  };
}
async function join(media, sent) {
  media.handle({
    version: 1,
    id: crypto.randomUUID(),
    type: "media.joined",
    replyTo: sent.find((item) => item.type === "media.join").id,
    data: {
      mediaPeerId: crypto.randomUUID(),
      workerId: "synthetic-worker",
      maxPeers: 2,
      tracks: [],
      iceServers: [],
    },
  });
  await media.queue;
}

test("100 camera join/leave cycles release all owned tracks and peers", async () => {
  for (let cycle = 0; cycle < 100; cycle++) {
    const { media, sent } = client();
    try {
      await media.start(true, { microphoneEnabled: false });
      await join(media, sent);
      assert.equal(media.snapshot().cameraEnabled, true);
    } finally {
      media.stop();
    }
    assert.equal(media.snapshot().active, false);
    assert.equal(media.snapshot().localStream, null);
    assert.deepEqual(media.snapshot().remoteStreams, []);
    assert.equal(
      tracks.filter((track) => track.readyState === "live").length,
      0,
    );
    assert.equal(peers.filter((peer) => !peer.closed).length, 0);
  }
  assert.equal(tracks.length, 100);
  assert.equal(peers.length, 100);
});

test("capture that resolves after stop does not leave the camera running", async () => {
  let resolveCapture;
  navigator.mediaDevices.getUserMedia = () =>
    new Promise((resolve) => {
      resolveCapture = resolve;
    });
  const { media, sent } = client();
  const start = media.start(true, { microphoneEnabled: false });
  media.stop();
  const track = new Track();
  resolveCapture(new Stream([track]));
  await start;
  assert.equal(track.readyState, "ended");
  assert.equal(sent.filter((item) => item.type === "media.join").length, 0);
  assert.equal(media.snapshot().localStream, null);
});

test("late camera constraints cannot recreate a stopped PeerConnection", async () => {
  const { media, sent } = client();
  await media.start(true, { microphoneEnabled: false });
  let release;
  tracks[0].applyConstraints = () =>
    new Promise((resolve) => {
      release = resolve;
    });
  const joined = join(media, sent);
  await Promise.resolve();
  media.stop();
  release();
  await joined;
  assert.equal(peers.length, 0);
  assert.equal(tracks[0].readyState, "ended");
  assert.equal(media.snapshot().active, false);
});

test("screen capture resolving after leave is stopped without publication", async () => {
  const { media, sent } = client();
  await media.start(false);
  await join(media, sent);
  let resolveScreen;
  navigator.mediaDevices.getDisplayMedia = () =>
    new Promise((resolve) => {
      resolveScreen = resolve;
    });
  const pending = media.startScreen();
  media.stop();
  const screen = new Track();
  resolveScreen(new Stream([screen]));
  await pending;
  assert.equal(screen.readyState, "ended");
  assert.equal(media.snapshot().localScreen, null);
  assert.equal(peers[0].closed, true);
});

test("joining without devices never asks for camera or microphone access", async () => {
  let captures = 0;
  navigator.mediaDevices.getUserMedia = async () => {
    captures++;
    throw new Error("unexpected capture");
  };
  const { media, sent } = client();
  try {
    await media.start(false);
    await join(media, sent);
    assert.equal(media.snapshot().active, true);
    assert.equal(captures, 0);
    assert.equal(media.snapshot().localStream, null);
  } finally {
    media.stop();
  }
});
