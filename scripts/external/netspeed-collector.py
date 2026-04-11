#!/usr/bin/env python3
"""
External Network Interface Traffic Monitor for ProbeX.

Monitors real-time network interface throughput by sampling OS byte counters.
NOT a speed test — this measures actual traffic flowing through the NIC.

Requires: pip install psutil

Usage:
    python3 netspeed-collector.py [--controller http://localhost:8080] [--interval 5] [--iface auto]
"""

import argparse
import json
import socket
import sys
import time
import urllib.request
import urllib.error

try:
    import psutil
except ImportError:
    print("ERROR: psutil is required. Install with: pip3 install psutil", flush=True)
    sys.exit(1)


def get_interface_bytes(iface: str) -> tuple:
    """Returns (rx_bytes, tx_bytes) via psutil — fast, cross-platform, no subprocess."""
    counters = psutil.net_io_counters(pernic=True)
    if iface in counters:
        c = counters[iface]
        return c.bytes_recv, c.bytes_sent
    return 0, 0


def detect_active_interface() -> str:
    """Auto-detect the interface with the most traffic."""
    counters = psutil.net_io_counters(pernic=True)
    best, best_total = "en0", 0
    for name, c in counters.items():
        if name == "lo0" or name.startswith("lo"):
            continue
        total = c.bytes_recv + c.bytes_sent
        if total > best_total:
            best_total = total
            best = name
    return best


def register_probe(base: str) -> bool:
    payload = {
        "name": "netspeed",
        "description": "Network interface traffic monitor — real-time NIC rx/tx throughput",
        "parameter_schema": {
            "type": "object",
            "properties": {
                "interface": {"type": "string", "title": "Network Interface", "default": "auto"},
                "interval_sec": {"type": "integer", "title": "Sample Interval (sec)", "default": 5, "minimum": 1},
            },
        },
        "output_schema": {
            "standard_fields": ["download_bps", "upload_bps"],
            "extra_fields": [
                {"name": "rx_mbps", "type": "number", "unit": "Mbps", "description": "Download throughput", "chartable": True},
                {"name": "tx_mbps", "type": "number", "unit": "Mbps", "description": "Upload throughput", "chartable": True},
                {"name": "rx_bytes_delta", "type": "number", "description": "Bytes received in interval"},
                {"name": "tx_bytes_delta", "type": "number", "description": "Bytes sent in interval"},
                {"name": "interface", "type": "string", "description": "Monitored interface"},
            ],
        },
    }
    data = json.dumps(payload).encode()
    req = urllib.request.Request(f"{base}/probes/register", data=data,
                                 headers={"Content-Type": "application/json"}, method="POST")
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            print("[OK] Registered", flush=True)
            return True
    except urllib.error.HTTPError:
        print("[OK] Already registered", flush=True)
        return True
    except Exception as e:
        print(f"[ERR] Register: {e}", flush=True)
        return False


def push(base: str, result: dict) -> bool:
    payload = json.dumps({"agent_id": f"netspeed-{socket.gethostname()}", "results": [result]}).encode()
    req = urllib.request.Request(f"{base}/probes/netspeed/push", data=payload,
                                 headers={"Content-Type": "application/json"}, method="POST")
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            resp.read()
            return True
    except Exception as e:
        print(f"[ERR] Push: {e}", flush=True)
        return False


def main():
    parser = argparse.ArgumentParser(description="ProbeX NIC Traffic Monitor")
    parser.add_argument("--controller", default="http://localhost:8080")
    parser.add_argument("--interval", type=int, default=5)
    parser.add_argument("--iface", default="auto")
    args = parser.parse_args()

    iface = detect_active_interface() if args.iface == "auto" else args.iface
    base = f"{args.controller}/api/v1"

    print(f"NIC Traffic Monitor", flush=True)
    print(f"  Interface:  {iface}", flush=True)
    print(f"  Interval:   {args.interval}s", flush=True)
    print(f"  Controller: {args.controller}", flush=True)
    print(flush=True)

    register_probe(base)

    prev_rx, prev_tx = get_interface_bytes(iface)
    prev_t = time.time()
    print(f"Initial: rx={prev_rx/1e9:.1f}GB  tx={prev_tx/1e9:.1f}GB", flush=True)
    print(flush=True)

    while True:
        time.sleep(args.interval)
        try:
            now = time.time()
            rx, tx = get_interface_bytes(iface)
            dt = max(now - prev_t, 0.1)

            drx = max(0, rx - prev_rx)
            dtx = max(0, tx - prev_tx)
            rx_bps = drx * 8 / dt
            tx_bps = dtx * 8 / dt

            prev_rx, prev_tx, prev_t = rx, tx, now

            result = {
                "success": True,
                "download_bps": round(rx_bps),
                "upload_bps": round(tx_bps),
                "extra": {
                    "rx_mbps": round(rx_bps / 1e6, 3),
                    "tx_mbps": round(tx_bps / 1e6, 3),
                    "rx_bytes_delta": drx,
                    "tx_bytes_delta": dtx,
                    "interface": iface,
                },
            }

            ok = push(base, result)
            tag = "OK" if ok else "FAIL"
            print(f"  [{time.strftime('%H:%M:%S')}] {tag}  RX={rx_bps/1e6:8.3f} Mbps  TX={tx_bps/1e6:8.3f} Mbps", flush=True)

        except KeyboardInterrupt:
            print("\nStopped.", flush=True)
            break
        except Exception as e:
            print(f"  [{time.strftime('%H:%M:%S')}] ERR: {e}", flush=True)


if __name__ == "__main__":
    main()
