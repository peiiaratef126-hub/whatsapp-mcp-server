"""Contact-related MCP tools."""

from typing import Any
from mcp.server.fastmcp import FastMCP
from client import BridgeClient, BridgeError


def register_contacts_tools(mcp: FastMCP, client: BridgeClient) -> None:
    """Register contacts tools with FastMCP server."""

    @mcp.tool()
    def search_contacts(query: str) -> list[dict[str, Any]]:
        """Search WhatsApp contacts by name, push name, phone number, or JID.

        Args:
            query: Name, partial name, phone number, or JID to search for.

        Returns:
            A list of matching contact records containing JID, phone number, name, and push name.
        """
        if not query or not query.strip():
            return []
        try:
            return client.search_contacts(query.strip())
        except BridgeError as e:
            return [{"error": str(e)}]

    @mcp.tool()
    def get_direct_chat_by_contact(contact: str) -> dict[str, Any]:
        """Retrieve the direct (1-on-1) chat metadata for a specific contact.

        Args:
            contact: Contact phone number (e.g. +1234567890), JID (e.g. 1234@s.whatsapp.net), or name.

        Returns:
            Direct chat details including chat JID, name, and last interaction timestamp.
        """
        if not contact or not contact.strip():
            return {"error": "contact identifier cannot be empty"}
        try:
            return client.get_direct_chat_by_contact(contact.strip())
        except BridgeError as e:
            return {"error": str(e), "contact": contact}

    @mcp.tool()
    def get_contact_chats(contact: str) -> list[dict[str, Any]]:
        """Retrieve all chats (direct 1-on-1 chats and group chats) involving a specific contact.

        Args:
            contact: Contact phone number, JID, or name identifier.

        Returns:
            A list of chat records where the contact is a participant.
        """
        if not contact or not contact.strip():
            return [{"error": "contact identifier cannot be empty"}]
        try:
            return client.get_contact_chats(contact.strip())
        except BridgeError as e:
            return [{"error": str(e), "contact": contact}]
