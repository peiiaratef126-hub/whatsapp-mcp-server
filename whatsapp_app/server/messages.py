"""Message and chat management MCP tools."""

from typing import Any

from client import BridgeClient, BridgeError
from confirmation import format_pending_confirmation
from mcp.server.fastmcp import FastMCP
from mcp.types import ToolAnnotations


def register_messages_tools(mcp: FastMCP, client: BridgeClient) -> None:
    """Register message-related tools with FastMCP server."""

    @mcp.tool(
        annotations=ToolAnnotations(
            readOnlyHint=True,
            destructiveHint=False,
            idempotentHint=True,
            openWorldHint=True,
        )
    )
    def list_chats(limit: int = 50) -> list[dict[str, Any]]:
        """List recent WhatsApp conversations (direct chats, groups, and channels).

        Args:
            limit: Maximum number of chats to return (default: 50).

        Returns:
            List of chats with JID, name, unread count, and last message info.
        """
        try:
            return client.list_chats(limit=limit)
        except BridgeError as e:
            return [{"error": str(e)}]

    @mcp.tool(
        annotations=ToolAnnotations(
            readOnlyHint=True,
            destructiveHint=False,
            idempotentHint=True,
            openWorldHint=True,
        )
    )
    def get_chat(chat_jid: str) -> dict[str, Any]:
        """Get details and metadata for a specific WhatsApp chat or group.

        Args:
            chat_jid: The chat JID (e.g. 12345@s.whatsapp.net, 123-456@g.us, or newsletter JID).

        Returns:
            Chat information including name, group/channel status, unread count, and participant list.
        """
        if not chat_jid or not chat_jid.strip():
            return {"error": "chat_jid cannot be empty"}
        try:
            return client.get_chat(chat_jid.strip())
        except BridgeError as e:
            return {"error": str(e), "chat_jid": chat_jid}

    @mcp.tool(
        annotations=ToolAnnotations(
            readOnlyHint=True,
            destructiveHint=False,
            idempotentHint=True,
            openWorldHint=True,
        )
    )
    def list_messages(
        chat_jid: str | None = None,
        query: str | None = None,
        since: int | None = None,
        until: int | None = None,
        limit: int = 50,
        offset: int = 0,
    ) -> list[dict[str, Any]]:
        """List and search WhatsApp message history.

        Args:
            chat_jid: Optional chat JID to filter messages by conversation.
            query: Optional search string to filter messages by text content.
            since: Optional Unix timestamp for the start of the date range.
            until: Optional Unix timestamp for the end of the date range.
            limit: Maximum number of messages to return (default: 50).
            offset: Number of messages to skip for pagination (default: 0).

        Returns:
            List of message objects ordered by timestamp descending.
        """
        try:
            return client.list_messages(
                chat_jid=chat_jid.strip() if chat_jid else None,
                query=query.strip() if query else None,
                since=since,
                until=until,
                limit=limit,
                offset=offset,
            )
        except BridgeError as e:
            return [{"error": str(e)}]

    @mcp.tool(
        annotations=ToolAnnotations(
            readOnlyHint=True,
            destructiveHint=False,
            idempotentHint=True,
            openWorldHint=True,
        )
    )
    def get_last_interaction(contact: str) -> dict[str, Any]:
        """Get the single most recent WhatsApp message exchanged with or involving a contact.

        Args:
            contact: Contact phone number, JID, or name.

        Returns:
            The most recent message object involving the specified contact.
        """
        if not contact or not contact.strip():
            return {"error": "contact identifier cannot be empty"}
        try:
            return client.get_last_interaction(contact.strip())
        except BridgeError as e:
            return {"error": str(e), "contact": contact}

    @mcp.tool(
        annotations=ToolAnnotations(
            readOnlyHint=True,
            destructiveHint=False,
            idempotentHint=True,
            openWorldHint=True,
        )
    )
    def get_message_context(
        message_id: str, before: int = 5, after: int = 5
    ) -> list[dict[str, Any]]:
        """Get surrounding conversation context for a specific WhatsApp message.

        Args:
            message_id: The ID of the target message.
            before: Number of messages to retrieve immediately before the target (default: 5).
            after: Number of messages to retrieve immediately after the target (default: 5).

        Returns:
            Chronologically ordered list of messages surrounding the target message.
        """
        if not message_id or not message_id.strip():
            return [{"error": "message_id cannot be empty"}]
        try:
            return client.get_message_context(message_id.strip(), before=before, after=after)
        except BridgeError as e:
            return [{"error": str(e), "message_id": message_id}
            ]

    @mcp.tool(
        annotations=ToolAnnotations(
            readOnlyHint=False,
            destructiveHint=False,
            idempotentHint=False,
            openWorldHint=True,
        )
    )
    def send_message(recipient_jid: str, text: str) -> dict[str, Any]:
        """Send a plain text message to a WhatsApp user, group, or channel.

        Args:
            recipient_jid: Recipient JID (e.g. phone@s.whatsapp.net, group@g.us, or channel@newsletter).
            text: Message body to send.

        Returns:
            Result with success status, message ID, timestamp, and chat JID.
        """
        if not recipient_jid or not recipient_jid.strip():
            return {"error": "recipient_jid cannot be empty"}
        if not text or not text.strip():
            return {"error": "text cannot be empty"}
        try:
            return client.send_message(recipient_jid.strip(), text.strip())
        except BridgeError as e:
            return {"error": str(e), "recipient_jid": recipient_jid}

    @mcp.tool(
        annotations=ToolAnnotations(
            readOnlyHint=False,
            destructiveHint=False,
            idempotentHint=True,
            openWorldHint=True,
        )
    )
    def send_reaction(message_id: str, emoji: str, chat_jid: str = "") -> dict[str, Any]:
        """Send an emoji reaction to a specific WhatsApp message.

        Args:
            message_id: Target WhatsApp message ID.
            emoji: Emoji character to react with (e.g. '👍', '❤️', or empty string '' to remove reaction).
            chat_jid: The chat JID where the message is located.

        Returns:
            Result with success status, message ID, and emoji.
        """
        if not message_id or not message_id.strip():
            return {"error": "message_id cannot be empty"}
        try:
            return client.send_reaction(message_id.strip(), emoji, chat_jid=chat_jid.strip())
        except BridgeError as e:
            return {"error": str(e), "message_id": message_id}

    @mcp.tool(
        annotations=ToolAnnotations(
            readOnlyHint=False,
            destructiveHint=True,
            idempotentHint=True,
            openWorldHint=True,
        )
    )
    def delete_message(
        message_id: str, chat_jid: str = "", confirm: bool = False
    ) -> dict[str, Any]:
        """Delete/revoke a WhatsApp message for everyone in the chat.

        REQUIRES CONFIRMATION: Set confirm=True to execute deletion.
        When confirm=False or omitted, returns a pending confirmation summary.

        Args:
            message_id: The ID of the message to delete.
            chat_jid: Optional chat JID where the message resides.
            confirm: Set to True to confirm and execute deletion. Defaults to False.

        Returns:
            Confirmation request when confirm=False, or execution result when confirm=True.
        """
        if not message_id or not message_id.strip():
            return {"error": "message_id cannot be empty"}

        clean_msg_id = message_id.strip()
        clean_chat_jid = chat_jid.strip() if chat_jid else ""

        if not confirm:
            chat_desc = f"in chat '{clean_chat_jid}'" if clean_chat_jid else "in its chat"
            return format_pending_confirmation(
                action="delete_message",
                details=f"This will delete message '{clean_msg_id}' for everyone {chat_desc}.",
                tool_name="delete_message",
            )

        try:
            return client.delete_message(clean_msg_id, chat_jid=clean_chat_jid)
        except BridgeError as e:
            return {"error": str(e), "message_id": clean_msg_id}

    @mcp.tool(
        annotations=ToolAnnotations(
            readOnlyHint=False,
            destructiveHint=False,
            idempotentHint=True,
            openWorldHint=True,
        )
    )
    def mark_as_read(message_id: str, chat_jid: str = "") -> dict[str, Any]:
        """Mark a WhatsApp message as read.

        Args:
            message_id: ID of the message to mark as read.
            chat_jid: Optional chat JID of the conversation.

        Returns:
            Result containing success status and message ID.
        """
        if not message_id or not message_id.strip():
            return {"error": "message_id cannot be empty"}
        try:
            return client.mark_as_read(message_id.strip(), chat_jid=chat_jid.strip())
        except BridgeError as e:
            return {"error": str(e), "message_id": message_id}
