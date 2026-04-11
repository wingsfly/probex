#!/bin/bash
# PROBEX_META: {
# PROBEX_META:   "name": "rtp-sim",
# PROBEX_META:   "description": "RTP traffic simulation — measures jitter, loss, reorder with codec-like UDP patterns, computes MOS (E-model)",
# PROBEX_META:   "parameter_schema": {
# PROBEX_META:     "type": "object",
# PROBEX_META:     "properties": {
# PROBEX_META:       "port":        {"type":"integer","title":"iPerf3 Server Port","default":5201,"minimum":1,"maximum":65535},
# PROBEX_META:       "mode":        {"type":"string","title":"Traffic Mode","enum":["audio","video","custom"],"default":"audio"},
# PROBEX_META:       "duration":    {"type":"integer","title":"Duration (sec)","default":10,"minimum":1,"maximum":300},
# PROBEX_META:       "bandwidth":   {"type":"string","title":"Bandwidth","x-ui-placeholder":"auto-set by mode, or e.g. 500K"},
# PROBEX_META:       "codec":       {"type":"string","title":"Codec Model","enum":["opus","g711","g729"],"default":"opus"},
# PROBEX_META:       "jitter_buffer_ms": {"type":"integer","title":"Jitter Buffer (ms)","default":60,"minimum":10,"maximum":500}
# PROBEX_META:     },
# PROBEX_META:     "x-ui-order": ["mode","codec","duration","port","bandwidth","jitter_buffer_ms"]
# PROBEX_META:   },
# PROBEX_META:   "output_schema": {
# PROBEX_META:     "standard_fields": ["latency_ms","jitter_ms","packet_loss_pct"],
# PROBEX_META:     "extra_fields": [
# PROBEX_META:       {"name":"mos","type":"number","description":"Estimated MOS (1-4.5)","chartable":true},
# PROBEX_META:       {"name":"r_factor","type":"number","description":"E-model R-factor (0-100)","chartable":true},
# PROBEX_META:       {"name":"out_of_order_pct","type":"number","unit":"%","description":"Packet reorder rate","chartable":true},
# PROBEX_META:       {"name":"effective_loss_pct","type":"number","unit":"%","description":"Loss including reorder beyond jitter buffer","chartable":true},
# PROBEX_META:       {"name":"packets_sent","type":"number","description":"Total packets sent"},
# PROBEX_META:       {"name":"packets_received","type":"number","description":"Total packets received"},
# PROBEX_META:       {"name":"out_of_order","type":"number","description":"Out-of-order packet count"}
# PROBEX_META:     ]
# PROBEX_META:   }
# PROBEX_META: }

# RTP Traffic Simulation Probe
# Uses iperf3 in UDP mode with codec-appropriate bandwidth, then computes MOS.

# Ensure common tool paths are available
export PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:$PATH"

TARGET="${PROBEX_TARGET:-}"
MODE="${PROBEX_PARAM_MODE:-audio}"
DURATION="${PROBEX_PARAM_DURATION:-10}"
PORT="${PROBEX_PARAM_PORT:-5201}"
BANDWIDTH="${PROBEX_PARAM_BANDWIDTH:-}"
CODEC="${PROBEX_PARAM_CODEC:-opus}"
JB_MS="${PROBEX_PARAM_JITTER_BUFFER_MS:-60}"

if [ -z "$TARGET" ]; then
  echo '{"success":false,"error":"PROBEX_TARGET is required"}'
  exit 0
fi

if ! command -v iperf3 &>/dev/null; then
  echo '{"success":false,"error":"iperf3 not found in PATH"}'
  exit 0
fi

# Set bandwidth based on mode if not explicitly given
if [ -z "$BANDWIDTH" ]; then
  case "$MODE" in
    audio)  BANDWIDTH="64K" ;;
    video)  BANDWIDTH="1M" ;;
    custom) BANDWIDTH="500K" ;;
  esac
fi

# Run iperf3 in UDP reverse mode
TMPFILE=$(mktemp)
iperf3 -c "$TARGET" -p "$PORT" -u -R -b "$BANDWIDTH" -t "$DURATION" -J > "$TMPFILE" 2>&1
IPERF_EXIT=$?

# Use python3 to parse and compute MOS
python3 - "$TMPFILE" "$CODEC" "$JB_MS" "$MODE" <<'PYEOF'
import sys, json, math, os

tmpfile, codec, jb_ms_str, mode = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
jb_ms = int(jb_ms_str)

try:
    with open(tmpfile) as f:
        data = json.load(f)
except Exception as e:
    print(json.dumps({"success": False, "error": f"Failed to parse iperf3 output: {e}"}))
    sys.exit(0)
finally:
    os.unlink(tmpfile)

if "error" in data:
    print(json.dumps({"success": False, "error": data["error"]}))
    sys.exit(0)

end = data.get("end", {})
summary = end.get("sum", {})

jitter_ms = summary.get("jitter_ms", 0)
lost = summary.get("lost_packets", 0)
total = summary.get("packets", 0)
loss_pct = (lost / total * 100) if total > 0 else 0

# Out of order from streams
ooo = 0
for s in end.get("streams", []):
    udp = s.get("udp", {})
    ooo += udp.get("out_of_order", 0)
ooo_pct = (ooo / total * 100) if total > 0 else 0

# Effective loss: real loss + reorder beyond jitter buffer
reorder_loss = ooo_pct * 0.3 * min(jitter_ms / jb_ms, 1.0) if jb_ms > 0 else 0
effective_loss = loss_pct + reorder_loss

# E-model MOS
ie_map = {"opus": 2.0, "g711": 0, "g729": 11}
ie_base = ie_map.get(codec, 2.0)

# RTT from sender stats
rtt_ms = 0
for s in end.get("streams", []):
    sender = s.get("sender", {})
    if sender.get("mean_rtt", 0) > 0:
        rtt_ms = sender["mean_rtt"] / 1000.0
        break

delay_ms = jitter_ms * 2 + 50
id_delay = 0.024 * delay_ms
if delay_ms > 177.3:
    id_delay += 0.11 * (delay_ms - 177.3)
ie_eff = ie_base + effective_loss * 2.5
R = max(0, min(100, 93.2 - id_delay - ie_eff))

if R <= 0: mos = 1.0
elif R >= 100: mos = 4.5
else: mos = 1 + 0.035 * R + R * (R - 60) * (100 - R) * 7e-6

print(json.dumps({
    "success": True,
    "latency_ms": round(rtt_ms, 2),
    "jitter_ms": round(jitter_ms, 2),
    "packet_loss_pct": round(loss_pct, 2),
    "extra": {
        "mos": round(mos, 2),
        "r_factor": round(R, 1),
        "out_of_order": ooo,
        "out_of_order_pct": round(ooo_pct, 2),
        "effective_loss_pct": round(effective_loss, 2),
        "packets_sent": total,
        "packets_received": total - lost,
        "codec": codec,
        "jitter_buffer_ms": jb_ms,
        "mode": mode
    }
}))
PYEOF
