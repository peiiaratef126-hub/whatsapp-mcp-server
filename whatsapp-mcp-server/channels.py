"""WhatsApp Channel (Newsletter) MCP tools."""

from typing import Any
from mcp.server.fastmcp import FastMCP
from client import BridgeClient, BridgeError


def register_channels_tools(mcp: FastMCP, client: BridgeClient) -> None:
    """Register channel management tools with FastMCP server."""

    @mcp.tool()
    def create_channel(name: str, description: str = "") -> dict[str, Any]:
        """Create a new WhatsApp broadcast channel (@newsletter).

        Channels are broadcast-only feeds where channel admins post updates and
        followers can only read and react. The resulting channel JID will have
        the '@newsletter' suffix.

        Args:
            name: The name/title of the channel.
            description: Optional text description of the channel's topic or purpose.

        Returns:
            Dictionary containing the newly created channel's JID, name, and description.
        """
        if not name or not name.strip():
            return {"error": "channel name cannot be empty"}

        clean_name = name.strip()
        clean_desc = description.strip() if description else ""

        try:
            res = client.create_channel(clean_name, clean_desc)
            return res
        except BridgeError as e:
            # Handle version / capability limitation if unsupported by bridge
            if "not supported" in str(e).lower() or e.status_code == 501:
                return {
                    "error": "Channel (newsletter) creation is not supported by the current library version.",
                    "status_code": 501,
                }
            return {"error": str(e), "channel_name": clean_name}
