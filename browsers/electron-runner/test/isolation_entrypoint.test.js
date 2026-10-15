const { test } = require('node:test');
const assert = require('node:assert');
const fs = require('node:fs');
const path = require('node:path');

const read = (rel) => fs.readFileSync(path.join(__dirname, '..', '..', rel), 'utf8');
const electron = read('browser-node-electron/entrypoint.sh');

test('firewall rules are fail-closed (no swallowed iptables errors)', () => {
  for (const line of electron.split('\n')) {
    const trimmed = line.trim();
    if (trimmed.startsWith('#')) continue;
    if (/iptables\b/.test(trimmed) || /ip6tables\b/.test(trimmed)) {
      assert.ok(!/\|\s*true\s*$/.test(trimmed), `firewall line must not ignore errors: ${trimmed}`);
    }
  }
});

test('private-range DROP precedes outbound port ACCEPTs', () => {
  const dropIdx = electron.indexOf('iptables -A OUTPUT -d "$net" -j DROP');
  const acceptIdx = electron.indexOf('for port in 80 443 53 123');
  assert.ok(dropIdx >= 0 && acceptIdx >= 0, 'expected DROP loop and egress port loop');
  assert.ok(dropIdx < acceptIdx, 'RFC1918 DROP must come before web port ACCEPTs');
});

test('session service ports require a source address', () => {
  assert.match(electron, /apply_session_input_ipv4/, 'expected named INPUT helper');
  assert.doesNotMatch(
    electron,
    /iptables -A INPUT -p tcp --dport 5900 -j ACCEPT/,
    'must not accept 5900 from any source',
  );
  assert.doesNotMatch(
    electron,
    /iptables -A INPUT -p tcp --dport 9223 -j ACCEPT/,
    'must not accept 9223 from any source',
  );
  assert.match(electron, /-s "\$peer" --dport 5900/, '5900 rule must use -s');
});

test('session processes drop to isolauser', () => {
  assert.match(electron, /RUN_AS_USER="gosu isolauser"/);
  assert.match(electron, /\$RUN_AS_USER \/usr\/local\/bin\/isola-streamer/);
  assert.match(electron, /\$RUN_AS_USER Xvfb/);
  assert.match(electron, /\$RUN_AS_USER node_modules\/electron\/dist\/electron/);
  assert.match(electron, /\$RUN_AS_USER socat/);
});

test('entrypoint aborts when session peer IP cannot be resolved', () => {
  assert.match(electron, /cannot determine default gateway for session service ports/);
  assert.match(electron, /firewall_fatal/);
});

test('electron argv does not bypass /dev/shm sizing', () => {
  assert.doesNotMatch(electron, /--disable-dev-shm-usage/);
});

test('ip6tables DNS allows only IPv6 resolvers', () => {
  assert.match(electron, /dns_addr_matches_table/, 'DNS allows must be family-scoped');
  assert.match(electron, /is_ipv6_addr "\$peer"/, 'IPv6 INPUT must skip IPv4 peers');
});

test('entrypoint requires an enforcing MAC policy (fail-closed)', () => {
  assert.match(electron, /require_mac_confinement/, 'expected a MAC precondition');
  assert.match(electron, /ISOLA_REQUIRE_MAC/, 'expected an opt-out env var');
  assert.match(
    electron,
    /\/sys\/module\/apparmor\/parameters\/enabled/,
    'must probe AppArmor state',
  );
  assert.match(electron, /\/sys\/fs\/selinux\/enforce/, 'must probe SELinux state');
  // Must abort, not warn-and-continue, when nothing confines the sandbox.
  assert.match(electron, /refusing to start an unconfined sandbox/);
});

test('MAC precondition runs before the firewall is applied', () => {
  assert.match(
    electron,
    /require_mac_confinement\s*\n\s*apply_egress_ipv4/,
    'MAC check must run before apply_egress_ipv4',
  );
});
