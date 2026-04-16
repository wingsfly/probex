"""
Persistent 8-character hex node ID for ProbeX external scripts.

Generates a unique node ID on first call and persists it to
~/.probex/node_id. Subsequent calls return the same ID, enabling
the ProbeX server to correlate results from the same physical node
even when user-chosen probe names collide.

Usage:
    from probex_nodeid import get_node_id
    nid = get_node_id()           # uses ~/.probex/node_id
    nid = get_node_id("/tmp/px")  # custom data dir
"""

import os
import secrets

_ID_BYTES = 4  # 4 bytes = 8 hex chars
_FILE_NAME = "node_id"
_cached = None


def get_node_id(data_dir: str = "") -> str:
    """Return the persistent node ID, generating one if it does not exist.

    Args:
        data_dir: Directory to store the ID file. Defaults to ~/.probex.
    Returns:
        8-character lowercase hex string (e.g. "a3f0b12c").
    """
    global _cached
    if _cached is not None:
        return _cached

    if not data_dir:
        data_dir = os.path.join(os.path.expanduser("~"), ".probex")

    path = os.path.join(data_dir, _FILE_NAME)

    # Try to read existing ID
    try:
        with open(path) as f:
            nid = f.read().strip()
        if len(nid) == _ID_BYTES * 2 and all(c in "0123456789abcdef" for c in nid):
            _cached = nid
            return nid
    except FileNotFoundError:
        pass

    # Generate new ID
    nid = secrets.token_hex(_ID_BYTES)

    # Persist
    os.makedirs(data_dir, exist_ok=True)
    with open(path, "w") as f:
        f.write(nid + "\n")

    _cached = nid
    return nid


def reset():
    """Clear the in-memory cache (useful for testing)."""
    global _cached
    _cached = None
