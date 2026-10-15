// Chromium command-line switches for the isola Electron sandbox.
//
// Kept in its own module, away from main.js, because the invariants here are
// load-bearing and otherwise only live in comments: one flag in this list was
// the root cause of a class of frozen sessions (`--disable-dev-shm-usage`,
// see below), and test/chrome_flags.test.js now asserts the parts that must
// stay true.

// CHROME_FLAGS lists [name, value] pairs applied through
// app.commandLine.appendSwitch before app.whenReady(). `value: undefined` means
// a bare switch.
const CHROME_FLAGS = [
  ['remote-debugging-port', '9222'],
  ['remote-debugging-address', '127.0.0.1'],
  ['no-sandbox'],
  // NOT --disable-dev-shm-usage.
  //
  // That flag is the standard Docker workaround for a 64 MiB /dev/shm, and it
  // works by moving Chromium's shared memory into /tmp. Here /tmp is a 256 MiB
  // tmpfs that also carries the Electron profile, and the offscreen capture's
  // copy-output bitmaps are 7.8 MiB each at 1920x1018. Measured with the flag
  // set: /tmp at 94% (239 MiB of unlinked shm, invisible to `du`), ~10 empty
  // captures per second, and a viewer frozen at ~1 fps on a page whose renderer
  // was answering CDP in 3 ms. The orchestrator sizes /dev/shm for exactly this
  // (shmBudgetMB in backend/pkg/orchestrator/orchestrator.go), so shared memory
  // belongs there — adding the flag back re-creates the freeze.
  ['in-process-gpu'],
  ['enable-features', 'UseSkiaRenderer'],
  ['disable-gpu-sandbox'],
  // Video capture in offscreen mode: without these, video elements use a
  // separate GPU compositing layer that bypasses the paint event output —
  // they appear black in the stream. Force software video compositing.
  ['disable-accelerated-video-decode'],
  ['disable-accelerated-video-encode'],
  ['disable-features', 'VideoCaptureUseGpuMemoryBuffer'],
  // WebGL on a CPU-only host: Chromium removed the automatic software fallback
  // and asks for this opt-in explicitly. It relaxes GPU-process isolation, which
  // is acceptable inside the disposable sandbox and nowhere else. Without it a
  // WebGL page (a maps/radar page, say) can end up with no working GL path.
  ['enable-unsafe-swiftshader'],
  // Allow autoplay: the offscreen window has no real user gesture,
  // so videos would never start without this.
  ['autoplay-policy', 'no-user-gesture-required'],
];

// applyChromeFlags pushes every switch onto an Electron `app.commandLine`.
function applyChromeFlags(commandLine) {
  for (const [name, value] of CHROME_FLAGS) {
    if (value === undefined) commandLine.appendSwitch(name);
    else commandLine.appendSwitch(name, value);
  }
}

module.exports = { CHROME_FLAGS, applyChromeFlags };
