const test = require('node:test');
const assert = require('node:assert');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const { hardenCtrlSocket } = require('../src/ctrl_socket');

function makeFile() {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'ctrl-sock-'));
  const file = path.join(dir, 'ctrl.sock');
  fs.writeFileSync(file, '');
  fs.chmodSync(file, 0o666); // world-writable, as Node creates a socket by default
  return { dir, file };
}

function cleanup(dir, file) {
  fs.rmSync(dir, { recursive: true, force: true });
  fs.rmSync(file, { force: true });
}

test('drops world-writable mode and chowns to the supplied uid/gid', () => {
  const { dir, file } = makeFile();
  try {
    hardenCtrlSocket(file, {
      ISOLA_CTRL_SOCK_UID: String(process.getuid()),
      ISOLA_CTRL_SOCK_GID: String(process.getgid()),
    });
    const st = fs.statSync(file);
    assert.strictEqual(st.mode & 0o777, 0o660, 'socket must be 0o660');
    assert.strictEqual(st.uid, process.getuid(), 'socket owner must be the supplied uid');
    assert.strictEqual(st.gid, process.getgid(), 'socket group must be the supplied gid');
  } finally {
    cleanup(dir, file);
  }
});

test('still tightens the mode when no uid/gid is provided', () => {
  const { dir, file } = makeFile();
  try {
    hardenCtrlSocket(file, {});
    assert.strictEqual(fs.statSync(file).mode & 0o777, 0o660);
  } finally {
    cleanup(dir, file);
  }
});

test('ignores a non-numeric uid/gid but still tightens the mode', () => {
  const { dir, file } = makeFile();
  try {
    hardenCtrlSocket(file, { ISOLA_CTRL_SOCK_UID: '', ISOLA_CTRL_SOCK_GID: 'not-a-number' });
    assert.strictEqual(fs.statSync(file).mode & 0o777, 0o660);
  } finally {
    cleanup(dir, file);
  }
});

test('does not throw when the socket path does not exist', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'ctrl-sock-'));
  try {
    assert.doesNotThrow(() =>
      hardenCtrlSocket(path.join(dir, 'missing.sock'), {
        ISOLA_CTRL_SOCK_UID: String(process.getuid()),
        ISOLA_CTRL_SOCK_GID: String(process.getgid()),
      }));
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});
