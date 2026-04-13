// ProbeX WebRTC Monitor - Injected into page main world
// Hooks RTCPeerConnection, collects getStats(), and pushes directly to ProbeX hub.
// Runs as pure page JS — survives extension updates/restarts.
(function () {
  'use strict';

  // If already hooked, just update config and resume
  if (window.__probexWebrtcHooked) {
    if (window.__probexResume) window.__probexResume();
    return;
  }
  window.__probexWebrtcHooked = true;

  const OriginalRTCPeerConnection =
    window.RTCPeerConnection || window.webkitRTCPeerConnection;
  if (!OriginalRTCPeerConnection) return;

  // --- Config (injected via postMessage from content-script) ---
  let hubUrl = 'http://localhost:8080';
  let probeName = 'webrtc-browser';
  let agentId = '';
  let collectInterval = 2000;
  let pushInterval = 5000;
  let enabled = true;
  let registered = false;

  // --- State ---
  const connections = new Map();
  let nextId = 1;
  let resultBuffer = [];
  let collectTimer = null;
  let pushTimer = null;
  let stopped = false;
  let pushOk = 0;
  let pushFail = 0;
  let lastPushAt = 0;
  let latestMetrics = null;
  let activeConnections = 0;

  // Listen for config from content-script
  window.addEventListener('message', (event) => {
    if (event.source !== window) return;

    if (event.data?.type === 'probex-config') {
      const c = event.data;
      if (c.hubUrl) hubUrl = c.hubUrl;
      if (c.probeName) probeName = c.probeName;
      if (c.agentId) agentId = c.agentId;
      if (c.collectInterval) {
        collectInterval = c.collectInterval;
        startCollectLoop();
      }
      if (c.pushInterval) {
        pushInterval = c.pushInterval;
        startPushLoop();
      }
      if (c.enabled !== undefined) {
        enabled = c.enabled;
        if (enabled) { startCollectLoop(); startPushLoop(); }
        else { stopAll(); }
      }
      // Re-register if probe name changed
      registered = false;
      return;
    }

    if (event.data?.type === 'probex-stop') {
      stopAll();
      return;
    }
  });

  // ====== RTCPeerConnection Hook ======

  function ProxiedRTCPeerConnection(config, constraints) {
    const pc = constraints
      ? new OriginalRTCPeerConnection(config, constraints)
      : new OriginalRTCPeerConnection(config);

    const id = nextId++;
    connections.set(id, { pc, prevStats: null });
    console.log('[ProbeX] new PeerConnection #%d, total=%d', id, connections.size);

    const origClose = pc.close.bind(pc);
    pc.close = function () {
      connections.delete(id);
      console.log('[ProbeX] PC #%d closed (close()), remaining=%d', id, connections.size);
      return origClose();
    };
    pc.addEventListener('connectionstatechange', () => {
      console.log('[ProbeX] PC #%d state=%s', id, pc.connectionState);
      if (pc.connectionState === 'closed' || pc.connectionState === 'failed') {
        connections.delete(id);
        console.log('[ProbeX] PC #%d removed, remaining=%d', id, connections.size);
      }
    });
    return pc;
  }

  ProxiedRTCPeerConnection.prototype = OriginalRTCPeerConnection.prototype;
  Object.keys(OriginalRTCPeerConnection).forEach(k => { ProxiedRTCPeerConnection[k] = OriginalRTCPeerConnection[k]; });
  ProxiedRTCPeerConnection.generateCertificate = OriginalRTCPeerConnection.generateCertificate;

  window.RTCPeerConnection = ProxiedRTCPeerConnection;
  if (window.webkitRTCPeerConnection) window.webkitRTCPeerConnection = ProxiedRTCPeerConnection;

  // ====== Stats Collection ======

  async function collectAllStats() {
    if (stopped || !enabled) return;

    for (const [id, entry] of connections) {
      const { pc, prevStats } = entry;
      if (pc.connectionState === 'closed' || pc.connectionState === 'failed') {
        connections.delete(id);
        continue;
      }
      try {
        const stats = await pc.getStats();
        const metrics = extractMetrics(stats, prevStats);
        if (metrics) {
          latestMetrics = metrics;
          resultBuffer.push({
            timestamp: new Date().toISOString(),
            pageUrl: location.href,
            connectionCount: connections.size,
            metrics,
          });
        }
        entry.prevStats = statsToMap(stats);
      } catch (e) { /* PC may have been closed */ }
    }

    // Bound buffer
    if (resultBuffer.length > 200) resultBuffer = resultBuffer.slice(-100);
  }

  // ====== Network: proxy through extension (avoids mixed content) with direct fallback ======

  // Request ID counter for proxy RPC
  let rpcId = 0;
  const rpcCallbacks = new Map();

  // Listen for proxy responses from content-script
  window.addEventListener('message', (event) => {
    if (event.source !== window || event.data?.type !== 'probex-fetch-response') return;
    const cb = rpcCallbacks.get(event.data.id);
    if (cb) { rpcCallbacks.delete(event.data.id); cb(event.data); }
  });

  function proxyFetch(url, options) {
    return new Promise((resolve) => {
      const id = ++rpcId;
      const timeout = setTimeout(() => {
        rpcCallbacks.delete(id);
        resolve(null); // null = proxy unavailable, caller should fallback
      }, 3000);
      rpcCallbacks.set(id, (data) => {
        clearTimeout(timeout);
        resolve(data); // { ok, status, body }
      });
      window.postMessage({
        type: 'probex-fetch-request',
        id,
        url,
        method: options.method || 'GET',
        body: options.body || null,
      }, '*');
    });
  }

  // Unified fetch: try extension proxy first (no mixed content), fallback to direct
  async function probexFetch(url, options) {
    // Try proxy through content-script → background (bypasses mixed content)
    const proxyResult = await proxyFetch(url, options);
    if (proxyResult) {
      return { ok: proxyResult.ok, status: proxyResult.status };
    }
    // Fallback: direct fetch (works for localhost, same-protocol, etc.)
    const resp = await fetch(url, {
      method: options.method || 'GET',
      headers: { 'Content-Type': 'application/json' },
      body: options.body || null,
    });
    return { ok: resp.ok, status: resp.status };
  }

  // ====== Push to ProbeX ======

  async function registerProbe() {
    try {
      const result = await probexFetch(`${hubUrl}/api/v1/probes/register`, {
        method: 'POST',
        body: JSON.stringify({
          name: probeName,
          description: 'Chrome extension: browser-side WebRTC quality monitor via getStats()',
          output_schema: {
            standard_fields: ['latency_ms', 'packet_loss_pct', 'download_bps', 'upload_bps'],
            extra_fields: [
              { name: 'audio_jitter', type: 'number', unit: 'ms', description: 'Audio RTP interarrival jitter', chartable: true },
              { name: 'video_jitter', type: 'number', unit: 'ms', description: 'Video RTP interarrival jitter', chartable: true },
              { name: 'video_frames_decoded', type: 'number', unit: 'frames', chartable: true },
              { name: 'video_frames_dropped', type: 'number', unit: 'frames', chartable: true },
              { name: 'video_fps', type: 'number', unit: 'fps', chartable: true },
              { name: 'quality_limitation', type: 'string', description: 'cpu/bandwidth/none' },
              { name: 'available_outgoing_bitrate', type: 'number', unit: 'bps', chartable: true },
              { name: 'page_url', type: 'string', description: 'Source page URL' },
              { name: 'connection_count', type: 'number', description: 'Active PeerConnection count' },
            ],
          },
        }),
      });
      registered = result.ok || result.status === 409;
    } catch (e) { registered = false; }
  }

  async function pushResults() {
    if (!enabled || resultBuffer.length === 0) return;
    console.log('[ProbeX] push: buffer=%d conns=%d', resultBuffer.length, connections.size);
    if (!registered) { await registerProbe(); if (!registered) { console.warn('[ProbeX] registration failed'); return; } }

    const batch = resultBuffer.splice(0);
    const probeResults = mergeBatch(batch);
    if (probeResults.length === 0) return;

    try {
      const result = await probexFetch(
        `${hubUrl}/api/v1/probes/${encodeURIComponent(probeName)}/push`,
        {
          method: 'POST',
          body: JSON.stringify({
            task_id: `ext_${probeName}`,
            agent_id: agentId,
            results: probeResults,
          }),
        }
      );
      if (result.ok) {
        pushOk += probeResults.length;
        lastPushAt = Date.now();
        console.log('[ProbeX] pushed %d results OK', probeResults.length);
      } else {
        pushFail++;
        console.error('[ProbeX] push HTTP %d', result.status);
      }
      if (result.status === 404) registered = false;
      activeConnections = batch[batch.length - 1]?.connectionCount || 0;
    } catch (e) {
      pushFail++;
      console.error('[ProbeX] push error:', e.message);
      resultBuffer.unshift(...batch.slice(-20));
    }
  }

  function mergeBatch(batch) {
    const probeResults = [];
    const groups = new Map();
    for (const item of batch) {
      const key = item.timestamp;
      if (!groups.has(key)) groups.set(key, []);
      groups.get(key).push(item);
    }
    for (const [ts, items] of groups) {
      const result = { timestamp: ts, success: true, extra: {} };
      let totalDown = 0, hasDown = false, totalUp = 0, hasUp = false;
      let totalDecoded = 0, hasDecoded = false, totalDropped = 0, hasDropped = false;
      let worstLoss = 0;
      for (const item of items) {
        const r = item.metrics;
        if (result.latency_ms == null && r.latencyMs != null) result.latency_ms = r.latencyMs;
        if (!result.extra.audio_jitter && r.audioJitter != null) result.extra.audio_jitter = r.audioJitter;
        if (!result.extra.video_jitter && r.videoJitter != null) result.extra.video_jitter = r.videoJitter;
        if (!result.extra.video_fps && r.videoFps != null) result.extra.video_fps = r.videoFps;
        if (!result.extra.quality_limitation || result.extra.quality_limitation === 'none') {
          if (r.qualityLimitation) result.extra.quality_limitation = r.qualityLimitation;
        }
        if (!result.extra.available_outgoing_bitrate && r.availableOutgoingBitrate != null)
          result.extra.available_outgoing_bitrate = r.availableOutgoingBitrate;
        if (r.packetLossPct != null && r.packetLossPct > worstLoss) worstLoss = r.packetLossPct;
        if (r.downloadBps != null) { totalDown += r.downloadBps; hasDown = true; }
        if (r.uploadBps != null) { totalUp += r.uploadBps; hasUp = true; }
        if (r.videoFramesDecoded != null) { totalDecoded += r.videoFramesDecoded; hasDecoded = true; }
        if (r.videoFramesDropped != null) { totalDropped += r.videoFramesDropped; hasDropped = true; }
      }
      result.packet_loss_pct = worstLoss;
      if (hasDown) result.download_bps = totalDown;
      if (hasUp) result.upload_bps = totalUp;
      if (hasDecoded) result.extra.video_frames_decoded = totalDecoded;
      if (hasDropped) result.extra.video_frames_dropped = totalDropped;
      result.extra.page_url = items[0].pageUrl || '';
      result.extra.connection_count = items[0].connectionCount || 0;
      probeResults.push(result);
    }
    return probeResults;
  }

  // ====== Metric Extraction (unchanged logic) ======

  function statsToMap(statsReport) {
    const map = new Map();
    statsReport.forEach(r => map.set(r.id, { ...r }));
    return map;
  }

  function extractMetrics(currentReport, prevMap) {
    const now = {};
    const inboundAudio = [], inboundVideo = [], outboundAudio = [], outboundVideo = [], remoteInbound = [];
    let selectedPair = null;

    currentReport.forEach(report => {
      if (report.type === 'inbound-rtp') {
        if (report.kind === 'audio') inboundAudio.push(report);
        else if (report.kind === 'video') inboundVideo.push(report);
      } else if (report.type === 'outbound-rtp') {
        if (report.kind === 'audio') outboundAudio.push(report);
        else if (report.kind === 'video') outboundVideo.push(report);
      } else if (report.type === 'remote-inbound-rtp') {
        remoteInbound.push(report);
      } else if (report.type === 'candidate-pair' && report.state === 'succeeded') {
        if (!selectedPair || report.nominated || report.bytesReceived > (selectedPair.bytesReceived || 0))
          selectedPair = report;
      }
    });

    if (inboundAudio.length === 0 && inboundVideo.length === 0 && remoteInbound.length === 0)
      return null;

    // RTT
    let rttMs = null;
    for (const r of remoteInbound) { if (r.roundTripTime != null) { rttMs = r.roundTripTime * 1000; break; } }
    if (rttMs == null && selectedPair?.currentRoundTripTime != null) rttMs = selectedPair.currentRoundTripTime * 1000;
    now.latencyMs = rttMs;

    // Audio jitter
    for (const r of inboundAudio) { if (r.jitter != null) { now.audioJitter = r.jitter * 1000; break; } }
    // Video jitter
    for (const r of inboundVideo) { if (r.jitter != null) { now.videoJitter = r.jitter * 1000; break; } }

    // Packet loss
    let totalLostDelta = 0, totalRecvDelta = 0;
    const allInbound = [...inboundAudio, ...inboundVideo];
    if (prevMap) {
      for (const r of allInbound) {
        const prev = prevMap.get(r.id);
        if (prev) {
          totalLostDelta += (r.packetsLost || 0) - (prev.packetsLost || 0);
          totalRecvDelta += (r.packetsReceived || 0) - (prev.packetsReceived || 0);
        }
      }
    }
    const totalPkt = totalRecvDelta + totalLostDelta;
    now.packetLossPct = totalPkt > 0 ? (totalLostDelta / totalPkt) * 100 : 0;

    // Download bitrate
    let bytesRecvDelta = 0, timeDelta = 0;
    if (prevMap) {
      for (const r of allInbound) {
        const prev = prevMap.get(r.id);
        if (prev) { bytesRecvDelta += (r.bytesReceived || 0) - (prev.bytesReceived || 0); if (!timeDelta) timeDelta = (r.timestamp - prev.timestamp) / 1000; }
      }
    }
    now.downloadBps = timeDelta > 0 ? (bytesRecvDelta * 8) / timeDelta : null;

    // Upload bitrate
    let bytesSentDelta = 0, sendDelta = 0;
    if (prevMap) {
      for (const r of [...outboundAudio, ...outboundVideo]) {
        const prev = prevMap.get(r.id);
        if (prev) { bytesSentDelta += (r.bytesSent || 0) - (prev.bytesSent || 0); if (!sendDelta) sendDelta = (r.timestamp - prev.timestamp) / 1000; }
      }
    }
    now.uploadBps = sendDelta > 0 ? (bytesSentDelta * 8) / sendDelta : null;

    // Video frames
    let framesDecodedDelta = 0, framesDroppedDelta = 0, videoFps = null;
    for (const r of inboundVideo) {
      if (prevMap) {
        const prev = prevMap.get(r.id);
        if (prev) { framesDecodedDelta += (r.framesDecoded || 0) - (prev.framesDecoded || 0); framesDroppedDelta += (r.framesDropped || 0) - (prev.framesDropped || 0); }
      }
      if (r.framesPerSecond != null) videoFps = r.framesPerSecond;
    }
    now.videoFramesDecoded = framesDecodedDelta;
    now.videoFramesDropped = framesDroppedDelta;
    now.videoFps = videoFps;

    // Quality limitation
    now.qualityLimitation = 'none';
    for (const r of outboundVideo) { if (r.qualityLimitationReason && r.qualityLimitationReason !== 'none') { now.qualityLimitation = r.qualityLimitationReason; break; } }

    // Available outgoing bitrate
    now.availableOutgoingBitrate = selectedPair?.availableOutgoingBitrate ?? null;

    return now;
  }

  // ====== Timers ======

  function startCollectLoop() {
    if (collectTimer) clearInterval(collectTimer);
    collectTimer = setInterval(collectAllStats, collectInterval);
  }

  function startPushLoop() {
    if (pushTimer) clearInterval(pushTimer);
    pushTimer = setInterval(pushResults, pushInterval);
  }

  function stopAll() {
    stopped = true;
    if (collectTimer) { clearInterval(collectTimer); collectTimer = null; }
    if (pushTimer) { clearInterval(pushTimer); pushTimer = null; }
  }

  // ====== API for popup (called via chrome.scripting.executeScript with world: MAIN) ======

  window.__probexStats = () => ({
    connections: connections.size,
    pushOk,
    pushFail,
    lastPushAt,
    latest: latestMetrics,
    bufferSize: resultBuffer.length,
    stopped,
  });

  // ====== Resume (called when extension re-injects content-script) ======

  window.__probexResume = () => {
    stopped = false;
    startCollectLoop();
    startPushLoop();
  };

  // ====== Start ======

  console.log('[ProbeX] injected.js started, hubUrl=%s, collectInterval=%d, pushInterval=%d', hubUrl, collectInterval, pushInterval);
  startCollectLoop();
  startPushLoop();
})();
