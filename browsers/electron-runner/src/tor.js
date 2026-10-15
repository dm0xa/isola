'use strict';

// Tor SOCKS proxy wiring for the Electron runner.
//
// Two invariants this module exists to keep, both of which the previous inline
// `session.setProxy` call violated:
//
//   1. The proxy must actually be applied. The old activation check compared
//      TOR_ENABLED to '1', but the orchestrator exports 'true', so the proxy
//      was never installed and the browser went out unproxied under a Tor
//      session (see isTorEnabled).
//   2. There must be no proxy bypass. The old `proxyBypassRules: '<local>'`
//      sent dotless hostnames straight out, unproxied — a leak, and a
//      reachability difference from the Chromium path.
//
// The scheme is `socks5`, not `socks5h`: Chromium's SOCKS5 client resolves
// hostnames THROUGH the proxy (remote DNS is its default), and it has no
// `socks5h` scheme — passing one is rejected at load time with
// ERR_NO_SUPPORTED_PROXIES (verified against this image). DNS therefore does
// not leak, and adding an `h` would only break the session.
const TOR_SOCKS_HOST = '127.0.0.1';
const TOR_SOCKS_PORT = 9050;

// isTorEnabled parses the TOR_ENABLED value. The orchestrator exports `true`,
// but `1` is accepted too for parity with the container entrypoints. The old
// `=== '1'` check meant the Electron proxy was never applied in practice —
// Tor looked enabled in the UI while the browser went out unproxied.
function isTorEnabled(value) {
  return value === '1' || value === 'true';
}

// torProxyConfig returns the Electron session proxy descriptor for Tor.
function torProxyConfig() {
  return {
    proxyRules: `socks5://${TOR_SOCKS_HOST}:${TOR_SOCKS_PORT}`,
    proxyBypassRules: '',
  };
}

// applyTorProxy installs the Tor proxy on a session and resolves only once
// Chromium has accepted it, so a caller can await it before issuing any
// navigation. A rejection means the session would have served traffic without
// Tor: callers must fail closed rather than continue.
function applyTorProxy(session) {
  return Promise.resolve(session.setProxy(torProxyConfig()));
}

module.exports = { TOR_SOCKS_HOST, TOR_SOCKS_PORT, isTorEnabled, torProxyConfig, applyTorProxy };
