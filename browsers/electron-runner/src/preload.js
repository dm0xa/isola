// Preload script: minimal exposure bridge.
// contextIsolation: true means this is the only Node/Electron API accessible
// from the renderer. We expose nothing — input comes via CDP, not injected JS.
const { contextBridge } = require('electron');

contextBridge.exposeInMainWorld('isolaBridge', {
  // Reserved for future renderer-side features.
  // Currently all interaction is via CDP (mouse/keyboard/clipboard).
});
