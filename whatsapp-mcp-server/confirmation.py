"""Confirmation mechanism for destructive actions."""

from typing import Any


def format_pending_confirmation(
    action: str,
    details: str,
    tool_name: str,
) -> dict[str, Any]:
    """Return a structured pending confirmation response when confirm is False."""
    return {
        "status": "pending_confirmation",
        "action": action,
        "details": details,
        "instructions": f"Call {tool_name} again with confirm=True to execute this action.",
    }
