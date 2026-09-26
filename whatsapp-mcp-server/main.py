"""Main entry point for running the WhatsApp MCP server."""

import sys
from server import create_mcp_server


def main() -> None:
    """Run the FastMCP server over standard I/O."""
    mcp = create_mcp_server()
    print("Starting WhatsApp MCP server on stdio transport...", file=sys.stderr)
    mcp.run()


if __name__ == "__main__":
    main()
