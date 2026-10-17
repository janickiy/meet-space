# Microphone attenuation checks

Use a supported Node.js version from `../package.json` (Node.js 24 recommended).
From `frontend/`, run:

```sh
npm run test:microphone
npm run typecheck
npm run build
```

`test:microphone` loads the production TypeScript sources with Node's type
transformation in a temporary directory. It does not modify `src/`, require
microphone permissions, open a network connection, or record audio.

The lifecycle regression covers processed publication, unchanged camera tracks,
constraints on the physical microphone, device replacement (success and failure),
mute and re-enable, moderation, hardware end, AudioContext suspension, permission
errors and late permission results after stop. It also checks that processing
failures never publish the original unattenuated microphone. All owned tracks,
audio nodes and contexts must be released.

These tests use browser API doubles. They cannot establish the actual audio
amplitude or hardware behavior. Run the separate browser regression to measure
real Web Audio PCM and check the client startup/shutdown paths:

```sh
node scripts/check-microphone-gain-browser.mjs /tmp/meetspace-microphone-result.json
```

The optional argument writes a JSON result. A failed check exits with code 1.
Chrome/Chromium must be installed; set `CHROME_BIN` if it is not at a standard macOS
or Linux path. The runner starts and stops its own localhost Vite server and a
headless browser with a temporary profile, using the existing frontend dependencies.
It never attaches to an existing browser or requests physical microphones/cameras.

Synthetic 48 kHz mono/stereo oscillators pass through the actual
`MicrophoneGain.attach()` implementation. Simultaneous RMS measurements must have
an amplitude ratio of `0.8` (about `-1.94 dB`), allowing for asynchronous audio
buffering. The test also checks channel counts, track/context cleanup, simulated
capture ending, device replacement, processing errors, receive-only startup, and
stop during a pending capture request. No server or real meeting is used.

For headless rendering, test AudioContexts use Chromium's silent
`sinkId: {type: "none"}` to advance real Web Audio clocks without audio hardware.
No node connects to `AudioContext.destination`; no audio plays through speakers.
This proves client PCM attenuation, not browser AGC behavior, device acoustics,
perceived loudness, or complete two-device call behavior.

For a real call, compare the same physical microphone and system input level
before and after the change. Verify meetings and direct calls, mute and unmute,
device changes, reconnects, and closing the page. Browser AGC, noise suppression,
microphone hardware, Opus encoding and receiver playback can affect a live
measurement; the constant gain is applied after browser capture processing.
