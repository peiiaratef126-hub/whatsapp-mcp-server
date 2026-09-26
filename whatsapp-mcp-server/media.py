"""Media and document handling MCP tools."""

import os
import shutil
import subprocess
import tempfile
from typing import Any, Optional
import pypdf
from pypdf.errors import PdfReadError
from mcp.server.fastmcp import FastMCP
from client import BridgeClient, BridgeError
from config import DEFAULT_MAX_PAGES_PER_PDF, DEFAULT_MAX_CHARS_TOTAL


# Pure logic helpers for unit testing & extraction
def extract_text_from_pdf(
    file_path: str,
    max_pages: int = DEFAULT_MAX_PAGES_PER_PDF,
    char_budget: int = DEFAULT_MAX_CHARS_TOTAL,
) -> dict[str, Any]:
    """Extract text from a local PDF file up to max_pages and char_budget.

    Handles password-protected and malformed PDFs safely without crashing.
    """
    if not os.path.exists(file_path):
        return {
            "file_path": file_path,
            "filename": os.path.basename(file_path),
            "error": "File does not exist",
            "text": "",
            "page_count": 0,
            "truncated": False,
        }

    filename = os.path.basename(file_path)
    try:
        reader = pypdf.PdfReader(file_path)
    except PdfReadError as e:
        return {
            "file_path": file_path,
            "filename": filename,
            "error": f"Corrupted or malformed PDF: {e}",
            "text": "",
            "page_count": 0,
            "truncated": False,
        }
    except Exception as e:
        return {
            "file_path": file_path,
            "filename": filename,
            "error": f"Failed to open PDF: {e}",
            "text": "",
            "page_count": 0,
            "truncated": False,
        }

    # Check for password-protection / encryption
    if reader.is_encrypted:
        try:
            # Try blank password
            decrypt_res = reader.decrypt("")
            if decrypt_res == pypdf.PasswordType.NOT_DECRYPTED:
                return {
                    "file_path": file_path,
                    "filename": filename,
                    "error": "Password-protected or encrypted PDF (cannot extract text without password)",
                    "text": "",
                    "page_count": len(reader.pages),
                    "is_encrypted": True,
                    "truncated": False,
                }
        except Exception:
            return {
                "file_path": file_path,
                "filename": filename,
                "error": "Password-protected or encrypted PDF",
                "text": "",
                "page_count": 0,
                "is_encrypted": True,
                "truncated": False,
            }

    total_pages = len(reader.pages)
    pages_to_extract = min(max_pages, total_pages)
    extracted_text_chunks: list[str] = []
    total_chars = 0
    truncated = False

    for page_idx in range(pages_to_extract):
        try:
            page = reader.pages[page_idx]
            page_text = page.extract_text() or ""
        except Exception as e:
            page_text = f"\n[Error extracting page {page_idx + 1}: {e}]\n"

        if total_chars + len(page_text) > char_budget:
            remaining_chars = max(0, char_budget - total_chars)
            extracted_text_chunks.append(page_text[:remaining_chars])
            total_chars += remaining_chars
            truncated = True
            break
        else:
            extracted_text_chunks.append(page_text)
            total_chars += len(page_text)

    if total_pages > max_pages:
        truncated = True

    return {
        "file_path": file_path,
        "filename": filename,
        "page_count": total_pages,
        "pages_extracted": pages_to_extract,
        "char_count": total_chars,
        "truncated": truncated,
        "text": "\n\n--- Page Break ---\n\n".join(extracted_text_chunks).strip(),
    }


def cap_pdf_collection(
    extracted_files: list[dict[str, Any]],
    max_chars_total: int = DEFAULT_MAX_CHARS_TOTAL,
) -> dict[str, Any]:
    """Enforce a global character budget across a collection of extracted PDF documents."""
    remaining_budget = max_chars_total
    capped_files: list[dict[str, Any]] = []
    overall_truncated = False

    for doc in extracted_files:
        if "error" in doc:
            capped_files.append(doc)
            continue

        doc_text = doc.get("text", "")
        doc_len = len(doc_text)

        if doc_len <= remaining_budget:
            capped_files.append(doc)
            remaining_budget -= doc_len
        else:
            truncated_text = doc_text[:remaining_budget]
            doc_copy = dict(doc)
            doc_copy["text"] = truncated_text
            doc_copy["char_count"] = len(truncated_text)
            doc_copy["truncated"] = True
            capped_files.append(doc_copy)
            remaining_budget = 0
            overall_truncated = True

    total_chars = sum(len(f.get("text", "")) for f in capped_files)
    return {
        "total_files": len(capped_files),
        "total_characters": total_chars,
        "budget_limit": max_chars_total,
        "budget_exceeded": overall_truncated,
        "files": capped_files,
    }


def convert_audio_to_ogg_opus(input_path: str) -> tuple[str, bool, Optional[str]]:
    """Convert audio file to .ogg Opus format using ffmpeg if installed.

    Returns:
        (output_file_path, is_converted, warning_message)
    """
    ffmpeg_bin = shutil.which("ffmpeg")
    if not ffmpeg_bin:
        return (
            input_path,
            False,
            "ffmpeg is not installed or not in PATH; audio will be sent as a generic file attachment and will not appear as a playable voice note.",
        )

    # If already .ogg with opus, return directly
    ext = os.path.splitext(input_path)[1].lower()
    if ext == ".ogg" or ext == ".opus":
        return input_path, True, None

    temp_out = os.path.join(tempfile.gettempdir(), f"voice_note_{os.getpid()}_{os.path.basename(input_path)}.ogg")
    cmd = [
        ffmpeg_bin,
        "-y",
        "-i",
        input_path,
        "-c:a",
        "libopus",
        "-b:a",
        "32k",
        "-vbr",
        "on",
        temp_out,
    ]

    try:
        res = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
        if res.returncode == 0 and os.path.exists(temp_out) and os.path.getsize(temp_out) > 0:
            return temp_out, True, None
        return (
            input_path,
            False,
            f"ffmpeg conversion failed (code {res.returncode}): falling back to sending original file.",
        )
    except Exception as e:
        return (
            input_path,
            False,
            f"ffmpeg conversion error ({e}): falling back to sending original file.",
        )


def register_media_tools(mcp: FastMCP, client: BridgeClient) -> None:
    """Register media and document tools with FastMCP server."""

    @mcp.tool()
    def send_file(
        recipient_jid: str, file_path: str, caption: str = ""
    ) -> dict[str, Any]:
        """Send a document, image, or media file to a WhatsApp chat.

        Args:
            recipient_jid: Recipient user JID, group JID, or channel JID.
            file_path: Absolute or relative path to the local file to send.
            caption: Optional text caption accompanying the file.

        Returns:
            Result with success status, message ID, filename, and mime type.
        """
        if not recipient_jid or not recipient_jid.strip():
            return {"error": "recipient_jid cannot be empty"}
        if not file_path or not file_path.strip():
            return {"error": "file_path cannot be empty"}

        abs_path = os.path.abspath(file_path.strip())
        if not os.path.exists(abs_path):
            return {"error": f"File does not exist: {abs_path}"}

        try:
            return client.send_file(recipient_jid.strip(), abs_path, caption=caption)
        except BridgeError as e:
            return {"error": str(e), "file_path": abs_path}

    @mcp.tool()
    def send_audio_message(
        recipient_jid: str, file_path: str
    ) -> dict[str, Any]:
        """Send an audio message / voice note to a WhatsApp chat.

        Automatically converts audio to .ogg Opus format via ffmpeg if installed.
        If ffmpeg is missing, falls back to sending the file as a regular document with a warning.

        Args:
            recipient_jid: Recipient user JID or group JID.
            file_path: Local path to the audio file (e.g. mp3, wav, m4a, ogg).

        Returns:
            Result containing message ID and any ffmpeg conversion warnings.
        """
        if not recipient_jid or not recipient_jid.strip():
            return {"error": "recipient_jid cannot be empty"}
        if not file_path or not file_path.strip():
            return {"error": "file_path cannot be empty"}

        abs_path = os.path.abspath(file_path.strip())
        if not os.path.exists(abs_path):
            return {"error": f"Audio file does not exist: {abs_path}"}

        converted_path, is_converted, warning = convert_audio_to_ogg_opus(abs_path)
        try:
            if is_converted:
                res = client.send_audio_message(recipient_jid.strip(), converted_path)
            else:
                res = client.send_file(recipient_jid.strip(), abs_path, caption="[Audio Message]")

            if warning:
                res["warning"] = warning
            return res
        except BridgeError as e:
            return {"error": str(e), "file_path": abs_path}

    @mcp.tool()
    def download_media(
        message_id: str, chat_jid: str = ""
    ) -> dict[str, Any]:
        """Download media attachment from a WhatsApp message and return its local file path.

        Args:
            message_id: The ID of the message containing media.
            chat_jid: Optional chat JID where the message was sent.

        Returns:
            Dictionary with local file_path, filename, mime_type, and file size in bytes.
        """
        if not message_id or not message_id.strip():
            return {"error": "message_id cannot be empty"}
        try:
            return client.download_media(message_id.strip(), chat_jid=chat_jid.strip())
        except BridgeError as e:
            return {"error": str(e), "message_id": message_id}

    @mcp.tool()
    def get_group_pdfs(
        chat_jid: str,
        since: Optional[int] = None,
        until: Optional[int] = None,
        max_pages_per_pdf: int = DEFAULT_MAX_PAGES_PER_PDF,
        max_chars_total: int = DEFAULT_MAX_CHARS_TOTAL,
    ) -> dict[str, Any]:
        """Scan a group's message history for PDF attachments, download them, and extract their text.

        Returns a structured per-file breakdown of extracted text capped to the character budget,
        ready for the LLM to summarize. This tool does not make any internal LLM calls.

        Args:
            chat_jid: Group JID (e.g. 123456-789@g.us) to scan.
            since: Optional Unix timestamp (start date/time boundary).
            until: Optional Unix timestamp (end date/time boundary).
            max_pages_per_pdf: Maximum pages to extract per PDF file (default: 10).
            max_chars_total: Maximum total character budget across all extracted PDFs (default: 50,000).

        Returns:
            Dictionary with total_files, files list (each with filename, page_count, text), and budget info.
        """
        if not chat_jid or not chat_jid.strip():
            return {"error": "chat_jid cannot be empty"}

        try:
            pdf_messages = client.get_group_pdfs(chat_jid.strip(), since=since, until=until)
        except BridgeError as e:
            return {"error": str(e), "chat_jid": chat_jid}

        if not pdf_messages:
            return {
                "chat_jid": chat_jid,
                "total_files": 0,
                "files": [],
                "message": "No PDF attachments found in the specified chat and date range.",
            }

        extracted_docs: list[dict[str, Any]] = []
        for msg in pdf_messages:
            msg_id = msg.get("id")
            local_path = msg.get("media_path")

            # If not yet downloaded to local disk, download it
            if not local_path or not os.path.exists(local_path):
                try:
                    dl_res = client.download_media(msg_id, chat_jid=chat_jid)
                    local_path = dl_res.get("file_path")
                except BridgeError as e:
                    extracted_docs.append({
                        "message_id": msg_id,
                        "filename": msg.get("media_filename", f"{msg_id}.pdf"),
                        "error": f"Failed to download attachment: {e}",
                    })
                    continue

            if local_path and os.path.exists(local_path):
                doc_info = extract_text_from_pdf(
                    local_path,
                    max_pages=max_pages_per_pdf,
                    char_budget=max_chars_total,
                )
                doc_info["message_id"] = msg_id
                doc_info["timestamp"] = msg.get("timestamp")
                extracted_docs.append(doc_info)

        capped_collection = cap_pdf_collection(extracted_docs, max_chars_total=max_chars_total)
        capped_collection["chat_jid"] = chat_jid
        return capped_collection
