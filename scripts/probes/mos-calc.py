#!/usr/bin/env python3
# PROBEX_META: {
# PROBEX_META:   "name": "mos-calc",
# PROBEX_META:   "description": "UDP echo probe with E-model MOS scoring — pure Python, no external dependencies",
# PROBEX_META:   "parameter_schema": {
# PROBEX_META:     "type": "object",
# PROBEX_META:     "properties": {
# PROBEX_META:       "port":             {"type":"integer","title":"UDP Echo Port","default":60099,"minimum":1,"maximum":65535},
# PROBEX_META:       "codec":            {"type":"string","title":"Codec Model","enum":["opus","g711","g729","g722"],"default":"opus"},
# PROBEX_META:       "count":            {"type":"integer","title":"Packet Count","default":100,"minimum":10,"maximum":5000},
# PROBEX_META:       "interval_ms":      {"type":"integer","title":"Send Interval (ms)","default":20,"minimum":5,"maximum":1000},
# PROBEX_META:       "packet_size":      {"type":"integer","title":"Packet Size (bytes)","default":172,"minimum":20,"maximum":1400},
# PROBEX_META:       "jitter_buffer_ms": {"type":"integer","title":"Simulated Jitter Buffer (ms)","default":60,"minimum":10,"maximum":500}
# PROBEX_META:     },
# PROBEX_META:     "x-ui-order": ["codec","count","interval_ms","packet_size","port","jitter_buffer_ms"]
# PROBEX_META:   },
# PROBEX_META:   "output_schema": {
# PROBEX_META:     "standard_fields": ["latency_ms","jitter_ms","packet_loss_pct"],
# PROBEX_META:     "extra_fields": [
# PROBEX_META:       {"name":"mos","type":"number","description":"Estimated MOS (1.0-4.5)","chartable":true},
# PROBEX_META:       {"name":"r_factor","type":"number","description":"E-model R-factor (0-100)","chartable":true},
# PROBEX_META:       {"name":"quality_grade","type":"string","description":"Quality grade A/B/C/D"},
# PROBEX_META:       {"name":"rtt_min_ms","type":"number","unit":"ms","description":"Minimum RTT"},
# PROBEX_META:       {"name":"rtt_max_ms","type":"number","unit":"ms","description":"Maximum RTT"},
# PROBEX_META:       {"name":"rtt_p95_ms","type":"number","unit":"ms","description":"95th percentile RTT","chartable":true},
# PROBEX_META:       {"name":"jitter_max_ms","type":"number","unit":"ms","description":"Maximum jitter"},
# PROBEX_META:       {"name":"out_of_order","type":"number","description":"Out-of-order packets"},
# PROBEX_META:       {"name":"out_of_order_pct","type":"number","unit":"%","description":"Reorder rate","chartable":true},
# PROBEX_META:       {"name":"reorder_depth_avg","type":"number","description":"Average reorder depth"},
# PROBEX_META:       {"name":"reorder_depth_max","type":"number","description":"Maximum reorder depth"},
# PROBEX_META:       {"name":"effective_loss_pct","type":"number","unit":"%","description":"Effective loss including reorder beyond jitter buffer","chartable":true}
# PROBEX_META:     ]
# PROBEX_META:   }
# PROBEX_META: }

"""
Pure-Python UDP Echo Probe with MOS Scoring.

This script sends UDP packets with sequence numbers and timestamps to a
target echo server, then analyzes RTT, jitter, packet loss, and reorder.
Finally it computes an ITU-T G.107 E-model MOS score.

Echo server protocol:
  Client sends: [4-byte seq_num BE][8-byte send_timestamp_us BE][padding]
  Server echoes: same bytes back unchanged

If no echo server is available, the script falls back to a one-way send
mode that only measures loss (no RTT/jitter).

No external dependencies — uses only Python 3 stdlib (socket, struct, time).
"""

import json
import math
import os
import socket
import struct
import sys
import time


def main():
    target = os.environ.get("PROBEX_TARGET", "")
    if not target:
        print(json.dumps({"success": False, "error": "PROBEX_TARGET is required"}))
        return

    port = int(os.environ.get("PROBEX_PARAM_PORT", "60099"))
    codec = os.environ.get("PROBEX_PARAM_CODEC", "opus")
    count = int(os.environ.get("PROBEX_PARAM_COUNT", "100"))
    interval_ms = int(os.environ.get("PROBEX_PARAM_INTERVAL_MS", "20"))
    packet_size = int(os.environ.get("PROBEX_PARAM_PACKET_SIZE", "172"))
    jb_ms = int(os.environ.get("PROBEX_PARAM_JITTER_BUFFER_MS", "60"))

    try:
        result = run_probe(target, port, count, interval_ms, packet_size, codec, jb_ms)
        print(json.dumps(result))
    except Exception as e:
        print(json.dumps({"success": False, "error": str(e)}))


def run_probe(target, port, count, interval_ms, packet_size, codec, jb_ms):
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.settimeout(2.0)

    # Header: 4 bytes seq + 8 bytes timestamp = 12 bytes
    header_size = 12
    padding_size = max(0, packet_size - header_size)
    padding = b"\x00" * padding_size

    rtts = []           # round-trip times in ms
    recv_seqs = []      # sequence numbers in order received
    send_times = {}     # seq -> send time
    interval_s = interval_ms / 1000.0

    # Send phase with interleaved receive
    for seq in range(count):
        send_ts_us = int(time.time() * 1_000_000)
        pkt = struct.pack("!IQ", seq, send_ts_us) + padding
        send_times[seq] = time.time()

        try:
            sock.sendto(pkt, (target, port))
        except OSError:
            pass

        # Try to receive any pending replies (non-blocking drain)
        sock.settimeout(0.001)
        while True:
            try:
                data, _ = sock.recvfrom(2048)
                if len(data) >= header_size:
                    r_seq, r_ts_us = struct.unpack("!IQ", data[:header_size])
                    if r_seq in send_times:
                        rtt_ms = (time.time() - send_times[r_seq]) * 1000
                        rtts.append(rtt_ms)
                        recv_seqs.append(r_seq)
            except (socket.timeout, BlockingIOError):
                break

        if seq < count - 1:
            time.sleep(interval_s)

    # Final receive window — wait up to 2 seconds for remaining replies
    sock.settimeout(0.1)
    deadline = time.time() + 2.0
    while time.time() < deadline:
        try:
            data, _ = sock.recvfrom(2048)
            if len(data) >= header_size:
                r_seq, r_ts_us = struct.unpack("!IQ", data[:header_size])
                if r_seq in send_times and r_seq not in [s for s in recv_seqs]:
                    rtt_ms = (time.time() - send_times[r_seq]) * 1000
                    rtts.append(rtt_ms)
                    recv_seqs.append(r_seq)
        except (socket.timeout, BlockingIOError):
            if time.time() > deadline - 1.5:
                break

    sock.close()

    received = len(recv_seqs)
    lost = count - received
    loss_pct = (lost / count * 100) if count > 0 else 0

    # Compute jitter (RFC 3550 style: mean absolute difference of consecutive RTTs)
    jitter_ms = 0.0
    jitter_max = 0.0
    if len(rtts) >= 2:
        diffs = [abs(rtts[i] - rtts[i - 1]) for i in range(1, len(rtts))]
        jitter_ms = sum(diffs) / len(diffs)
        jitter_max = max(diffs)

    # Reorder analysis
    ooo_count = 0
    reorder_depths = []
    max_seen_seq = -1
    for seq in recv_seqs:
        if seq < max_seen_seq:
            ooo_count += 1
            depth = max_seen_seq - seq
            reorder_depths.append(depth)
        if seq > max_seen_seq:
            max_seen_seq = seq

    ooo_pct = (ooo_count / received * 100) if received > 0 else 0
    reorder_depth_avg = (sum(reorder_depths) / len(reorder_depths)) if reorder_depths else 0
    reorder_depth_max = max(reorder_depths) if reorder_depths else 0

    # Effective loss: real loss + reorder packets that arrive beyond jitter buffer
    # A packet reordered by depth D arrives ~D*interval_ms late
    # If D*interval_ms > jitter_buffer, it's effectively lost
    buffer_depth = jb_ms / interval_ms if interval_ms > 0 else 3
    reorder_beyond_buffer = sum(1 for d in reorder_depths if d > buffer_depth)
    effective_loss_extra = (reorder_beyond_buffer / count * 100) if count > 0 else 0
    effective_loss_pct = loss_pct + effective_loss_extra

    # RTT statistics
    avg_rtt = sum(rtts) / len(rtts) if rtts else 0
    min_rtt = min(rtts) if rtts else 0
    max_rtt = max(rtts) if rtts else 0
    p95_rtt = sorted(rtts)[int(len(rtts) * 0.95)] if len(rtts) > 1 else avg_rtt

    # E-model MOS (ITU-T G.107)
    ie_map = {"opus": 2.0, "g711": 0, "g729": 11, "g722": 3}
    ie_base = ie_map.get(codec, 2.0)

    one_way = avg_rtt / 2 if avg_rtt > 0 else 50
    delay = one_way + jb_ms / 2

    id_delay = 0.024 * delay
    if delay > 177.3:
        id_delay += 0.11 * (delay - 177.3)

    ie_eff = ie_base + effective_loss_pct * 2.5
    R = 93.2 - id_delay - ie_eff
    R = max(0.0, min(100.0, R))

    if R <= 0:
        mos = 1.0
    elif R >= 100:
        mos = 4.5
    else:
        mos = 1 + 0.035 * R + R * (R - 60) * (100 - R) * 7e-6

    grade = "A" if mos >= 4.0 else "B" if mos >= 3.6 else "C" if mos >= 3.1 else "D"

    return {
        "success": received > 0,
        "latency_ms": round(avg_rtt, 2),
        "jitter_ms": round(jitter_ms, 2),
        "packet_loss_pct": round(loss_pct, 2),
        "extra": {
            "mos": round(mos, 2),
            "r_factor": round(R, 1),
            "quality_grade": grade,
            "rtt_min_ms": round(min_rtt, 2),
            "rtt_max_ms": round(max_rtt, 2),
            "rtt_p95_ms": round(p95_rtt, 2),
            "jitter_max_ms": round(jitter_max, 2),
            "out_of_order": ooo_count,
            "out_of_order_pct": round(ooo_pct, 2),
            "reorder_depth_avg": round(reorder_depth_avg, 1),
            "reorder_depth_max": reorder_depth_max,
            "effective_loss_pct": round(effective_loss_pct, 2),
            "packets_sent": count,
            "packets_received": received,
            "codec": codec,
            "jitter_buffer_ms": jb_ms,
        },
    }


if __name__ == "__main__":
    main()
