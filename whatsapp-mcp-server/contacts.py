"""Contact-related MCP tools."""

from typing import Any

from mcp.server.fastmcp import FastMCP
from mcp.types import ToolAnnotations

from client import BridgeClient, BridgeError

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


def search_contacts(query: str) -> list[dict[str, Any]]:
    """Search WhatsApp contacts by name, push name, phone number, or JID.

    Args:
        query: Name, partial name, phone number, or JID to search for.

    Returns:
        A list of matching contact records containing JID, phone number, name, and push name.
    """
    if not query or not query.strip():
        return []
    client = get_client()
    try:
        return client.search_contacts(query.strip())
    except BridgeError as e:
        return [{"error": str(e)}]


def get_direct_chat_by_contact(contact: str) -> dict[str, Any]:
    """Retrieve the direct (1-on-1) chat metadata for a specific contact.

    Args:
        contact: Contact phone number (e.g. +1234567890), JID (e.g. 1234@s.whatsapp.net), or name.

    Returns:
        Direct chat details including chat JID, name, and last interaction timestamp.
    """
    if not contact or not contact.strip():
        return {"error": "contact identifier cannot be empty"}
    client = get_client()
    try:
        return client.get_direct_chat_by_contact(contact.strip())
    except BridgeError as e:
        return {"error": str(e), "contact": contact}


def get_contact_chats(contact: str) -> list[dict[str, Any]]:
    """Retrieve all chats (direct 1-on-1 chats and group chats) involving a specific contact.

    Args:
        contact: Contact phone number, JID, or name identifier.

    Returns:
        A list of chat records where the contact is a participant.
    """
    if not contact or not contact.strip():
        return [{"error": "contact identifier cannot be empty"}]
    client = get_client()
    try:
        return client.get_contact_chats(contact.strip())
    except BridgeError as e:
        return [{"error": str(e), "contact": contact}]


def register_contacts_tools(mcp: FastMCP, client: BridgeClient | None = None) -> None:
    """Register contacts tools with FastMCP server."""
    if client is not None:
        set_client(client)

    contact_annotations = ToolAnnotations(
        readOnlyHint=True,
        destructiveHint=False,
        idempotentHint=True,
        openWorldHint=True,
    )
    mcp.tool(annotations=contact_annotations)(search_contacts)
    mcp.tool(annotations=contact_annotations)(get_direct_chat_by_contact)
    mcp.tool(annotations=contact_annotations)(get_contact_chats)
