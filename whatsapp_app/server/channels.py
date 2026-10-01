"""WhatsApp Channel (Newsletter) MCP tools."""

from typing import Any

from client import BridgeClient, BridgeError
from mcp.server.fastmcp import FastMCP
from mcp.types import ToolAnnotations

_client: BridgeClient | None = None


def get_client() -> BridgeClient:
    """Get active bridge client, creating a default one if not set."""
    global _client
    if _client is None:
        _client = BridgeClient()
    return _client


def set_client(client: BridgeClient) -> None:
    """Set active bridge client."""
    global _client
    _client = client


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

    client = get_client()
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


def register_channels_tools(mcp: FastMCP, client: BridgeClient | None = None) -> None:
    """Register channel management tools with FastMCP server."""
    if client is not None:
        set_client(client)
    mcp.tool(
        annotations=ToolAnnotations(
            readOnlyHint=False,
            destructiveHint=False,
            idempotentHint=False,
            openWorldHint=True,
        )
    )(create_channel)
