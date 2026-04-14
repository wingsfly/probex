# Chrome Extension Migration

The Chrome WebRTC + Guidex interaction extension has been split out from this repository.

New standalone project path:

- `/Users/hjma/workspace/network/probex-webrtc-guidex-extension`

This keeps the ProbeX backend and the browser extension decoupled for independent release and maintenance.

To load the extension in Chrome:

1. Open `chrome://extensions/`
2. Enable `Developer mode`
3. Click `Load unpacked`
4. Select `/Users/hjma/workspace/network/probex-webrtc-guidex-extension`
