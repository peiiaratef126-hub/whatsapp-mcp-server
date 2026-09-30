"""Main entry point for running the WhatsApp MCP server."""

import sys

from config import (
    SENTRY_DSN,
    SENTRY_ENVIRONMENT,
    SENTRY_RELEASE,
    SENTRY_TRACES_SAMPLE_RATE,
)
from server import create_mcp_server


def init_sentry() -> None:
    """Initialize Sentry SDK if SENTRY_DSN environment variable is set."""
    if SENTRY_DSN:
        try:
            import sentry_sdk

            sentry_sdk.init(
                dsn=SENTRY_DSN,
                environment=SENTRY_ENVIRONMENT,
                release=SENTRY_RELEASE,
                traces_sample_rate=SENTRY_TRACES_SAMPLE_RATE,
            )
            print("Sentry initialized for WhatsApp MCP Server.", file=sys.stderr)
        except Exception as e:
            print(f"Warning: Failed to initialize Sentry: {e}", file=sys.stderr)


def main() -> None:
    """Run the FastMCP server over standard I/O."""
    init_sentry()
    mcp = create_mcp_server()
    print("Starting WhatsApp MCP server on stdio transport...", file=sys.stderr)
    mcp.run()


if __name__ == "__main__":
    main()
