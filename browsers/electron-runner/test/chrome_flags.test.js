const { test } = require('node:test');
const assert = require('node:assert');
const { CHROME_FLAGS, applyChromeFlags } = require('../src/chrome_flags');

const names = CHROME_FLAGS.map(([name]) => name);

test('CDP listens on loopback only', () => {
  const addr = CHROME_FLAGS.find(([name]) => name === 'remote-debugging-address');
  assert.ok(addr, 'remote-debugging-address must be set');
  assert.equal(addr[1], '127.0.0.1');
});

test('shared memory stays in /dev/shm', () => {
  // The regression guard for frozen sessions: this flag moves Chromium's shared
  // memory into the 256 MiB /tmp tmpfs, where the capture's copy-output
  // bitmaps could not be allocated and every frame came back empty.
  assert.ok(
    !names.includes('disable-dev-shm-usage'),
    '--disable-dev-shm-usage must not be set: /dev/shm is sized for the capture, /tmp is not',
  );
  assert.ok(names.includes('in-process-gpu'), 'the rest of the sandbox tuning assumes in-process GPU');
});

test('software rendering opt-ins are present', () => {
  // Both were added for measured failures: WebGL pages with no usable context,
  // and black video elements bypassing the paint event.
  assert.ok(names.includes('enable-unsafe-swiftshader'));
  assert.ok(names.includes('disable-accelerated-video-decode'));
  assert.ok(names.includes('disable-accelerated-video-encode'));
});

test('flag table is well formed', () => {
  for (const entry of CHROME_FLAGS) {
    // [name] is a bare switch, [name, value] carries a value.
    assert.ok(Array.isArray(entry) && (entry.length === 1 || entry.length === 2), `bad entry: ${JSON.stringify(entry)}`);
    const [name, value] = entry;
    assert.equal(typeof name, 'string');
    assert.ok(name.length > 0 && !name.startsWith('-'), `switch names are unprefixed: ${name}`);
    assert.ok(value === undefined || typeof value === 'string', `bad value for ${name}: ${value}`);
  }
  assert.equal(new Set(names).size, names.length, 'duplicate switch names would be applied twice');
});

test('applyChromeFlags forwards every switch once, in order', () => {
  const calls = [];
  applyChromeFlags({ appendSwitch: (name, value) => calls.push([name, value]) });
  assert.equal(calls.length, CHROME_FLAGS.length);
  assert.deepEqual(calls, CHROME_FLAGS.map(([name, value]) => [name, value]));
});
