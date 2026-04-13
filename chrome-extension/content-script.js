// ProbeX WebRTC Monitor - Content Script (ISOLATED world)
// 1. Passes config from chrome.storage to injected.js (MAIN world)
// 2. Proxies fetch requests from injected.js through the extension
//    (bypasses mixed content: HTTPS page → HTTP ProbeX server)
(function () {
  'use strict';

  // --- Config delivery ---

  function sendConfig() {
    try {
      chrome.storage.local.get('probexConfig', (stored) => {
        const c = stored.probexConfig || {};
        window.postMessage({
          type: 'probex-config',
          hubUrl: c.hubUrl || 'http://localhost:8080',
          probeName: c.probeName || 'webrtc-browser',
          agentId: c.agentId || '',
          collectInterval: c.collectInterval || 2000,
          pushInterval: c.pushInterval || 5000,
          enabled: c.enabled !== false,
        }, '*');
      });
    } catch (e) { /* extension context gone */ }
  }

  sendConfig();

  try {
    chrome.storage.onChanged.addListener((changes) => {
      if (changes.probexConfig) sendConfig();
    });
  } catch (e) {}

  // --- Fetch proxy: injected.js (MAIN) → content-script (ISOLATED) → background SW ---
  // This bypasses mixed content restrictions because the background SW
  // has host_permissions: <all_urls> and is not bound by the page's protocol.

  window.addEventListener('message', async (event) => {
    if (event.source !== window) return;
    if (event.data?.type !== 'probex-fetch-request') return;

    const { id, url, method, body } = event.data;

    try {
      // Send to background SW which does the actual fetch
      const resp = await chrome.runtime.sendMessage({
        type: 'proxy-fetch',
        url,
        method,
        body,
      });
      window.postMessage({
        type: 'probex-fetch-response',
        id,
        ok: resp?.ok ?? false,
        status: resp?.status ?? 0,
        body: resp?.body ?? null,
      }, '*');
    } catch (e) {
      // Extension context invalidated — tell injected.js to use fallback
      window.postMessage({
        type: 'probex-fetch-response',
        id,
        ok: false,
        status: 0,
        error: 'proxy unavailable',
      }, '*');
    }
  });
})();
