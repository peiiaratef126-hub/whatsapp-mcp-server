"""Pytest configuration and sys.path setup."""

import sys
from pathlib import Path

# Add the parent package directory to sys.path
package_dir = Path(__file__).resolve().parent.parent
if str(package_dir) not in sys.path:
    sys.path.insert(0, str(package_dir))
