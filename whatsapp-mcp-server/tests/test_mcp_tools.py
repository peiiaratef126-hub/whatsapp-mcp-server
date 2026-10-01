"""Comprehensive tests for all 22 MCP tools using a mocked/fake bridge response."""

import os
import tempfile
from typing import Any

import pytest

from client import BridgeClient, BridgeError
from server import create_mcp_server


class FakeBridgeClient(BridgeClient):
    """Mock bridge client recording calls and returning configurable responses."""

    def __init__(self):
        self.calls: list[dict[str, Any]] = []
        self.should_fail_permission = False
        self.should_fail_timeout = False
        self.should_fail_not_found = False

    def _record(self, _method_name: str, **kwargs) -> None:
        self.calls.append({"method": _method_name, "args": kwargs})
        if self.should_fail_timeout:
            raise BridgeError("Request to WhatsApp bridge timed out after 20.0s")
        if self.should_fail_permission:
            raise BridgeError(
                "Permission denied: the connected account is not an admin", status_code=403
            )
        if self.should_fail_not_found:
            raise BridgeError("Resource not found", status_code=404)

    def get_status(self) -> dict[str, Any]:
        self._record("get_status")
        return {
            "connected": True,
            "logged_in": True,
            "jid": "1234567890@s.whatsapp.net",
            "phone": "1234567890",
            "push_name": "Test User",
            "needs_reauth": False,
        }

    def search_contacts(self, query: str) -> list[dict[str, Any]]:
        self._record("search_contacts", query=query)
        if query == "nobody":
            return []
        return [{"jid": "123@s.whatsapp.net", "name": f"Contact {query}", "phone_number": "123"}]

    def get_direct_chat_by_contact(self, contact: str) -> dict[str, Any]:
        self._record("get_direct_chat_by_contact", contact=contact)
        return {"jid": "123@s.whatsapp.net", "name": "Direct Contact", "is_group": False}

    def get_contact_chats(self, contact: str) -> list[dict[str, Any]]:
        self._record("get_contact_chats", contact=contact)
        return [
            {"jid": "123@s.whatsapp.net", "name": "Direct"},
            {"jid": "grp@g.us", "name": "Group"},
        ]

    def get_last_interaction(self, contact: str) -> dict[str, Any]:
        self._record("get_last_interaction", contact=contact)
        return {"id": "MSG_LAST", "text": "See you later", "timestamp": 1700000000}

    def list_chats(self, limit: int = 50) -> list[dict[str, Any]]:
        self._record("list_chats", limit=limit)
        return [{"jid": "chat1@s.whatsapp.net", "name": "Chat 1"}]

    def get_chat(self, jid: str) -> dict[str, Any]:
        self._record("get_chat", jid=jid)
        return {"jid": jid, "name": "Test Chat", "is_group": jid.endswith("@g.us")}

    def list_messages(
        self,
        chat_jid: str | None = None,
        query: str | None = None,
        since: int | None = None,
        until: int | None = None,
        limit: int = 50,
        offset: int = 0,
    ) -> list[dict[str, Any]]:
        self._record("list_messages", chat_jid=chat_jid, query=query, limit=limit)
        return [{"id": "M1", "text": "Hello world", "timestamp": 1700000000}]

    def get_message_context(
        self, message_id: str, before: int = 5, after: int = 5
    ) -> list[dict[str, Any]]:
        self._record("get_message_context", message_id=message_id, before=before, after=after)
        return [
            {"id": "M0", "text": "Before"},
            {"id": message_id, "text": "Target"},
            {"id": "M2", "text": "After"},
        ]

    def send_message(self, recipient_jid: str, text: str) -> dict[str, Any]:
        self._record("send_message", recipient_jid=recipient_jid, text=text)
        return {"success": True, "message_id": "MSG_SENT_1", "timestamp": 1700000100}

    def send_file(self, recipient_jid: str, file_path: str, caption: str = "") -> dict[str, Any]:
        self._record("send_file", recipient_jid=recipient_jid, file_path=file_path, caption=caption)
        return {
            "success": True,
            "message_id": "MSG_FILE_1",
            "file_name": os.path.basename(file_path),
        }

    def send_audio_message(self, recipient_jid: str, file_path: str) -> dict[str, Any]:
        self._record("send_audio_message", recipient_jid=recipient_jid, file_path=file_path)
        return {"success": True, "message_id": "MSG_AUDIO_1"}

    def send_reaction(self, message_id: str, emoji: str, chat_jid: str = "") -> dict[str, Any]:
        self._record("send_reaction", message_id=message_id, emoji=emoji, chat_jid=chat_jid)
        return {"success": True, "message_id": message_id, "emoji": emoji}

    def delete_message(self, message_id: str, chat_jid: str = "") -> dict[str, Any]:
        self._record("delete_message", message_id=message_id, chat_jid=chat_jid)
        return {"success": True, "message_id": message_id, "revoked": True}

    def mark_as_read(self, message_id: str, chat_jid: str = "") -> dict[str, Any]:
        self._record("mark_as_read", message_id=message_id, chat_jid=chat_jid)
        return {"success": True, "message_id": message_id}

    def download_media(self, message_id: str, chat_jid: str = "") -> dict[str, Any]:
        self._record("download_media", message_id=message_id, chat_jid=chat_jid)
        return {"file_path": f"/tmp/media_{message_id}.jpg", "filename": "image.jpg", "size": 1024}

    def get_group_pdfs(
        self, chat_jid: str, since: int | None = None, until: int | None = None
    ) -> list[dict[str, Any]]:
        self._record("get_group_pdfs", chat_jid=chat_jid, since=since, until=until)
        return []

    def create_group(self, name: str, participants: list[str]) -> dict[str, Any]:
        self._record("create_group", name=name, participants=participants)
        return {"success": True, "group_jid": "newgroup@g.us", "name": name}

    def add_participant(self, group_jid: str, participant: str) -> dict[str, Any]:
        self._record("add_participant", group_jid=group_jid, participant=participant)
        return {"success": True, "group_jid": group_jid, "participant": participant}

    def remove_participant(self, group_jid: str, participant: str) -> dict[str, Any]:
        self._record("remove_participant", group_jid=group_jid, participant=participant)
        return {"success": True, "group_jid": group_jid, "participant": participant}

    def get_group_invite_link(self, group_jid: str) -> dict[str, Any]:
        self._record("get_group_invite_link", group_jid=group_jid)
        return {"group_jid": group_jid, "invite_link": "https://chat.whatsapp.com/INVITE123"}

    def set_group_announce_only(self, group_jid: str, enabled: bool) -> dict[str, Any]:
        self._record("set_group_announce_only", group_jid=group_jid, enabled=enabled)
        return {"success": True, "group_jid": group_jid, "enabled": enabled}

    def create_channel(self, name: str, description: str = "") -> dict[str, Any]:
        self._record("create_channel", name=name, description=description)
        return {
            "success": True,
            "channel_jid": "123456789@newsletter",
            "name": name,
            "description": description,
        }


@pytest.fixture
def test_setup():
    fake_client = FakeBridgeClient()
    mcp = create_mcp_server(client=fake_client)
    tools = {t.name: t.fn for t in mcp._tool_manager.list_tools()}
    return fake_client, tools


# 1. Contacts Tools Tests
def test_search_contacts(test_setup):
    _, tools = test_setup
    res = tools["search_contacts"]("Alice")
    assert len(res) == 1
    assert res[0]["name"] == "Contact Alice"

    empty_res = tools["search_contacts"]("")
    assert empty_res == []


def test_get_direct_chat_by_contact(test_setup):
    _, tools = test_setup
    res = tools["get_direct_chat_by_contact"]("+1234567890")
    assert res["jid"] == "123@s.whatsapp.net"

    err_res = tools["get_direct_chat_by_contact"]("")
    assert "error" in err_res


def test_get_contact_chats(test_setup):
    _, tools = test_setup
    res = tools["get_contact_chats"]("+1234567890")
    assert len(res) == 2


# 2. Messages Tools Tests
def test_list_messages(test_setup):
    _, tools = test_setup
    res = tools["list_messages"](chat_jid="123@s.whatsapp.net", query="Hello")
    assert len(res) == 1
    assert res[0]["id"] == "M1"


def test_list_chats(test_setup):
    _, tools = test_setup
    res = tools["list_chats"](limit=10)
    assert len(res) == 1


def test_get_chat(test_setup):
    _, tools = test_setup
    res = tools["get_chat"]("mygroup@g.us")
    assert res["name"] == "Test Chat"
    assert res["is_group"] is True


def test_get_last_interaction(test_setup):
    _, tools = test_setup
    res = tools["get_last_interaction"]("1234567890")
    assert res["id"] == "MSG_LAST"


def test_get_message_context(test_setup):
    _, tools = test_setup
    res = tools["get_message_context"]("TARGET_MSG", before=2, after=2)
    assert len(res) == 3
    assert res[1]["id"] == "TARGET_MSG"


def test_send_message(test_setup):
    _, tools = test_setup
    res = tools["send_message"]("123@s.whatsapp.net", "Hi there!")
    assert res["success"] is True
    assert res["message_id"] == "MSG_SENT_1"

    err_res = tools["send_message"]("", "Hello")
    assert "error" in err_res


def test_send_reaction(test_setup):
    _, tools = test_setup
    res = tools["send_reaction"]("MSG_1", "👍", chat_jid="123@s.whatsapp.net")
    assert res["success"] is True
    assert res["emoji"] == "👍"


def test_delete_message_confirmation_gating(test_setup):
    fake_client, tools = test_setup

    fake_client.calls.clear()
    pending = tools["delete_message"]("MSG_DELETE_ME", chat_jid="123@s.whatsapp.net", confirm=False)
    assert pending["status"] == "pending_confirmation"
    assert "MSG_DELETE_ME" in pending["details"]
    assert len(fake_client.calls) == 0

    executed = tools["delete_message"]("MSG_DELETE_ME", chat_jid="123@s.whatsapp.net", confirm=True)
    assert executed["success"] is True
    assert executed["revoked"] is True
    assert len(fake_client.calls) == 1
    assert fake_client.calls[0]["method"] == "delete_message"


def test_mark_as_read(test_setup):
    _, tools = test_setup
    res = tools["mark_as_read"]("MSG_READ_ME")
    assert res["success"] is True


# 3. Media Tools Tests
def test_send_file_validation(test_setup):
    _, tools = test_setup
    err_res = tools["send_file"]("123@s.whatsapp.net", "/nonexistent/fake_file.pdf")
    assert "error" in err_res
    assert "does not exist" in err_res["error"]

    with tempfile.NamedTemporaryFile(delete=False) as tmp:
        tmp.write(b"Hello world file data")
        tmp_name = tmp.name
    try:
        ok_res = tools["send_file"]("123@s.whatsapp.net", tmp_name)
        assert ok_res["success"] is True
    finally:
        os.unlink(tmp_name)


def test_send_audio_message_validation(test_setup):
    _, tools = test_setup
    err_res = tools["send_audio_message"]("123@s.whatsapp.net", "/nonexistent/audio.mp3")
    assert "error" in err_res

    with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as tmp:
        tmp.write(b"WAVE_AUDIO_BYTES_TEST")
        tmp_name = tmp.name
    try:
        ok_res = tools["send_audio_message"]("123@s.whatsapp.net", tmp_name)
        assert ok_res["success"] is True
    finally:
        os.unlink(tmp_name)


def test_download_media(test_setup):
    _, tools = test_setup
    res = tools["download_media"]("MSG_MEDIA_1", chat_jid="123@s.whatsapp.net")
    assert "file_path" in res
    assert res["size"] == 1024


def test_get_group_pdfs(test_setup):
    _, tools = test_setup
    res = tools["get_group_pdfs"]("123456@g.us")
    assert "chat_jid" in res
    assert res["total_files"] == 0


# 4. Group Tools Tests
def test_create_group(test_setup):
    _, tools = test_setup
    res = tools["create_group"]("Team Group", ["+1234567890", "+1987654321"])
    assert res["success"] is True
    assert res["group_jid"] == "newgroup@g.us"

    assert "error" in tools["create_group"]("", ["+123"])
    assert "error" in tools["create_group"]("Group", [])


def test_add_participant_permission_denied(test_setup):
    fake_client, tools = test_setup
    res = tools["add_participant"]("group@g.us", "+1234567890")
    assert res["success"] is True

    fake_client.should_fail_permission = True
    err_res = tools["add_participant"]("group@g.us", "+1234567890")
    assert "Permission denied" in err_res["error"]
    assert "not an admin" in err_res["error"]


def test_remove_participant_permission_denied(test_setup):
    fake_client, tools = test_setup
    fake_client.should_fail_permission = True
    err_res = tools["remove_participant"]("group@g.us", "+1234567890")
    assert "Permission denied" in err_res["error"]
    assert "not an admin" in err_res["error"]


def test_get_group_invite_link_permission_denied(test_setup):
    fake_client, tools = test_setup
    res = tools["get_group_invite_link"]("group@g.us")
    assert "invite_link" in res

    fake_client.should_fail_permission = True
    err_res = tools["get_group_invite_link"]("group@g.us")
    assert "Permission denied" in err_res["error"]
    assert "not an admin" in err_res["error"]


# 5. Admin Tools Tests (set_group_admins_only)
def test_set_group_admins_only_confirmation_and_permission(test_setup):
    fake_client, tools = test_setup

    non_group = tools["set_group_admins_only"]("123456@newsletter", enabled=True, confirm=True)
    assert "Announcement Mode can only be configured for groups" in non_group["error"]

    fake_client.calls.clear()
    pending = tools["set_group_admins_only"]("mygroup@g.us", enabled=True, confirm=False)
    assert pending["status"] == "pending_confirmation"
    assert "restrict message sending to group admins only" in pending["details"]
    assert len(fake_client.calls) == 0

    executed = tools["set_group_admins_only"]("mygroup@g.us", enabled=True, confirm=True)
    assert executed["success"] is True
    assert executed["enabled"] is True
    assert len(fake_client.calls) == 1

    fake_client.should_fail_permission = True
    err_res = tools["set_group_admins_only"]("mygroup@g.us", enabled=True, confirm=True)
    assert "Permission denied" in err_res["error"]
    assert "not an admin" in err_res["error"]


# 6. Channel Tools Tests
def test_create_channel(test_setup):
    _, tools = test_setup
    res = tools["create_channel"]("Tech News", description="Latest updates")
    assert res["success"] is True
    assert res["channel_jid"].endswith("@newsletter")

    err_res = tools["create_channel"]("")
    assert "error" in err_res


# 7. Edge Cases: Timeout / Network Failure & Not Found
def test_network_timeout_edge_case(test_setup):
    fake_client, tools = test_setup
    fake_client.should_fail_timeout = True
    res = tools["list_chats"]()
    assert len(res) == 1
    assert "error" in res[0]
    assert "timed out" in res[0]["error"]


def test_contact_not_found_edge_case(test_setup):
    fake_client, tools = test_setup
    fake_client.should_fail_not_found = True
    res = tools["get_direct_chat_by_contact"]("+999999999")
    assert "error" in res
    assert "not found" in res["error"].lower()


def test_all_22_tool_annotations_defined():
    """Verify that all 22 tools have all four hints explicitly set with boolean values."""
    from server import create_mcp_server

    server = create_mcp_server()
    server_tools = {t.name: t for t in server._tool_manager.list_tools()}
    assert len(server_tools) == 22

    for name, tool in server_tools.items():
        assert hasattr(tool, "annotations"), f"Tool '{name}' is missing annotations attribute"
        ann = tool.annotations
        assert ann is not None, f"Tool '{name}' has None annotations"
        assert isinstance(ann.readOnlyHint, bool), f"Tool '{name}' readOnlyHint is not a bool"
        assert isinstance(ann.destructiveHint, bool), f"Tool '{name}' destructiveHint is not a bool"
        assert isinstance(ann.idempotentHint, bool), f"Tool '{name}' idempotentHint is not a bool"
        assert isinstance(ann.openWorldHint, bool), f"Tool '{name}' openWorldHint is not a bool"

    # Verify contacts tool hints match M8ven requirements
    for contact_tool in ["search_contacts", "get_direct_chat_by_contact", "get_contact_chats"]:
        ann = server_tools[contact_tool].annotations
        assert ann.readOnlyHint is True
        assert ann.destructiveHint is False
        assert ann.idempotentHint is True
        assert ann.openWorldHint is False

