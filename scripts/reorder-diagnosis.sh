#!/usr/bin/env bash
#
# ProbeX - UDP Reorder Deep Diagnosis Script
# Automatically diagnoses packet reordering issues through multi-stage analysis
#
# Usage: sudo ./reorder-diagnosis.sh <target_ip> <iperf3_port> <bandwidth>
# Example: sudo ./reorder-diagnosis.sh 101.46.59.52 60020 20M
#

set -euo pipefail

# ============================================================
# Configuration
# ============================================================
TARGET="${1:?Usage: $0 <target_ip> <iperf3_port> <bandwidth>}"
PORT="${2:-60020}"
BW="${3:-20M}"
DURATION=10
OUTDIR="./diagnosis-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$OUTDIR"
REPORT="$OUTDIR/report.txt"
IFACE=$(route get "$TARGET" 2>/dev/null | awk '/interface:/{print $2}' || echo "en0")

# Colors
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[0;33m'; CYAN='\033[0;36m'; NC='\033[0m'

log()  { echo -e "${CYAN}[$(date +%H:%M:%S)]${NC} $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
ok()   { echo -e "${GREEN}[OK]${NC} $*"; }
fail() { echo -e "${RED}[FAIL]${NC} $*"; }

header() {
    echo ""
    echo "============================================================"
    echo "  $1"
    echo "============================================================"
    echo ""
}

# Write to both console and report
tee_report() { tee -a "$REPORT"; }

# ============================================================
# Pre-flight checks
# ============================================================
header "ProbeX Reorder Diagnosis" | tee_report
echo "Target:    $TARGET" | tee_report
echo "Port:      $PORT" | tee_report
echo "Bandwidth: $BW" | tee_report
echo "Interface: $IFACE" | tee_report
echo "Output:    $OUTDIR" | tee_report
echo "Time:      $(date)" | tee_report
echo "" | tee_report

for cmd in mtr traceroute iperf3 tshark tcpdump; do
    if ! command -v "$cmd" &>/dev/null; then
        fail "Missing: $cmd" | tee_report
        exit 1
    fi
done
ok "All required tools available" | tee_report

# Check if running as root (needed for mtr and tcpdump)
if [ "$EUID" -ne 0 ]; then
    warn "Not running as root. Some tests (mtr raw, tcpdump) may fail." | tee_report
    warn "Re-run with: sudo $0 $*" | tee_report
fi

# ============================================================
# Stage 1: Path Discovery — identify ECMP multi-path
# ============================================================
header "Stage 1: Path Discovery (ECMP Detection)" | tee_report

log "Running traceroute (10 probes per hop)..."
traceroute -n -q 10 -w 2 "$TARGET" 2>&1 | tee "$OUTDIR/traceroute.txt" | tee_report

# Analyze: count unique IPs per hop
echo "" | tee_report
echo "--- ECMP Analysis ---" | tee_report
awk '
/^ *[0-9]/ {
    hop = $1
    delete ips
    for (i=2; i<=NF; i++) {
        if ($i ~ /^[0-9]+\.[0-9]+/) ips[$i]++
    }
    n = length(ips)
    if (n > 1) {
        printf "  Hop %2d: %d paths detected → ", hop, n
        for (ip in ips) printf "%s(%d) ", ip, ips[ip]
        printf "⚠ ECMP\n"
    } else if (n == 1) {
        for (ip in ips) printf "  Hop %2d: %s (single path) ✓\n", hop, ip
    } else {
        printf "  Hop %2d: * (no response)\n", hop
    }
}' "$OUTDIR/traceroute.txt" | tee_report

# ============================================================
# Stage 2: MTR — per-hop loss and latency statistics
# ============================================================
header "Stage 2: MTR Per-Hop Analysis (100 probes)" | tee_report

log "Running MTR (this takes ~15s)..."
mtr -n -z -c 100 -r "$TARGET" 2>&1 | tee "$OUTDIR/mtr.txt" | tee_report

# Extract hops with notable loss or jitter
echo "" | tee_report
echo "--- Anomaly Hops ---" | tee_report
awk 'NR>1 && /^[| ]/ {
    if ($5+0 > 1.0 || $8+0 > 5.0) {
        printf "  %s  Loss=%.1f%%  Avg=%.1fms  StDev=%.1fms ⚠\n", $2, $5, $6, $8
    }
}' "$OUTDIR/mtr.txt" | tee_report
echo "(Flagged: loss > 1% or stdev > 5ms)" | tee_report

# ============================================================
# Stage 3: Multi-bandwidth iperf3 — reorder vs load correlation
# ============================================================
header "Stage 3: Bandwidth vs Reorder Correlation" | tee_report

for bw in 5M 10M 20M 30M; do
    log "Testing ${bw}bps (${DURATION}s reverse UDP)..."
    result=$(iperf3 -c "$TARGET" -u -b "$bw" -t "$DURATION" -R -p "$PORT" -J 2>&1)

    # Parse summary
    ooo=$(echo "$result" | python3 -c "
import json,sys
try:
    d = json.load(sys.stdin)
    if 'error' in d:
        print(f'ERROR: {d[\"error\"]}')
        sys.exit(0)
    streams = d.get('end',{}).get('streams',[])
    s = d.get('end',{}).get('sum',{})
    ooo = sum(st.get('udp',{}).get('out_of_order',0) for st in streams)
    pkts = s.get('packets',0)
    jitter = s.get('jitter_ms',0)
    loss_pct = s.get('lost_percent',0)
    bps = s.get('bits_per_second',0)/1e6
    ooo_pct = (ooo/pkts*100) if pkts>0 else 0
    print(f'{bps:.1f} Mbps | OoO: {ooo}/{pkts} ({ooo_pct:.1f}%) | Jitter: {jitter:.3f}ms | Loss: {loss_pct:.2f}%')
except Exception as e:
    print(f'Parse error: {e}')
" 2>&1)
    echo "  $bw → $ooo" | tee_report
    echo "$result" > "$OUTDIR/iperf3_${bw}.json"
done

echo "" | tee_report
echo "If OoO% increases with bandwidth → congestion-related reordering" | tee_report
echo "If OoO% is constant across bandwidths → path-level ECMP reordering" | tee_report

# ============================================================
# Stage 4: Flow hash test — per-packet vs per-flow balancing
# ============================================================
header "Stage 4: Flow Hash Analysis" | tee_report

log "Testing with different source ports to detect load balancing mode..."

for cport in 50001 50002 50003; do
    result=$(iperf3 -c "$TARGET" -u -b "$BW" -t "$DURATION" -R -p "$PORT" --cport "$cport" -J 2>&1)
    ooo=$(echo "$result" | python3 -c "
import json,sys
try:
    d = json.load(sys.stdin)
    if 'error' in d:
        print(f'ERROR: {d[\"error\"]}')
        sys.exit(0)
    streams = d.get('end',{}).get('streams',[])
    s = d.get('end',{}).get('sum',{})
    ooo = sum(st.get('udp',{}).get('out_of_order',0) for st in streams)
    pkts = s.get('packets',0)
    ooo_pct = (ooo/pkts*100) if pkts>0 else 0
    print(f'OoO: {ooo}/{pkts} ({ooo_pct:.1f}%)')
except Exception as e:
    print(f'Parse error: {e}')
" 2>&1)
    echo "  cport=$cport → $ooo" | tee_report
    echo "$result" > "$OUTDIR/iperf3_cport_${cport}.json"
done

echo "" | tee_report
echo "If all flows show similar OoO% → per-packet balancing (worst case)" | tee_report
echo "If OoO% varies significantly → per-flow balancing (each flow may hit different path)" | tee_report

# ============================================================
# Stage 5: Packet capture + reorder depth analysis
# ============================================================
header "Stage 5: Packet Capture & Reorder Depth Analysis" | tee_report

PCAP="$OUTDIR/capture.pcap"
log "Capturing packets for ${DURATION}s..."

# Start capture in background
tcpdump -i "$IFACE" -w "$PCAP" "host $TARGET and udp port $PORT" -c 100000 &>/dev/null &
TCPDUMP_PID=$!
sleep 1

# Run iperf3
iperf3 -c "$TARGET" -u -b "$BW" -t "$DURATION" -R -p "$PORT" -J > "$OUTDIR/iperf3_capture.json" 2>&1
sleep 2

# Stop capture
kill "$TCPDUMP_PID" 2>/dev/null || true
wait "$TCPDUMP_PID" 2>/dev/null || true

if [ -f "$PCAP" ] && [ -s "$PCAP" ]; then
    PCOUNT=$(tshark -r "$PCAP" -T fields -e frame.number 2>/dev/null | wc -l | tr -d ' ')
    log "Captured $PCOUNT packets"

    # Analyze inter-arrival time distribution
    log "Analyzing packet timing..."
    tshark -r "$PCAP" -T fields -e frame.time_delta_displayed -Y "udp" 2>/dev/null | \
        awk 'NF>0 && $1+0>0 {sum+=$1; sumsq+=$1*$1; n++; vals[n]=$1}
        END {
            if(n==0){print "No packets"; exit}
            avg=sum/n; var=sumsq/n - avg*avg; sd=sqrt(var>0?var:0)
            asort(vals)
            p50=vals[int(n*0.5)]; p95=vals[int(n*0.95)]; p99=vals[int(n*0.99)]
            printf "  Packets analyzed: %d\n", n
            printf "  Inter-arrival time:\n"
            printf "    Mean:  %.3f ms\n", avg*1000
            printf "    StDev: %.3f ms\n", sd*1000
            printf "    P50:   %.3f ms\n", p50*1000
            printf "    P95:   %.3f ms\n", p95*1000
            printf "    P99:   %.3f ms\n", p99*1000
            # Reorder proxy: if stdev > mean, significant reordering
            if (sd > avg*0.5) print "    ⚠ High variance — indicates reordering or buffering"
            else print "    ✓ Reasonable variance"
        }' | tee_report

    # TTL analysis — different TTLs indicate different paths
    echo "" | tee_report
    echo "--- TTL Distribution (different TTLs = different paths) ---" | tee_report
    tshark -r "$PCAP" -T fields -e ip.ttl -Y "udp and ip.src==$TARGET" 2>/dev/null | \
        sort | uniq -c | sort -rn | head -5 | \
        awk '{printf "  TTL=%s  count=%s\n", $2, $1}' | tee_report

    TTL_UNIQUE=$(tshark -r "$PCAP" -T fields -e ip.ttl -Y "udp and ip.src==$TARGET" 2>/dev/null | sort -u | wc -l | tr -d ' ')
    if [ "$TTL_UNIQUE" -gt 1 ]; then
        echo "  ⚠ Multiple TTL values ($TTL_UNIQUE) — confirms packets take different paths" | tee_report
    else
        echo "  ✓ Single TTL — packets likely on same path, reorder may be at endpoints" | tee_report
    fi
else
    warn "Packet capture failed or empty (need root?)" | tee_report
fi

# ============================================================
# Stage 6: Quantified Impact Assessment
# ============================================================
header "Stage 6: Quantified Impact on Real-Time Applications" | tee_report

# Parse the capture test result
python3 -c "
import json, math

# Load iperf3 result from capture run
try:
    with open('$OUTDIR/iperf3_capture.json') as f:
        d = json.load(f)
except:
    print('  Could not parse iperf3 result')
    exit(0)

if 'error' in d:
    print(f'  iperf3 error: {d[\"error\"]}')
    exit(0)

end = d.get('end', {})
s = end.get('sum', {})
streams = end.get('streams', [])

pkts = s.get('packets', 0)
lost = s.get('lost_packets', 0)
jitter_ms = s.get('jitter_ms', 0)
ooo = sum(st.get('udp', {}).get('out_of_order', 0) for st in streams)
ooo_pct = (ooo / pkts * 100) if pkts > 0 else 0
loss_pct = (lost / pkts * 100) if pkts > 0 else 0
bps = s.get('bits_per_second', 0)

print(f'  Throughput:      {bps/1e6:.1f} Mbps')
print(f'  Total packets:   {pkts}')
print(f'  Out-of-order:    {ooo} ({ooo_pct:.1f}%)')
print(f'  Packet loss:     {lost} ({loss_pct:.2f}%)')
print(f'  Jitter:          {jitter_ms:.3f} ms')
print()

# --- WebRTC Impact Model ---
print('--- WebRTC Impact Estimation ---')
print()

# Audio (Opus): 20ms frames, 50 pps
audio_pps = 50
audio_frame_ms = 20
# If a packet arrives out of order and exceeds jitter buffer, it's effectively lost
# Typical WebRTC jitter buffer: 20-200ms, adaptive, starts ~60ms
jitter_buf_ms = 60

# Estimate: reordered packets that arrive > jitter_buffer late are dropped
# Approximation: with X% reorder, ~Y% will exceed jitter buffer
# Based on empirical models: effective_loss = real_loss + ooo_pct * (jitter_ms / jitter_buf_ms)
effective_loss_audio = loss_pct + ooo_pct * min(jitter_ms / jitter_buf_ms, 1.0) * 0.3
print(f'  Audio (Opus, {audio_frame_ms}ms frames):')
print(f'    Jitter buffer:     {jitter_buf_ms}ms')
print(f'    Raw loss:          {loss_pct:.2f}%')
print(f'    OoO-induced loss:  ~{ooo_pct * min(jitter_ms/jitter_buf_ms, 1.0) * 0.3:.2f}%')
print(f'    Effective loss:    ~{effective_loss_audio:.2f}%')
if effective_loss_audio < 1:
    print(f'    Impact: ✓ Negligible — audio quality unaffected')
elif effective_loss_audio < 5:
    print(f'    Impact: ⚠ Minor — occasional artifacts possible')
elif effective_loss_audio < 10:
    print(f'    Impact: ⚠⚠ Noticeable — stuttering, reduced MOS score')
else:
    print(f'    Impact: ❌ Severe — frequent dropouts, poor call quality')

# R-factor / MOS estimation (simplified E-model ITU-T G.107)
# R = 93.2 - Id - Ie_eff
# Id (delay impairment) ≈ 0.024*d + 0.11*(d-177.3)*H(d-177.3) where d=one-way delay
# Simplified: assume 50ms one-way delay
ow_delay = 50  # ms, assumption
Id = 0.024 * ow_delay
Ie_eff = 0 + effective_loss_audio * 2.5  # rough: each 1% loss = 2.5 R-factor points
R = 93.2 - Id - Ie_eff
R = max(0, min(100, R))
if R > 0:
    MOS = 1 + 0.035*R + R*(R-60)*(100-R)*7e-6
else:
    MOS = 1.0
MOS = max(1.0, min(4.5, MOS))
print(f'    Estimated MOS:     {MOS:.1f}/5.0 (R={R:.0f})')
print()

# Video (VP8/VP9/H.264): 30fps, ~33ms frames, larger packets
video_fps = 30
print(f'  Video ({video_fps}fps, VP8/H.264):')
# Video is more sensitive: one lost/late packet can corrupt an entire frame
# Key frames are much larger (10-50 packets), losing any packet = frame loss
# Effective frame loss rate ≈ 1 - (1 - pkt_loss)^(pkts_per_frame)
avg_frame_pkts = max(1, (bps / 8 / video_fps) / 1200)  # assume 1200 byte packets
pkt_loss_rate = (loss_pct + ooo_pct * 0.1) / 100  # 10% of OoO become effectively lost for video
frame_loss = 1 - (1 - pkt_loss_rate) ** avg_frame_pkts
print(f'    Avg packets/frame: ~{avg_frame_pkts:.0f}')
print(f'    Pkt loss rate:     {pkt_loss_rate*100:.2f}%')
print(f'    Frame loss rate:   ~{frame_loss*100:.1f}%')
if frame_loss < 0.01:
    print(f'    Impact: ✓ Smooth playback')
elif frame_loss < 0.05:
    print(f'    Impact: ⚠ Occasional freeze/artifact ({frame_loss*video_fps:.1f} bad frames/sec)')
elif frame_loss < 0.15:
    print(f'    Impact: ⚠⚠ Visible degradation ({frame_loss*video_fps:.1f} bad frames/sec)')
else:
    print(f'    Impact: ❌ Severe — frequent freezes, unwatchable')

print()
print('--- Reorder Severity Classification ---')
if ooo_pct < 1:
    print(f'  {ooo_pct:.1f}% → NORMAL: Negligible impact on all applications')
elif ooo_pct < 5:
    print(f'  {ooo_pct:.1f}% → LOW: Acceptable for most real-time applications')
elif ooo_pct < 15:
    print(f'  {ooo_pct:.1f}% → MODERATE: May affect sensitive real-time apps')
elif ooo_pct < 30:
    print(f'  {ooo_pct:.1f}% → HIGH: Noticeable impact on VoIP/video')
else:
    print(f'  {ooo_pct:.1f}% → CRITICAL: Severe impact, immediate remediation needed')
" | tee_report

# ============================================================
# Summary
# ============================================================
header "Diagnosis Complete" | tee_report
echo "All results saved to: $OUTDIR/" | tee_report
echo "" | tee_report
echo "Files:" | tee_report
ls -la "$OUTDIR/" | awk '{print "  "$0}' | tee_report
echo "" | tee_report
echo "Full report: $REPORT" | tee_report
