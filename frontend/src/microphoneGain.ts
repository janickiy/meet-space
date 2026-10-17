/** Applied after browser capture processing and before WebRTC encoding. */
export const MICROPHONE_GAIN = 0.8;

export class MicrophoneGain {
  private readonly context: AudioContext;
  private source: MediaStreamAudioSourceNode | null = null;
  private gain: GainNode | null = null;
  private destination: MediaStreamAudioDestinationNode | null = null;
  private input: MediaStreamTrack | null = null;
  private output: MediaStreamTrack | null = null;
  private disposed = false;
  private attached = false;
  onEnded: (() => void) | null = null;

  constructor() {
    if (typeof AudioContext === "undefined")
      throw new Error("microphone_processing");
    this.context = new AudioContext();
    // Start inside the capture action, before the permission prompt can consume activation.
    void this.resume();
    this.context.addEventListener("statechange", this.contextChanged);
  }

  get inputTrack() {
    return this.input;
  }

  get outputTrack() {
    return this.output;
  }

  get contextState() {
    return this.context.state;
  }

  private resume(): Promise<boolean> {
    try {
      return this.context.resume().then(
        () => true,
        () => false,
      );
    } catch {
      return Promise.resolve(false);
    }
  }

  async attach(input: MediaStreamTrack): Promise<MediaStreamTrack> {
    if (this.disposed || this.input || input.kind !== "audio") {
      input.stop();
      this.dispose();
      throw new Error("microphone_processing");
    }
    this.input = input;
    input.addEventListener("ended", this.inputEnded);
    try {
      this.source = this.context.createMediaStreamSource(
        new MediaStream([input]),
      );
      this.gain = this.context.createGain();
      this.gain.gain.value = MICROPHONE_GAIN;
      const channels = input.getSettings().channelCount;
      this.destination = new MediaStreamAudioDestinationNode(this.context, {
        channelCount: channels === 1 ? 1 : 2,
      });
      this.source.connect(this.gain);
      this.gain.connect(this.destination);
      this.output = this.destination.stream.getAudioTracks()[0] || null;
      let timer: ReturnType<typeof setTimeout> | undefined;
      let running: boolean;
      try {
        running = await Promise.race([
          this.context.state === "running"
            ? Promise.resolve(true)
            : this.resume(),
          new Promise<boolean>((resolve) => {
            timer = setTimeout(() => resolve(false), 3000);
          }),
        ]);
      } finally {
        clearTimeout(timer);
      }
      if (
        !running ||
        this.disposed ||
        this.context.state !== "running" ||
        input.readyState !== "live" ||
        !this.output ||
        this.output.readyState !== "live"
      )
        throw new Error("microphone_processing");
      this.output.contentHint = "speech";
      this.attached = true;
      return this.output;
    } catch {
      this.dispose();
      throw new Error("microphone_processing");
    }
  }

  private inputEnded = () => {
    const ended = this.onEnded;
    this.dispose();
    ended?.();
  };

  private contextChanged = () => {
    if (this.attached && !this.disposed && this.context.state !== "running")
      this.inputEnded();
  };

  dispose() {
    if (this.disposed) return;
    this.disposed = true;
    this.onEnded = null;
    this.context.removeEventListener("statechange", this.contextChanged);
    this.input?.removeEventListener("ended", this.inputEnded);
    this.input?.stop();
    this.output?.stop();
    this.source?.disconnect();
    this.gain?.disconnect();
    this.source = null;
    this.gain = null;
    this.destination = null;
    void this.context.close().catch(() => {});
  }
}
