# WhatsApp MCP Server

[![M8ven Score](https://m8ven.ai/badge/mcp/peiiaratef126-hub-whatsapp-mcp-server-c6vpu65)](https://m8ven.ai/mcp/peiiaratef126-hub-whatsapp-mcp-server-c6vpu65)

A Model Context Protocol (MCP) server that connects to a personal WhatsApp account through the unofficial WhatsApp Web multi-device protocol using [`whatsmeow`](https://github.com/tulir/whatsmeow) and the official [Model Context Protocol Python SDK](https://github.com/modelcontextprotocol/python-sdk).

This system exposes tools for an LLM client (such as Claude Desktop) to read, search, and send messages, manage groups and channels, download media, and extract text from PDF documents shared in WhatsApp group chats.

---

## Architecture Overview

The system employs a two-process architecture communicating locally:

```
┌─────────────────────────────────┐
│     LLM Client (Claude Desktop) │
└────────────────┬────────────────┘
                 │ stdio (JSON-RPC)
┌────────────────▼────────────────────────────────┐
│   whatsapp-mcp-server (Python FastMCP)          │
│   - Enforces confirmation for destructive tools │
│   - Extracts text from PDFs (pypdf)             │
│   - Audio format conversion via ffmpeg          │
└────────────────┬────────────────────────────────┘
                 │ HTTP (localhost 127.0.0.1:8080 only)
┌────────────────▼────────────────────────────────┐
│   whatsapp-bridge (Go + whatsmeow)              │
│   - Multi-device WhatsApp connection            │
│   - Terminal QR code pairing                    │
│   - SQLite session persistence & history cache  │
│   - Real-time message & group ingestion         │
└─────────────────────────────────────────────────┘
```

1. **`whatsapp-bridge/` (Go)**:
   - Connects to WhatsApp via QR code pairing using `go.mau.fi/whatsmeow`.
   - Persists session credentials and stores message history, chats, contacts, and group participants in local SQLite databases (`whatsapp_session.db` and `whatsapp_data.db`).
   - Exposes an HTTP/RPC interface strictly bound to `127.0.0.1` (never exposed externally).
   - Handles WhatsApp session invalidation gracefully by signaling unauthenticated state and prompting for re-authentication rather than crashing.

2. **`whatsapp-mcp-server/` (Python)**:
   - Implements the MCP server using the official MCP Python SDK (`FastMCP` interface).
   - Organizes tools into modular files by concern (`contacts.py`, `messages.py`, `media.py`, `groups.py`, `channels.py`, `admin.py`).
   - Enforces the confirmation mechanism for destructive actions.
   - Extracts and budgets text from PDF attachments without making internal LLM calls.

---

## Prerequisites

- **Go**: 1.22+ (tested and verified with Go 1.27.0 windows/amd64).
- **C Compiler (CGO)**: MinGW-W64 GCC (required by `mattn/go-sqlite3`).
- **Python**: 3.11+ (tested and verified with Python 3.13.2).
- **FFmpeg** (optional, recommended): Used to encode audio messages to `.ogg` Opus for voice notes. If not installed, audio files are sent as standard audio documents with an informative warning.

---

## Setup & Running

### 1. Build and Run the Go WhatsApp Bridge

```bash
cd whatsapp-bridge

# Build binary
go build -o bridge.exe .

# Start the bridge (binds strictly to 127.0.0.1:8080)
./bridge.exe -port 8080 -session-db whatsapp_session.db -storage-db whatsapp_data.db
```

#### QR Code Pairing:
- On initial startup, the terminal will display a half-block ASCII QR code:
  ```
  [Bridge] INFO: === NEW WHATSAPP QR CODE ===
  [Bridge] INFO: Scan this QR code in WhatsApp -> Linked Devices -> Link a Device:
  <ASCII QR CODE>
  ```
- Open WhatsApp on your primary phone, go to **Settings** > **Linked Devices** > **Link a Device**, and scan the code.
- Once paired, the credentials are saved to `whatsapp_session.db`. Future runs connect automatically without re-pairing.
- **Session Lifecycle**: WhatsApp periodically invalidates linked device sessions (roughly every ~20 days). When this occurs, the bridge emits a warning and prompts for re-authentication rather than terminating.

### 2. Set Up the Python MCP Server

```bash
cd whatsapp-mcp-server

# Create virtual environment
python -m venv .venv

# Activate virtual environment
# Windows:
.venv\Scripts\activate
# Linux/macOS:
source .venv/bin/activate

# Install pinned dependencies
pip install -r requirements.txt
```

### 3. Claude Desktop Configuration

Add the server to your Claude Desktop configuration (`%APPDATA%\Claude\claude_desktop_config.json` on Windows or `~/Library/Application Support/Claude/claude_desktop_config.json` on macOS):

```json
{
  "mcpServers": {
    "whatsapp": {
      "command": "C:\\Users\\Al-Mahdi\\whatsapp-mcp-server\\whatsapp-mcp-server\\.venv\\Scripts\\python.exe",
      "args": [
        "C:\\Users\\Al-Mahdi\\whatsapp-mcp-server\\whatsapp-mcp-server\\main.py"
      ],
      "env": {
        "WHATSAPP_BRIDGE_URL": "http://127.0.0.1:8080",
        "WHATSAPP_REQUEST_TIMEOUT": "20.0"
      }
    }
  }
}
```

---

## Confirmation Mechanism (Destructive Actions)

For dangerous or irreversible actions (`delete_message` and `set_group_admins_only`), an explicit confirmation gate is enforced in the Python MCP layer:

- Every destructive tool includes a `confirm: bool = False` argument.
- **When `confirm=False` (default)**: The action is **NOT** sent to the bridge. The tool returns a structured `pending_confirmation` response describing the blast radius:
  ```json
  {
    "status": "pending_confirmation",
    "action": "delete_message",
    "details": "This will delete message 'MSG12345' for everyone in chat '123456-789@g.us'.",
    "instructions": "Call delete_message again with confirm=True to execute this action."
  }
  ```
- **When `confirm=True`**: The tool executes the action on WhatsApp via the Go bridge and returns the result.

---

## Available MCP Tools (22 Total)

### Contacts (`contacts.py`)
1. **`search_contacts(query: str) -> list[dict]`**  
   Search contacts by name, push name, phone number, or JID.
2. **`get_direct_chat_by_contact(contact: str) -> dict`**  
   Retrieve direct (1-on-1) chat metadata for a contact identifier.
3. **`get_contact_chats(contact: str) -> list[dict]`**  
   Retrieve all chats (direct and group chats) involving a contact.

### Messages (`messages.py`)
4. **`list_chats(limit: int = 50) -> list[dict]`**  
   List recent WhatsApp conversations (direct chats, groups, channels) ordered by activity.
5. **`get_chat(chat_jid: str) -> dict`**  
   Retrieve chat details and participant list for a specific chat or group.
6. **`list_messages(chat_jid: str = None, query: str = None, since: int = None, until: int = None, limit: int = 50, offset: int = 0) -> list[dict]`**  
   List and search message history across all chats or within a specific conversation.
7. **`get_last_interaction(contact: str) -> dict`**  
   Retrieve the most recent message exchanged with or involving a contact.
8. **`get_message_context(message_id: str, before: int = 5, after: int = 5) -> list[dict]`**  
   Get surrounding conversation context for a message (before and after).
9. **`send_message(recipient_jid: str, text: str) -> dict`**  
   Send a text message to a user, group, or channel.
10. **`send_reaction(message_id: str, emoji: str, chat_jid: str = "") -> dict`**  
    Send an emoji reaction to a specific message.
11. **`delete_message(message_id: str, chat_jid: str = "", confirm: bool = False) -> dict`**  
    Revoke a message for everyone in the chat. **Requires confirmation (`confirm=True`)**.
12. **`mark_as_read(message_id: str, chat_jid: str = "") -> dict`**  
    Send a read receipt for a specific message.

### Media & Documents (`media.py`)
13. **`send_file(recipient_jid: str, file_path: str, caption: str = "") -> dict`**  
    Send a document, image, or media file from local disk.
14. **`send_audio_message(recipient_jid: str, file_path: str) -> dict`**  
    Send an audio message / voice note. Automatically converts audio to `.ogg` Opus via FFmpeg if present, otherwise sends as standard audio file with a warning.
15. **`download_media(message_id: str, chat_jid: str = "") -> dict`**  
    Download media attachment from a message to the local cache and return its file path.
16. **`get_group_pdfs(chat_jid: str, since: int = None, until: int = None, max_pages_per_pdf: int = 10, max_chars_total: int = 50000) -> dict`**  
    Scans group history for PDF attachments, downloads them, extracts text using `pypdf`, and returns a structured per-file text breakdown within character budget constraints. **Does not make internal LLM calls**.

### Groups (`groups.py`)
17. **`create_group(name: str, participants: list[str]) -> dict`**  
    Create a new WhatsApp group chat with initial members.
18. **`add_participant(group_jid: str, participant: str) -> dict`**  
    Add a user to a group. Requires admin privileges.
19. **`remove_participant(group_jid: str, participant: str) -> dict`**  
    Remove a user from a group. Requires admin privileges.
20. **`get_group_invite_link(group_jid: str) -> dict`**  
    Get the invite link for a group. Requires admin privileges.

### Channels (`channels.py`)
21. **`create_channel(name: str, description: str = "") -> dict`**  
    Create a broadcast channel with `@newsletter` JID suffix.

### Moderation / Admin (`admin.py`)
22. **`set_group_admins_only(group_jid: str, enabled: bool, confirm: bool = False) -> dict`**  
    Toggle WhatsApp Announcement Mode (restricting sending to admins only). Applies only to groups (`@g.us`). **Requires confirmation (`confirm=True`)** and admin privileges.

---

## Pinned Dependency Versions

### Go Bridge (`whatsapp-bridge/go.mod`)
- `go.mau.fi/whatsmeow`: `v0.0.0-20260925162019-b3832c2bd1d1` (verified to support `CreateNewsletter` and `SetGroupAnnounce`)
- `github.com/mattn/go-sqlite3`: `v1.14.52`
- `github.com/mdp/qrterminal/v3`: `v3.2.1`
- `google.golang.org/protobuf`: `v1.36.12`

### Python MCP Server (`whatsapp-mcp-server/requirements.txt`)
- `mcp`: `1.30.0` (official Python SDK with `FastMCP`)
- `pypdf`: `6.19.0` (pure-Python PDF extraction and password-protection detection)
- `httpx`: `0.28.1` (HTTP client for bridge communication)
- `pytest`: `9.1.1`
- `pytest-asyncio`: `1.4.0`
- `ruff`: `0.16.9`

---

## Verification & Quality Assurance

### What Was Tested and Passed

1. **Build & Compilation Verification**:
   - `go build ./...` compiled cleanly with 0 errors.
   - `go vet ./...` executed with 0 warnings.
   - Python virtual environment created and all dependencies installed with 0 import errors.
   - FastMCP tool registration validated: all 22 tools registered with valid schemas and signatures.

2. **Static Analysis & Linting**:
   - **Go**: `staticcheck ./...` executed with 0 errors.
   - **Go**: `go vet ./...` executed with 0 warnings.
   - **Python**: `ruff check .` executed with 0 errors across all modules and tests.

3. **Automated Unit & Integration Tests**:
   - **Go Bridge Tests (`storage_test.go`, `server_test.go`)**:
     - SQLite schema initialization (WAL mode, foreign keys).
     - Contact insertion, retrieval, and search.
     - Message insertion, retrieval, context window query, and revocation.
     - PDF message filtering by date range.
     - Group participant management and admin permission verification.
     - Local-only HTTP endpoints (`/status`, `/contacts`, `/chats`, `/messages`).
     - Admin permission gating returning `403 Forbidden` for non-admins.
   - **Python Pure Logic Tests (`test_pdf_logic.py`)**:
     - Multi-page PDF text extraction.
     - Per-file character budget capping.
     - Global multi-file character budget capping (`cap_pdf_collection`).
     - Malformed/corrupted PDF error handling without crashing.
     - Password-protected/encrypted PDF detection reporting clear error.
     - Non-existent file error handling.
   - **Python Mocked-Bridge Tool Tests (`test_mcp_tools.py`)**:
     - All 22 MCP tools tested for valid inputs, error handling, and parameter validation.
     - Confirmation gating: `confirm=False` blocks execution without calling the bridge for `delete_message` and `set_group_admins_only`; `confirm=True` performs the call.
     - Admin permission checking: non-admin conditions in `add_participant`, `remove_participant`, `get_group_invite_link`, and `set_group_admins_only` return clear, human-readable errors.
     - Announcement Mode restriction: fails with clear error when applied to non-group JIDs (e.g. `@newsletter`).
     - Network timeout and connection failure simulation handling.
     - Missing contact and missing message ID edge cases.

### What Was Skipped and Why

- **Manual Live WhatsApp Account QR Pairing**:
  - **Reason**: This step requires an interactive human operator with a physical mobile device running the WhatsApp mobile application to scan a live camera QR code during the session. In this automated, non-interactive development environment, no live personal WhatsApp account or phone camera was available.
  - **Bridge Verification**: The complete QR generation pipeline (`whatsmeow.Client.GetQRChannel`, `qrterminal.GenerateHalfBlock`), authentication session persistence (`sqlstore`), reconnection logic, and event dispatching are fully implemented and verified via unit tests, build checks, and mocked bridge interactions.
