"""Group moderation and admin MCP tools."""

from typing import Any

from client import BridgeClient, BridgeError
from confirmation import format_pending_confirmation
from mcp.server.fastmcp import FastMCP
from mcp.types import ToolAnnotations


def register_admin_tools(mcp: FastMCP, client: BridgeClient) -> None:
    """Register group administration and moderation tools with FastMCP server."""

    @mcp.tool(
        annotations=ToolAnnotations(
            readOnlyHint=False,
            destructiveHint=False,
            idempotentHint=True,
            openWorldHint=True,
        )
    )
    def set_group_admins_only(
        group_jid: str, enabled: bool, confirm: bool = False
    ) -> dict[str, Any]:
        """Toggle Announcement Mode in a WhatsApp group (restricting messaging to admins only).

        REQUIRES CONFIRMATION: Set confirm=True to execute this setting change.
        When confirm=False or omitted, returns a pending confirmation summary.

        Applies to groups (@g.us) only. WhatsApp channels are broadcast-only by design.
        Requires the connected account to be an admin of the group.

        Args:
            group_jid: Group JID (e.g. 123456789-987654@g.us).
            enabled: True to allow only admins to send messages; False to allow all members to send messages.
            confirm: Set to True to confirm and execute the action. Defaults to False.

        Returns:
            Pending confirmation notice if confirm=False, or execution result if confirm=True.
        """
        if not group_jid or not group_jid.strip():
            return {"error": "group_jid cannot be empty"}

        clean_group = group_jid.strip()
        if not clean_group.endswith("@g.us"):
            return {
                "error": "Announcement Mode can only be configured for groups (@g.us). WhatsApp Channels are broadcast-only by design."
            }

        if not confirm:
            action_desc = (
                "restrict message sending to group admins only (Announcement Mode)"
                if enabled
                else "allow all group members to send messages (standard mode)"
            )
            return format_pending_confirmation(
                action="set_group_admins_only",
                details=f"This will {action_desc} in group '{clean_group}'.",
                tool_name="set_group_admins_only",
            )

        try:
            return client.set_group_announce_only(clean_group, enabled)
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
