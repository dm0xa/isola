const { test } = require('node:test');
const assert = require('node:assert');
const fs = require('node:fs');
const path = require('node:path');

// Static wiring guards for the Electron container entrypoint. The behavior it
// encodes (Tor as its own UID, fail-closed bootstrap, no direct web egress)
// can only be exercised with a live container + Tor network, which a unit-test
// run cannot assume — so these assert the rules are present and un-drifted.
const read = (rel) => fs.readFileSync(path.join(__dirname, '..', '..', rel), 'utf8');
const electron = read('browser-node-electron/entrypoint.sh');

test('electron entrypoint gates startup on a live Tor circuit', () => {
  assert.match(electron, /status\/circuit-established=1/, 'must probe Tor readiness');
  assert.match(electron, /refusing to start without Tor routing/, 'must fail closed');
  assert.match(electron, /exit 2/, 'a Tor bootstrap failure must abort the container');
});

test('electron entrypoint scopes direct web egress to the Tor UID', () => {
  assert.match(electron, /--uid-owner/, 'web ports must be scoped to the Tor daemon user');
  assert.match(electron, /TOR_UID/, 'the firewall must reference the resolved Tor UID');
});

test('entrypoint exempts the Tor daemon by UID, not by a port list', () => {
  assert.match(
    electron,
    /iptables -A OUTPUT -m owner --uid-owner "\$TOR_UID" -j ACCEPT/,
    'expected a blanket Tor-UID egress exemption',
  );
});

test('no stale browser-node-tor image is referenced', () => {
  const justfile = fs.readFileSync(path.join(__dirname, '..', '..', '..', 'Justfile'), 'utf8');
  assert.doesNotMatch(justfile, /browser-node-tor/);
  assert.doesNotMatch(justfile, /isola-browser:tor/);
});
