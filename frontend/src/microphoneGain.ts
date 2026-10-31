import { DEFAULT_AUDIO_VOLUMES, normalizeAudioVolume } from "./audioVolume";

export class MicrophoneGain {
  private readonly context: AudioContext;
  private source: MediaStreamAudioSourceNode | null = null;
  private gain: GainNode | null = null;
  private destination: MediaStreamAudioDestinationNode | null = null;
  private input: MediaStreamTrack | null = null;
  private output: MediaStreamTrack | null = null;
  private disposed = false;
  private attached = false;
  private volume: number;
  onEnded: (() => void) | null = null;

  constructor(volume: number = DEFAULT_AUDIO_VOLUMES.microphoneVolume) {
    this.volume = normalizeAudioVolume(
      volume,
      DEFAULT_AUDIO_VOLUMES.microphoneVolume,
    );
    if (typeof AudioContext === "undefined")
      throw new Error("microphone_processing");
    this.context = new AudioContext();
    // Начинается внутри действия по захвату данных, до того как запрос на предоставление
    // разрешений может привести к активации.
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

  /** Меняет уровень исходящего звука, сохраняя захват и опубликованную дорожку. */
  setVolume(volume: number) {
    this.volume = normalizeAudioVolume(
      volume,
      DEFAULT_AUDIO_VOLUMES.microphoneVolume,
    );
    if (this.gain) this.gain.gain.value = this.volume;
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
      this.gain.gain.value = this.volume;
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
