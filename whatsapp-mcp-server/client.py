"""HTTP client for communicating with the local Go WhatsApp bridge."""

from typing import Any, Optional
import httpx
from config import BRIDGE_URL, REQUEST_TIMEOUT, MAX_RETRIES


class BridgeError(Exception):
    """Exception raised when a bridge request fails."""

    def __init__(self, message: str, status_code: Optional[int] = None):
        super().__init__(message)
        self.message = message
        self.status_code = status_code

    def __str__(self) -> str:
        if self.status_code:
            return f"[{self.status_code}] {self.message}"
        return self.message


class BridgeClient:
    """Client for invoking local WhatsApp bridge HTTP endpoints."""

    def __init__(
        self,
        base_url: str = BRIDGE_URL,
        timeout: float = REQUEST_TIMEOUT,
        max_retries: int = MAX_RETRIES,
    ):
        self.base_url = base_url.rstrip("/")
        self.timeout = timeout
        self.max_retries = max_retries
        self._client = httpx.Client(base_url=self.base_url, timeout=self.timeout)

    def close(self) -> None:
        """Close the underlying HTTP client session."""
        self._client.close()

    def _request(
        self,
        method: str,
        endpoint: str,
        params: Optional[dict[str, Any]] = None,
        json_data: Optional[dict[str, Any]] = None,
    ) -> Any:
        """Execute HTTP request with retries and structured error extraction."""
        last_exception = None
        for attempt in range(self.max_retries):
            try:
                resp = self._client.request(
                    method=method,
                    url=endpoint,
                    params=params,
                    json=json_data,
                )
                if resp.status_code >= 400:
                    try:
                        data = resp.json()
                        err_msg = data.get("error", resp.text)
                    except Exception:
                        err_msg = resp.text or f"HTTP {resp.status_code}"
                    raise BridgeError(err_msg, status_code=resp.status_code)

                return resp.json()
            except httpx.ConnectError as e:
                last_exception = BridgeError(
                    f"Could not connect to WhatsApp bridge at {self.base_url}. Is the Go bridge running? ({e})"
                )
            except httpx.TimeoutException as e:
                last_exception = BridgeError(f"Request to WhatsApp bridge timed out: {e}")
            except BridgeError:
                raise
            except Exception as e:
                last_exception = BridgeError(f"Bridge communication error: {e}")

        if last_exception:
            raise last_exception
        raise BridgeError("Request failed after retries")

    # Status
    def get_status(self) -> dict[str, Any]:
        """Fetch bridge status."""
        return self._request("GET", "/status")

    # Contacts
    def search_contacts(self, query: str) -> list[dict[str, Any]]:
        """Search contacts by name, push name, phone or JID."""
        return self._request("GET", "/contacts", params={"query": query})

    def get_direct_chat_by_contact(self, contact: str) -> dict[str, Any]:
        """Get direct chat record for a contact."""
        return self._request("GET", "/contacts/direct", params={"contact": contact})

    def get_contact_chats(self, contact: str) -> list[dict[str, Any]]:
        """Get all chats involving a contact."""
        return self._request("GET", "/contacts/chats", params={"contact": contact})

    def get_last_interaction(self, contact: str) -> dict[str, Any]:
        """Get most recent message involving a contact."""
        return self._request("GET", "/contacts/last_interaction", params={"contact": contact})

    # Chats
    def list_chats(self, limit: int = 50) -> list[dict[str, Any]]:
        """List recent chats."""
        return self._request("GET", "/chats", params={"limit": limit})

    def get_chat(self, jid: str) -> dict[str, Any]:
        """Get chat details with participants."""
        return self._request("GET", "/chats/detail", params={"jid": jid})

    # Messages
    def list_messages(
        self,
        chat_jid: Optional[str] = None,
        query: Optional[str] = None,
        since: Optional[int] = None,
        until: Optional[int] = None,
        limit: int = 50,
        offset: int = 0,
    ) -> list[dict[str, Any]]:
        """List messages with filters."""
        params: dict[str, Any] = {"limit": limit, "offset": offset}
        if chat_jid:
            params["chat_jid"] = chat_jid
        if query:
            params["query"] = query
        if since is not None:
            params["since"] = since
        if until is not None:
            params["until"] = until
        return self._request("GET", "/messages", params=params)

    def get_message_context(
        self, message_id: str, before: int = 5, after: int = 5
    ) -> list[dict[str, Any]]:
        """Get surrounding message context."""
        return self._request(
            "GET",
            "/messages/context",
            params={"message_id": message_id, "before": before, "after": after},
        )

    def send_message(self, recipient_jid: str, text: str) -> dict[str, Any]:
        """Send a plain text message."""
        return self._request(
            "POST",
            "/send/message",
            json_data={"recipient_jid": recipient_jid, "text": text},
        )

    def send_file(
        self, recipient_jid: str, file_path: str, caption: str = ""
    ) -> dict[str, Any]:
        """Send a file/document/image message."""
        return self._request(
            "POST",
            "/send/file",
            json_data={
                "recipient_jid": recipient_jid,
                "file_path": file_path,
                "caption": caption,
            },
        )

    def send_audio_message(self, recipient_jid: str, file_path: str) -> dict[str, Any]:
        """Send an audio message."""
        return self._request(
            "POST",
            "/send/audio",
            json_data={"recipient_jid": recipient_jid, "file_path": file_path},
        )

    def send_reaction(self, message_id: str, emoji: str, chat_jid: str) -> dict[str, Any]:
        """Send an emoji reaction to a message."""
        return self._request(
            "POST",
            "/messages/reaction",
            json_data={"message_id": message_id, "chat_jid": chat_jid, "emoji": emoji},
        )

    def delete_message(self, message_id: str, chat_jid: str = "") -> dict[str, Any]:
        """Revoke a message for everyone."""
        return self._request(
            "POST",
            "/messages/delete",
            json_data={"message_id": message_id, "chat_jid": chat_jid},
        )

    def mark_as_read(self, message_id: str, chat_jid: str = "") -> dict[str, Any]:
        """Mark a message as read."""
        return self._request(
            "POST",
            "/messages/mark_read",
            json_data={"message_id": message_id, "chat_jid": chat_jid},
        )

    # Media
    def download_media(self, message_id: str, chat_jid: str = "") -> dict[str, Any]:
        """Download media attachment and return local path."""
        return self._request(
            "GET",
            "/media/download",
            params={"message_id": message_id, "chat_jid": chat_jid},
        )

    def get_group_pdfs(
        self,
        chat_jid: str,
        since: Optional[int] = None,
        until: Optional[int] = None,
    ) -> list[dict[str, Any]]:
        """Get list of PDF messages in a group."""
        params: dict[str, Any] = {"chat_jid": chat_jid}
        if since is not None:
            params["since"] = since
        if until is not None:
            params["until"] = until
        return self._request("GET", "/groups/pdfs", params=params)

    # Groups
    def create_group(self, name: str, participants: list[str]) -> dict[str, Any]:
        """Create a new group."""
        return self._request(
            "POST",
            "/groups/create",
            json_data={"name": name, "participants": participants},
        )

    def add_participant(self, group_jid: str, participant: str) -> dict[str, Any]:
        """Add participant to group."""
        return self._request(
            "POST",
            "/groups/participants/add",
            json_data={"group_jid": group_jid, "participant": participant},
        )

    def remove_participant(self, group_jid: str, participant: str) -> dict[str, Any]:
        """Remove participant from group."""
        return self._request(
            "POST",
            "/groups/participants/remove",
            json_data={"group_jid": group_jid, "participant": participant},
        )

    def get_group_invite_link(self, group_jid: str) -> dict[str, Any]:
        """Get group invite link."""
        return self._request("GET", "/groups/invite_link", params={"group_jid": group_jid})

    def set_group_announce_only(self, group_jid: str, enabled: bool) -> dict[str, Any]:
        """Set group announce-only (admins only) mode."""
        return self._request(
            "POST",
            "/groups/admin/announce_only",
            json_data={"group_jid": group_jid, "enabled": enabled},
        )

    # Channels
    def create_channel(self, name: str, description: str = "") -> dict[str, Any]:
        """Create a WhatsApp channel (@newsletter)."""
        return self._request(
            "POST",
            "/channels/create",
            json_data={"name": name, "description": description},
        )
