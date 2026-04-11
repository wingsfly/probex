#!/bin/bash
# PROBEX_META: {
# PROBEX_META:   "name": "stun-turn",
# PROBEX_META:   "description": "STUN/TURN connectivity probe — tests ICE server reachability and response time",
# PROBEX_META:   "parameter_schema": {
# PROBEX_META:     "type": "object",
# PROBEX_META:     "properties": {
# PROBEX_META:       "mode":           {"type":"string","title":"Test Mode","enum":["stun","turn","both"],"default":"stun"},
# PROBEX_META:       "port":           {"type":"integer","title":"Server Port","default":3478,"minimum":1,"maximum":65535},
# PROBEX_META:       "transport":      {"type":"string","title":"Transport","enum":["udp","tcp"],"default":"udp"},
# PROBEX_META:       "turn_username":  {"type":"string","title":"TURN Username"},
# PROBEX_META:       "turn_password":  {"type":"string","title":"TURN Password","x-ui-widget":"password"},
# PROBEX_META:       "count":          {"type":"integer","title":"Probe Count","default":5,"minimum":1,"maximum":50}
# PROBEX_META:     },
# PROBEX_META:     "x-ui-order": ["mode","port","transport","turn_username","turn_password","count"]
# PROBEX_META:   },
# PROBEX_META:   "output_schema": {
# PROBEX_META:     "standard_fields": ["latency_ms"],
# PROBEX_META:     "extra_fields": [
# PROBEX_META:       {"name":"stun_reachable","type":"boolean","description":"STUN server reachable"},
# PROBEX_META:       {"name":"stun_rtt_ms","type":"number","unit":"ms","description":"STUN binding request RTT","chartable":true},
# PROBEX_META:       {"name":"mapped_address","type":"string","description":"NAT mapped address (server-reflexive)"},
# PROBEX_META:       {"name":"turn_reachable","type":"boolean","description":"TURN server reachable"},
# PROBEX_META:       {"name":"turn_rtt_ms","type":"number","unit":"ms","description":"TURN allocate request RTT","chartable":true},
# PROBEX_META:       {"name":"relay_address","type":"string","description":"TURN relay address"}
# PROBEX_META:     ]
# PROBEX_META:   }
# PROBEX_META: }

# STUN/TURN Connectivity Probe
# Tests STUN binding and TURN allocation using stunclient or turnutils_uclient

# Ensure common tool paths are available
export PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:$PATH"

set -euo pipefail

TARGET="${PROBEX_TARGET:-}"
MODE="${PROBEX_PARAM_MODE:-stun}"
PORT="${PROBEX_PARAM_PORT:-3478}"
TRANSPORT="${PROBEX_PARAM_TRANSPORT:-udp}"
USERNAME="${PROBEX_PARAM_TURN_USERNAME:-}"
PASSWORD="${PROBEX_PARAM_TURN_PASSWORD:-}"
COUNT="${PROBEX_PARAM_COUNT:-5}"

if [ -z "$TARGET" ]; then
  echo '{"success":false,"error":"PROBEX_TARGET is required"}'
  exit 1
fi

STUN_OK=false
STUN_RTT=0
MAPPED=""
TURN_OK=false
TURN_RTT=0
RELAY=""

# STUN test using simple UDP probe
test_stun() {
  local total_ms=0
  local success_count=0

  for i in $(seq 1 "$COUNT"); do
    local start_ms=$(($(date +%s%N) / 1000000))

    # Use stunclient if available, otherwise use netcat for basic connectivity
    if command -v stunclient &>/dev/null; then
      local result=$(stunclient "$TARGET" "$PORT" 2>&1)
      if echo "$result" | grep -q "Mapped address"; then
        MAPPED=$(echo "$result" | grep "Mapped address" | awk '{print $NF}')
        success_count=$((success_count + 1))
      fi
    else
      # Fallback: simple UDP reachability test
      if echo -ne '\x00\x01\x00\x00\x21\x12\xa4\x42\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00' | \
         timeout 3 nc -u -w 2 "$TARGET" "$PORT" 2>/dev/null | head -c 1 | grep -q .; then
        success_count=$((success_count + 1))
      fi
    fi

    local end_ms=$(($(date +%s%N) / 1000000))
    total_ms=$((total_ms + end_ms - start_ms))
  done

  if [ "$success_count" -gt 0 ]; then
    STUN_OK=true
    STUN_RTT=$((total_ms / COUNT))
  fi
}

# TURN test using turnutils_uclient
test_turn() {
  if ! command -v turnutils_uclient &>/dev/null; then
    # Fallback: just test TCP/UDP connectivity to TURN port
    local start_ms=$(($(date +%s%N) / 1000000))
    if timeout 5 bash -c "echo '' > /dev/tcp/$TARGET/$PORT" 2>/dev/null || \
       timeout 5 bash -c "echo '' | nc -u -w 2 $TARGET $PORT" 2>/dev/null; then
      TURN_OK=true
    fi
    local end_ms=$(($(date +%s%N) / 1000000))
    TURN_RTT=$((end_ms - start_ms))
    return
  fi

  local turn_args="-e $TARGET -n $PORT"
  [ -n "$USERNAME" ] && turn_args="$turn_args -u $USERNAME"
  [ -n "$PASSWORD" ] && turn_args="$turn_args -w $PASSWORD"

  local start_ms=$(($(date +%s%N) / 1000000))
  local result=$(turnutils_uclient $turn_args -T 2>&1) || true
  local end_ms=$(($(date +%s%N) / 1000000))

  if echo "$result" | grep -qi "success\|allocat"; then
    TURN_OK=true
    TURN_RTT=$((end_ms - start_ms))
    RELAY=$(echo "$result" | grep -i "relay" | head -1 | awk '{print $NF}' || true)
  fi
}

# Execute based on mode
case "$MODE" in
  stun) test_stun ;;
  turn) test_turn ;;
  both) test_stun; test_turn ;;
esac

# Determine overall success
SUCCESS=false
LATENCY=0
if [ "$MODE" = "stun" ] && [ "$STUN_OK" = "true" ]; then
  SUCCESS=true; LATENCY=$STUN_RTT
elif [ "$MODE" = "turn" ] && [ "$TURN_OK" = "true" ]; then
  SUCCESS=true; LATENCY=$TURN_RTT
elif [ "$MODE" = "both" ] && ([ "$STUN_OK" = "true" ] || [ "$TURN_OK" = "true" ]); then
  SUCCESS=true
  [ "$STUN_OK" = "true" ] && LATENCY=$STUN_RTT || LATENCY=$TURN_RTT
fi

cat <<EOF
{
  "success": $SUCCESS,
  "latency_ms": $LATENCY,
  "extra": {
    "stun_reachable": $STUN_OK,
    "stun_rtt_ms": $STUN_RTT,
    "mapped_address": "$MAPPED",
    "turn_reachable": $TURN_OK,
    "turn_rtt_ms": $TURN_RTT,
    "relay_address": "$RELAY",
    "mode": "$MODE",
    "transport": "$TRANSPORT"
  }
}
EOF
