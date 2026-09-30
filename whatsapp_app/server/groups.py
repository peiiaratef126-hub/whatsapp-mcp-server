"""Group management MCP tools."""

from typing import Any

from client import BridgeClient, BridgeError
from mcp.server.fastmcp import FastMCP
from mcp.types import ToolAnnotations


def register_groups_tools(mcp: FastMCP, client: BridgeClient) -> None:
    """Register group management tools with FastMCP server."""

    @mcp.tool(
        annotations=ToolAnnotations(
            readOnlyHint=False,
            destructiveHint=False,
            idempotentHint=False,
            openWorldHint=True,
        )
    )
    def create_group(name: str, participants: list[str]) -> dict[str, Any]:
        """Create a new WhatsApp group chat with initial participants.

        Args:
            name: Group subject/name (max 25 characters recommended by WhatsApp).
            participants: List of participant phone numbers or JIDs to add to the new group.

        Returns:
            Result with group JID, name, and creation status.
        """
        if not name or not name.strip():
            return {"error": "group name cannot be empty"}
        if not participants:
            return {"error": "at least one participant is required to create a group"}

        cleaned_participants = [p.strip() for p in participants if p and p.strip()]
        if not cleaned_participants:
            return {"error": "participant list contains no valid entries"}

        try:
            return client.create_group(name=name.strip(), participants=cleaned_participants)
        except BridgeError as e:
            if e.status_code == 403 or "permission" in str(e).lower():
                return {
                    "error": f"Permission denied: not authorized to create group ({e.message})",
                    "status_code": 403,
                }
            return {"error": str(e)}

    @mcp.tool(
        annotations=ToolAnnotations(
            readOnlyHint=False,
            destructiveHint=False,
            idempotentHint=True,
            openWorldHint=True,
        )
    )
    def add_participant(group_jid: str, participant: str) -> dict[str, Any]:
        """Add a participant to an existing WhatsApp group.

        Requires admin permissions in the group.

        Args:
            group_jid: Target group JID (e.g. 123456789-987654@g.us).
            participant: Phone number or JID of the user to add.

        Returns:
            Result indicating success and added participant JID.
        """
        if not group_jid or not group_jid.strip():
            return {"error": "group_jid cannot be empty"}
        if not participant or not participant.strip():
            return {"error": "participant cannot be empty"}

        clean_group = group_jid.strip()
        clean_part = participant.strip()

        try:
            return client.add_participant(clean_group, clean_part)
        except BridgeError as e:
            if (
                e.status_code == 403
                or "permission" in str(e).lower()
                or "not an admin" in str(e).lower()
            ):
                return {
                    "error": f"Permission denied: the connected WhatsApp account is not an admin of group {clean_group}",
                    "details": e.message,
                    "status_code": 403,
                }
            return {"error": str(e), "group_jid": clean_group, "participant": clean_part}

    @mcp.tool(
        annotations=ToolAnnotations(
            readOnlyHint=False,
            destructiveHint=True,
            idempotentHint=True,
            openWorldHint=True,
        )
    )
    def remove_participant(group_jid: str, participant: str) -> dict[str, Any]:
        """Remove a participant from an existing WhatsApp group.

        Requires admin permissions in the group.

        Args:
            group_jid: Target group JID (e.g. 123456789-987654@g.us).
            participant: Phone number or JID of the user to remove.

        Returns:
            Result indicating success and removed participant JID.
        """
        if not group_jid or not group_jid.strip():
            return {"error": "group_jid cannot be empty"}
        if not participant or not participant.strip():
            return {"error": "participant cannot be empty"}

        clean_group = group_jid.strip()
        clean_part = participant.strip()

        try:
            return client.remove_participant(clean_group, clean_part)
        except BridgeError as e:
            if (
                e.status_code == 403
                or "permission" in str(e).lower()
                or "not an admin" in str(e).lower()
            ):
                return {
                    "error": f"Permission denied: the connected WhatsApp account is not an admin of group {clean_group}",
                    "details": e.message,
                    "status_code": 403,
                }
            return {"error": str(e), "group_jid": clean_group, "participant": clean_part}

    @mcp.tool(
        annotations=ToolAnnotations(
            readOnlyHint=True,
            destructiveHint=False,
            idempotentHint=True,
            openWorldHint=True,
        )
    )
    def get_group_invite_link(group_jid: str) -> dict[str, Any]:
        """Get the public invite link for a WhatsApp group.

        Requires admin permissions in the group.

        Args:
            group_jid: Target group JID (e.g. 123456789-987654@g.us).

        Returns:
            Dictionary containing the group invite link URL.
        """
        if not group_jid or not group_jid.strip():
            return {"error": "group_jid cannot be empty"}

        clean_group = group_jid.strip()
        try:
            return client.get_group_invite_link(clean_group)
        except BridgeError as e:
            if (
                e.status_code == 403
                or "permission" in str(e).lower()
                or "not an admin" in str(e).lower()
            ):
                return {
                    "error": f"Permission denied: the connected WhatsApp account is not an admin of group {clean_group}",
                    "details": e.message,
                    "status_code": 403,
                }
            return {"error": str(e), "group_jid": clean_group}
