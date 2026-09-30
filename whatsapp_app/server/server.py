"""FastMCP server initialization and wiring."""

from client import BridgeClient
from mcp.server.fastmcp import FastMCP


def create_mcp_server(client: BridgeClient | None = None) -> FastMCP:
    """Create and configure the WhatsApp FastMCP server with all tools registered."""
    mcp = FastMCP("whatsapp-mcp-server")
    bridge_client = client or BridgeClient()

    # Tool registration will be imported and wired as modules are implemented
    # contacts
    try:
        from contacts import register_contacts_tools

        register_contacts_tools(mcp, bridge_client)
    except ImportError:
        pass

    # messages
    try:
        from messages import register_messages_tools

        register_messages_tools(mcp, bridge_client)
    except ImportError:
        pass

    # media
    try:
        from media import register_media_tools

        register_media_tools(mcp, bridge_client)
    except ImportError:
        pass

    # groups
    try:
        from groups import register_groups_tools

        register_groups_tools(mcp, bridge_client)
    except ImportError:
        pass

    # channels
    try:
        from channels import register_channels_tools

        register_channels_tools(mcp, bridge_client)
    except ImportError:
        pass

    # admin
    try:
        from admin import register_admin_tools

        register_admin_tools(mcp, bridge_client)
    except ImportError:
        pass

    return mcp
