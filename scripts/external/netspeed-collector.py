#!/usr/bin/env python3
"""
External Network Interface Traffic Monitor for ProbeX.

Monitors real-time network interface throughput by sampling OS byte counters.
NOT a speed test — this measures actual traffic flowing through the NIC.

Requires: pip install psutil

Usage:
    python3 netspeed-collector.py [options]

Options:
    --controller URL    ProbeX server (default: http://localhost:8080)
    --interval SEC      Sampling interval in seconds (default: 5)
    --iface NAME        Network interface to monitor (default: auto-detect)
    --id ID             Instance ID for this collector. Determines the probe name
                        registered in ProbeX as "netspeed-{id}".
                        Default: auto-generated as "{hostname}-{iface}"

Examples:
    # Auto ID: registers as netspeed-myhost-en0
    python3 netspeed-collector.py

    # Manual ID: registers as netspeed-office-gw
    python3 netspeed-collector.py --id office-gw

    # Remote hub with specific interface
    python3 netspeed-collector.py --controller http://192.168.1.100:8080 --iface eth0
"""

import argparse
import json
import platform
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


def get_local_ip(iface: str) -> str:
    """Get the IPv4 address of the given interface, fallback to hostname resolution."""
    addrs = psutil.net_if_addrs()
    if iface in addrs:
        for addr in addrs[iface]:
            if addr.family == socket.AF_INET:
                return addr.address
    # Fallback: connect to a public IP to find default route IP
    try:
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        s.connect(("8.8.8.8", 80))
        ip = s.getsockname()[0]
        s.close()
        return ip
    except Exception:
        return "unknown"


def get_host_info(iface: str) -> dict:
    """Collect host metadata for reporting."""
    return {
        "hostname": socket.gethostname(),
        "ip": get_local_ip(iface),
        "os": f"{platform.system()} {platform.release()}",
        "interface": iface,
    }


def register_probe(base: str, probe_name: str, host_info: dict) -> bool:
    payload = {
        "name": probe_name,
        "description": f"NIC traffic monitor on {host_info['hostname']} ({host_info['ip']}, {host_info['interface']})",
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
                {"name": "hostname", "type": "string", "description": "Source hostname"},
                {"name": "ip", "type": "string", "description": "Source IP address"},
                {"name": "os", "type": "string", "description": "Operating system"},
            ],
        },
    }
    data = json.dumps(payload).encode()
    req = urllib.request.Request(f"{base}/probes/register", data=data,
                                 headers={"Content-Type": "application/json"}, method="POST")
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            print(f"[OK] Registered probe: {probe_name}", flush=True)
            return True
    except urllib.error.HTTPError:
        print(f"[OK] Already registered: {probe_name}", flush=True)
        return True
    except Exception as e:
        print(f"[ERR] Register: {e}", flush=True)
        return False


def push(base: str, probe_name: str, agent_id: str, result: dict) -> bool:
    payload = json.dumps({"agent_id": agent_id, "results": [result]}).encode()
    req = urllib.request.Request(f"{base}/probes/{probe_name}/push", data=payload,
                                 headers={"Content-Type": "application/json"}, method="POST")
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            resp.read()
            return True
    except Exception as e:
        print(f"[ERR] Push: {e}", flush=True)
        return False


def main():
    parser = argparse.ArgumentParser(
        description="ProbeX NIC Traffic Monitor",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="Each instance registers as a separate probe (netspeed-{id}) in ProbeX,\n"
               "so multiple hosts or interfaces show as independent tasks in Results.",
    )
    parser.add_argument("--controller", default="http://localhost:8080",
                        help="ProbeX server URL (default: http://localhost:8080)")
    parser.add_argument("--interval", type=int, default=5,
                        help="Sampling interval in seconds (default: 5)")
    parser.add_argument("--iface", default="auto",
                        help="Network interface to monitor (default: auto-detect)")
    parser.add_argument("--id", default="",
                        help="Instance ID. Probe registers as netspeed-{id}. "
                             "Default: auto-generated as {hostname}-{iface}")
    args = parser.parse_args()

    iface = detect_active_interface() if args.iface == "auto" else args.iface
    hostname = socket.gethostname()

    # Determine instance ID and probe name
    instance_id = args.id if args.id else f"{hostname}-{iface}"
    probe_name = f"netspeed-{instance_id}"
    agent_id = f"{probe_name}@{hostname}"

    host_info = get_host_info(iface)
    base = f"{args.controller}/api/v1"

    print(f"NIC Traffic Monitor", flush=True)
    print(f"  Probe:      {probe_name}", flush=True)
    print(f"  Agent:      {agent_id}", flush=True)
    print(f"  Interface:  {iface}", flush=True)
    print(f"  Host:       {hostname} ({host_info['ip']})", flush=True)
    print(f"  OS:         {host_info['os']}", flush=True)
    print(f"  Interval:   {args.interval}s", flush=True)
    print(f"  Controller: {args.controller}", flush=True)
    print(flush=True)

    register_probe(base, probe_name, host_info)

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
                    "hostname": hostname,
                    "ip": host_info["ip"],
                    "os": host_info["os"],
                },
            }

            ok = push(base, probe_name, agent_id, result)
            tag = "OK" if ok else "FAIL"
            print(f"  [{time.strftime('%H:%M:%S')}] {tag}  RX={rx_bps/1e6:8.3f} Mbps  TX={tx_bps/1e6:8.3f} Mbps", flush=True)

        except KeyboardInterrupt:
            print("\nStopped.", flush=True)
            break
        except Exception as e:
            print(f"  [{time.strftime('%H:%M:%S')}] ERR: {e}", flush=True)


if __name__ == "__main__":
    main()
