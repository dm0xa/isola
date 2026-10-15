// Address-bar and in-page navigation policy.
//
// Plain text still becomes a DuckDuckGo search. A value is loaded only when
// it is about:blank or an http(s) URL whose host is not loopback, private,
// link-local, or unspecified. Any other scheme is searched, never opened.

const net = require('net');

const SEARCH_URL = 'https://duckduckgo.com/?q=';

const internalNets = new net.BlockList();
for (const [addr, prefix] of [
  ['0.0.0.0', 8],
  ['10.0.0.0', 8],
  ['127.0.0.0', 8],
  ['169.254.0.0', 16],
  ['172.16.0.0', 12],
  ['192.168.0.0', 16],
  ['224.0.0.0', 24],
]) {
  internalNets.addSubnet(addr, prefix, 'ipv4');
}
internalNets.addAddress('::', 'ipv6');
internalNets.addAddress('::1', 'ipv6');
for (const [addr, prefix] of [
  ['fc00::', 7],
  ['fe80::', 10],
  ['ff00::', 8],
]) {
  internalNets.addSubnet(addr, prefix, 'ipv6');
}

function searchURL(query) {
  return SEARCH_URL + encodeURIComponent(query);
}

// A single token the address bar should treat as a host rather than a search,
// even when it has no dot (localhost, ::1).
function looksLikeLocalTarget(trimmed) {
  const hostOnly = trimmed.split(/[/?#]/)[0];
  const withoutPort = hostOnly.replace(/:\d+$/, '');
  const host = withoutPort.replace(/^\[|\]$/g, '').replace(/\.+$/, '').toLowerCase();
  if (!host) return false;
  if (host === 'localhost' || host.endsWith('.localhost')) return true;
  return net.isIP(host) !== 0;
}

function isInternalHost(hostname) {
  // A trailing dot is the DNS root label: `localhost.` resolves exactly like
  // `localhost`, so strip it (and any percent-/IDN-encoded equivalent, which
  // `new URL` has already normalised to a dot) before the denylist check.
  const host = String(hostname || '').replace(/^\[|\]$/g, '').replace(/\.+$/, '').toLowerCase();
  if (!host) return true;
  if (host === 'localhost' || host.endsWith('.localhost')) return true;
  const kind = net.isIP(host);
  if (!kind) return false;
  return internalNets.check(host, kind === 4 ? 'ipv4' : 'ipv6');
}

// isAllowedPageNavigation is the check for a URL the page itself is about to
// open (link, redirect, window.open). It does not rewrite text into a search.
function isAllowedPageNavigation(raw) {
  let parsed;
  try {
    parsed = new URL(raw);
  } catch {
    return false;
  }
  if (parsed.protocol === 'about:' && parsed.pathname === 'blank') return true;
  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') return false;
  return !isInternalHost(parsed.hostname);
}

// resolveAddressBar maps address-bar / tab command input to a URL to load.
// Returns null when the input is an http(s) URL that must not be opened.
// Non-URLs, including non-http schemes, become a DuckDuckGo search.
function resolveAddressBar(input) {
  const trimmed = String(input || '').trim();
  if (!trimmed || /^about:blank(?:[?#].*)?$/i.test(trimmed)) return 'about:blank';
  if (/[ \t\n\r]/.test(trimmed)) return searchURL(trimmed);
  if (/^https?:\/\//i.test(trimmed)) {
    return isAllowedPageNavigation(trimmed) ? trimmed : null;
  }
  if (/^[a-z][a-z0-9+.-]*:/i.test(trimmed)) return searchURL(trimmed);
  if (looksLikeLocalTarget(trimmed) || trimmed.includes('.')) {
    const url = 'https://' + trimmed;
    return isAllowedPageNavigation(url) ? url : null;
  }
  return searchURL(trimmed);
}

module.exports = {
  isAllowedPageNavigation,
  resolveAddressBar,
};
