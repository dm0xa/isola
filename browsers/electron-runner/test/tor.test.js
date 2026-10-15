const { test } = require('node:test');
const assert = require('node:assert');
const { torProxyConfig, applyTorProxy, isTorEnabled } = require('../src/tor');

test('TOR_ENABLED accepts the orchestrator value', () => {
  // The orchestrator exports `true`; the old `=== '1'` check silently disabled
  // the Electron proxy, so the browser went out unproxied under a Tor session.
  assert.equal(isTorEnabled('true'), true);
  assert.equal(isTorEnabled('1'), true);
  for (const off of [undefined, '', '0', 'false', 'no']) {
    assert.equal(isTorEnabled(off), false, `${JSON.stringify(off)} must read as off`);
  }
});

test('Tor proxy uses the Chromium SOCKS5 scheme (remote DNS, no socks5h)', () => {
  // Chromium has no `socks5h` scheme: it resolves hostnames through the SOCKS
  // proxy by default, and `socks5h` is rejected with ERR_NO_SUPPORTED_PROXIES.
  const cfg = torProxyConfig();
  assert.match(cfg.proxyRules, /^socks5:\/\//);
  assert.doesNotMatch(cfg.proxyRules, /socks5h/);
  assert.ok(cfg.proxyRules.endsWith(':9050'), `unexpected port: ${cfg.proxyRules}`);
});

test('Tor proxy bypasses nothing', () => {
  // A non-empty bypass rule (the old `<local>`) sends matching hostnames out
  // unproxied — both a leak and a divergence from the Chromium path.
  assert.equal(torProxyConfig().proxyBypassRules, '');
});

test('applyTorProxy installs the config and surfaces failures', async () => {
  const seen = [];
  const session = { setProxy: (cfg) => { seen.push(cfg); return Promise.resolve(); } };
  await applyTorProxy(session);
  assert.equal(seen.length, 1);
  assert.equal(seen[0].proxyRules, torProxyConfig().proxyRules);

  // A rejected setProxy must propagate: the caller fails closed on it.
  const failing = { setProxy: () => Promise.reject(new Error('no tor')) };
  await assert.rejects(() => applyTorProxy(failing), /no tor/);
});
