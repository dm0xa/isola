const test = require('node:test');
const assert = require('node:assert');

const { isAllowedPageNavigation, resolveAddressBar } = require('../src/navigation_url');

test('plain text and non-web schemes become a DuckDuckGo search', () => {
  assert.strictEqual(
    resolveAddressBar('isola browser'),
    'https://duckduckgo.com/?q=isola%20browser',
  );
  assert.strictEqual(
    resolveAddressBar('file:///etc/passwd'),
    'https://duckduckgo.com/?q=file%3A%2F%2F%2Fetc%2Fpasswd',
  );
  assert.strictEqual(
    resolveAddressBar('javascript:alert(1)'),
    'https://duckduckgo.com/?q=javascript%3Aalert(1)',
  );
  assert.ok(resolveAddressBar('chrome://settings').startsWith('https://duckduckgo.com/?q='));
});

test('public hosts load as https, explicit http(s) is kept', () => {
  assert.strictEqual(resolveAddressBar('example.com'), 'https://example.com');
  assert.strictEqual(resolveAddressBar('example.com/docs'), 'https://example.com/docs');
  assert.strictEqual(resolveAddressBar('https://example.com/a'), 'https://example.com/a');
  assert.strictEqual(resolveAddressBar('http://example.com'), 'http://example.com');
});

test('internal http(s) targets are refused', () => {
  for (const input of [
    'http://127.0.0.1/',
    'http://127.1/',
    'http://2130706433/',
    'http://0x7f000001/',
    'https://localhost/',
    'http://foo.localhost/admin',
    'http://10.0.0.1/',
    'http://192.168.1.1/',
    'http://169.254.169.254/',
    'http://[::1]/',
    '127.0.0.1',
    'localhost',
  ]) {
    assert.strictEqual(resolveAddressBar(input), null, input);
  }
});

test('empty input and about:blank stay on about:blank', () => {
  assert.strictEqual(resolveAddressBar(''), 'about:blank');
  assert.strictEqual(resolveAddressBar('   '), 'about:blank');
  assert.strictEqual(resolveAddressBar('about:blank'), 'about:blank');
});

test('page navigations only allow about:blank and public http(s)', () => {
  assert.strictEqual(isAllowedPageNavigation('about:blank'), true);
  assert.strictEqual(isAllowedPageNavigation('https://example.com/path'), true);
  assert.strictEqual(isAllowedPageNavigation('file:///etc/passwd'), false);
  assert.strictEqual(isAllowedPageNavigation('javascript:alert(1)'), false);
  assert.strictEqual(isAllowedPageNavigation('http://127.0.0.1/'), false);
  assert.strictEqual(isAllowedPageNavigation('http://[::1]/'), false);
});

test('loopback with a trailing dot or encoded dot is still refused', () => {
  // `localhost.` is the DNS root form of `localhost` and resolves the same way,
  // so it must not slip past the denylist (the same goes for %2e spellings,
  // which `new URL` normalises to a dot before the check).
  for (const input of [
    'http://localhost./',
    'https://localhost.:8443/',
    'http://localhost%2e/',
    'http://localhost%2e%2e/',
    'http://sub.localhost./',
  ]) {
    assert.strictEqual(isAllowedPageNavigation(input), false, input);
    assert.strictEqual(resolveAddressBar(input), null, input);
  }
});
