"""Comprehensive coverage tests referencing and testing all 22 MCP tools by name.

This file provides 100% explicit reference coverage for:
1. Admin: set_group_admins_only
2. Channels: create_channel
3. Contacts: search_contacts, get_direct_chat_by_contact, get_contact_chats
4. Groups: create_group, add_participant, remove_participant, get_group_invite_link
5. Media: send_file, send_audio_message, download_media, get_group_pdfs
6. Messages: list_chats, get_chat, list_messages, get_last_interaction, get_message_context,
            send_message, send_reaction, delete_message, mark_as_read
"""

import os
import tempfile
from unittest.mock import MagicMock

import pytest

from admin import set_client as set_admin_client
from admin import set_group_admins_only
from channels import create_channel
from channels import set_client as set_channels_client
from client import BridgeClient
from contacts import (
    get_contact_chats,
    get_direct_chat_by_contact,
    search_contacts,
)
from contacts import (
    set_client as set_contacts_client,
)
from groups import (
    add_participant,
    create_group,
    get_group_invite_link,
    remove_participant,
)
from groups import (
    set_client as set_groups_client,
)
from media import (
    download_media,
    get_group_pdfs,
    send_audio_message,
    send_file,
)
from media import (
    set_client as set_media_client,
)
from messages import (
    delete_message,
    get_chat,
    get_last_interaction,
    get_message_context,
    list_chats,
    list_messages,
    mark_as_read,
    send_message,
    send_reaction,
)
from messages import (
    set_client as set_messages_client,
)


@pytest.fixture(autouse=True)
def setup_mock_clients():
    """Configure a unified mock BridgeClient for all tool modules."""
    mock_client = MagicMock(spec=BridgeClient)

    # Admin mock
    mock_client.set_group_announce_only.return_value = {
        "success": True,
        "group_jid": "123456-789@g.us",
        "enabled": True,
    }

    # Channels mock
    mock_client.create_channel.return_value = {
        "success": True,
        "channel_jid": "123456789@newsletter",
        "name": "Tech Channel",
        "description": "Tech news",
    }

    # Contacts mock
    mock_client.search_contacts.return_value = [
        {"jid": "123@s.whatsapp.net", "name": "Alice", "phone_number": "123"}
    ]
    mock_client.get_direct_chat_by_contact.return_value = {
        "jid": "123@s.whatsapp.net",
        "name": "Alice",
        "is_group": False,
    }
    mock_client.get_contact_chats.return_value = [
        {"jid": "123@s.whatsapp.net", "name": "Alice Direct"},
        {"jid": "123456-789@g.us", "name": "Project Group"},
    ]

    # Groups mock
    mock_client.create_group.return_value = {
        "success": True,
        "group_jid": "newgroup@g.us",
        "name": "New Team",
    }
    mock_client.add_participant.return_value = {
        "success": True,
        "group_jid": "group@g.us",
        "participant": "123@s.whatsapp.net",
    }
    mock_client.remove_participant.return_value = {
        "success": True,
        "group_jid": "group@g.us",
        "participant": "123@s.whatsapp.net",
    }
    mock_client.get_group_invite_link.return_value = {
        "group_jid": "group@g.us",
        "invite_link": "https://chat.whatsapp.com/INVITE123",
    }

    # Media mock
    mock_client.send_file.return_value = {
        "success": True,
        "message_id": "MSG_FILE_01",
        "file_name": "document.pdf",
    }
    mock_client.send_audio_message.return_value = {
        "success": True,
        "message_id": "MSG_AUDIO_01",
    }
    mock_client.download_media.return_value = {
        "file_path": "/tmp/media_1.jpg",
        "filename": "media_1.jpg",
        "size": 2048,
    }
    mock_client.get_group_pdfs.return_value = []

    # Messages mock
    mock_client.list_chats.return_value = [
        {"jid": "chat1@s.whatsapp.net", "name": "Chat 1", "unread_count": 0}
    ]
    mock_client.get_chat.return_value = {
        "jid": "chat1@s.whatsapp.net",
        "name": "Chat 1",
        "is_group": False,
    }
    mock_client.list_messages.return_value = [
        {"id": "MSG_01", "text": "Hello world", "timestamp": 1700000000}
    ]
    mock_client.get_last_interaction.return_value = {
        "id": "MSG_LAST",
        "text": "Latest message",
        "timestamp": 1700000100,
    }
    mock_client.get_message_context.return_value = [
        {"id": "MSG_PREV", "text": "Before"},
        {"id": "MSG_TARGET", "text": "Target"},
        {"id": "MSG_NEXT", "text": "After"},
    ]
    mock_client.send_message.return_value = {
        "success": True,
        "message_id": "MSG_SENT_01",
        "timestamp": 1700000200,
    }
    mock_client.send_reaction.return_value = {
        "success": True,
        "message_id": "MSG_01",
        "emoji": "👍",
    }
    mock_client.delete_message.return_value = {
        "success": True,
        "message_id": "MSG_01",
        "revoked": True,
    }
    mock_client.mark_as_read.return_value = {
        "success": True,
        "message_id": "MSG_01",
    }

    # Inject mock client into all modules
    set_admin_client(mock_client)
    set_channels_client(mock_client)
    set_contacts_client(mock_client)
    set_groups_client(mock_client)
    set_media_client(mock_client)
    set_messages_client(mock_client)

    return mock_client


# ==============================================================================
# 1. Admin Tools Tests (1 tool)
# ==============================================================================
def test_set_group_admins_only():
    """Test set_group_admins_only parameter validation and confirmation gating."""
    # Validation error for empty JID
    assert "error" in set_group_admins_only("", enabled=True)

    # Validation error for non-group JID
    non_group = set_group_admins_only("12345@s.whatsapp.net", enabled=True, confirm=True)
    assert "error" in non_group
    assert "Announcement Mode can only be configured for groups" in non_group["error"]

    # Confirmation gating: confirm=False must return pending_confirmation
    pending = set_group_admins_only("123456-789@g.us", enabled=True, confirm=False)
    assert pending["status"] == "pending_confirmation"
    assert "set_group_admins_only" in pending["instructions"]

    # Execution with confirm=True
    executed = set_group_admins_only("123456-789@g.us", enabled=True, confirm=True)
    assert executed["success"] is True
    assert executed["enabled"] is True


# ==============================================================================
# 2. Channels Tools Tests (1 tool)
# ==============================================================================
def test_create_channel():
    """Test create_channel parameter validation and response."""
    # Validation error for empty channel name
    assert "error" in create_channel("")

    # Success creation
    res = create_channel("Tech News", description="Latest announcements")
    assert res["success"] is True
    assert res["channel_jid"].endswith("@newsletter")
    assert res["name"] == "Tech Channel"


# ==============================================================================
# 3. Contacts Tools Tests (3 tools)
# ==============================================================================
def test_search_contacts():
    """Test search_contacts parameter validation and search result format."""
    # Empty query returns empty list
    assert search_contacts("") == []

    # Valid search returns contact records
    contacts = search_contacts("Alice")
    assert len(contacts) == 1
    assert contacts[0]["name"] == "Alice"


def test_get_direct_chat_by_contact():
    """Test get_direct_chat_by_contact parameter validation and result."""
    # Empty contact returns error
    assert "error" in get_direct_chat_by_contact("")

    # Valid contact returns chat metadata
    chat = get_direct_chat_by_contact("+1234567890")
    assert chat["jid"] == "123@s.whatsapp.net"
    assert chat["is_group"] is False


def test_get_contact_chats():
    """Test get_contact_chats parameter validation and chat list."""
    # Empty contact returns error
    assert "error" in get_contact_chats("")[0]

    # Valid contact returns list of chats
    chats = get_contact_chats("+1234567890")
    assert len(chats) == 2
    assert chats[0]["name"] == "Alice Direct"


# ==============================================================================
# 4. Groups Tools Tests (4 tools)
# ==============================================================================
def test_create_group():
    """Test create_group parameter validation and creation result."""
    assert "error" in create_group("", ["+1234567890"])
    assert "error" in create_group("My Team", [])

    res = create_group("New Team", ["+1234567890", "+1987654321"])
    assert res["success"] is True
    assert res["group_jid"] == "newgroup@g.us"


def test_add_participant():
    """Test add_participant parameter validation and success."""
    assert "error" in add_participant("", "+1234567890")
    assert "error" in add_participant("group@g.us", "")

    res = add_participant("group@g.us", "+1234567890")
    assert res["success"] is True
    assert res["participant"] == "123@s.whatsapp.net"


def test_remove_participant():
    """Test remove_participant parameter validation and success."""
    assert "error" in remove_participant("", "+1234567890")
    assert "error" in remove_participant("group@g.us", "")

    res = remove_participant("group@g.us", "+1234567890")
    assert res["success"] is True
    assert res["group_jid"] == "group@g.us"


def test_get_group_invite_link():
    """Test get_group_invite_link parameter validation and invite URL."""
    assert "error" in get_group_invite_link("")

    res = get_group_invite_link("group@g.us")
    assert "invite_link" in res
    assert "https://chat.whatsapp.com/" in res["invite_link"]


# ==============================================================================
# 5. Media Tools Tests (4 tools)
# ==============================================================================
def test_send_file():
    """Test send_file parameter validation and delivery."""
    assert "error" in send_file("", "test.pdf")
    assert "error" in send_file("123@s.whatsapp.net", "/path/to/nonexistent_file.pdf")

    with tempfile.NamedTemporaryFile(suffix=".txt", delete=False) as tmp:
        tmp.write(b"Test file content")
        tmp_name = tmp.name

    try:
        res = send_file("123@s.whatsapp.net", tmp_name, caption="Hello document")
        assert res["success"] is True
        assert res["message_id"] == "MSG_FILE_01"
    finally:
        os.unlink(tmp_name)


def test_send_audio_message():
    """Test send_audio_message parameter validation and delivery."""
    assert "error" in send_audio_message("", "audio.mp3")
    assert "error" in send_audio_message("123@s.whatsapp.net", "/nonexistent/audio.mp3")

    with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as tmp:
        tmp.write(b"WAVE_HEADER_DUMMY_DATA")
        tmp_name = tmp.name

    try:
        from unittest.mock import patch
        with patch("media.convert_audio_to_ogg_opus", return_value=(tmp_name, True, None)):
            res = send_audio_message("123@s.whatsapp.net", tmp_name)
            assert res["success"] is True
            assert res["message_id"] == "MSG_AUDIO_01"

        with patch("media.convert_audio_to_ogg_opus", return_value=(tmp_name, False, "ffmpeg not found")):
            res_fallback = send_audio_message("123@s.whatsapp.net", tmp_name)
            assert res_fallback["success"] is True
            assert "warning" in res_fallback
    finally:
        os.unlink(tmp_name)


def test_download_media():
    """Test download_media parameter validation and file path result."""
    assert "error" in download_media("")

    res = download_media("MSG_MEDIA_01", chat_jid="123@s.whatsapp.net")
    assert "file_path" in res
    assert res["size"] == 2048


def test_get_group_pdfs():
    """Test get_group_pdfs parameter validation and return structure."""
    assert "error" in get_group_pdfs("")

    res = get_group_pdfs("123456-789@g.us")
    assert "chat_jid" in res
    assert res["total_files"] == 0
    assert "files" in res


# ==============================================================================
# 6. Messages Tools Tests (9 tools)
# ==============================================================================
def test_list_chats():
    """Test list_chats with limit argument."""
    chats = list_chats(limit=10)
    assert len(chats) == 1
    assert chats[0]["name"] == "Chat 1"


def test_get_chat():
    """Test get_chat parameter validation and chat metadata."""
    assert "error" in get_chat("")

    chat = get_chat("chat1@s.whatsapp.net")
    assert chat["name"] == "Chat 1"
    assert chat["is_group"] is False


def test_list_messages():
    """Test list_messages with filtering arguments."""
    msgs = list_messages(chat_jid="chat1@s.whatsapp.net", query="Hello", limit=5)
    assert len(msgs) == 1
    assert msgs[0]["id"] == "MSG_01"


def test_get_last_interaction():
    """Test get_last_interaction parameter validation and return."""
    assert "error" in get_last_interaction("")

    msg = get_last_interaction("+1234567890")
    assert msg["id"] == "MSG_LAST"
    assert msg["text"] == "Latest message"


def test_get_message_context():
    """Test get_message_context parameter validation and window range."""
    assert "error" in get_message_context("")[0]

    ctx = get_message_context("MSG_TARGET", before=1, after=1)
    assert len(ctx) == 3
    assert ctx[1]["id"] == "MSG_TARGET"


def test_send_message():
    """Test send_message parameter validation and send response."""
    assert "error" in send_message("", "Hello")
    assert "error" in send_message("123@s.whatsapp.net", "")

    res = send_message("123@s.whatsapp.net", "Hello WhatsApp!")
    assert res["success"] is True
    assert res["message_id"] == "MSG_SENT_01"


def test_send_reaction():
    """Test send_reaction parameter validation and emoji payload."""
    assert "error" in send_reaction("", "❤️")

    res = send_reaction("MSG_01", "👍", chat_jid="123@s.whatsapp.net")
    assert res["success"] is True
    assert res["emoji"] == "👍"


def test_delete_message():
    """Test delete_message parameter validation and confirmation gating."""
    assert "error" in delete_message("")

    # confirm=False must return pending_confirmation
    pending = delete_message("MSG_01", chat_jid="123@s.whatsapp.net", confirm=False)
    assert pending["status"] == "pending_confirmation"
    assert "delete_message" in pending["instructions"]

    # confirm=True executes deletion
    executed = delete_message("MSG_01", chat_jid="123@s.whatsapp.net", confirm=True)
    assert executed["success"] is True
    assert executed["revoked"] is True


def test_mark_as_read():
    """Test mark_as_read parameter validation and success."""
    assert "error" in mark_as_read("")

    res = mark_as_read("MSG_01", chat_jid="123@s.whatsapp.net")
    assert res["success"] is True
    assert res["message_id"] == "MSG_01"
