"""Pytest configuration and sys.path setup for root tests."""

import sys
from pathlib import Path

root_dir = Path(__file__).resolve().parent.parent
server_dir = root_dir / "whatsapp-mcp-server"
if str(server_dir) not in sys.path:
    sys.path.insert(0, str(server_dir))
if str(root_dir) not in sys.path:
    sys.path.insert(0, str(root_dir))
