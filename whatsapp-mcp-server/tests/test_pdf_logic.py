"""Unit tests for pure PDF text extraction and character budget capping logic."""

import os
import tempfile

from pypdf import PdfWriter

from media import cap_pdf_collection, extract_text_from_pdf


def create_sample_pdf(pages_text: list[str], password: str | None = None) -> str:
    """Create a temporary PDF file with given text pages and optional encryption."""
    writer = PdfWriter()
    for _ in pages_text:
        _ = writer.add_blank_page(width=200, height=200)
    if password:
        writer.encrypt(password)

    with tempfile.NamedTemporaryFile(suffix=".pdf", delete=False) as tmp:
        writer.write(tmp)
        tmp_name = tmp.name
    return tmp_name


def create_text_pdf(text: str) -> str:
    """Create a valid PDF containing extractable text."""
    pdf_content = (
        b"%PDF-1.4\n"
        b"1 0 obj <</Type /Catalog /Pages 2 0 R>> endobj\n"
        b"2 0 obj <</Type /Pages /Kids [3 0 R] /Count 1>> endobj\n"
        b"3 0 obj <</Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >> endobj\n"
        b"4 0 obj <</Length 44>> stream\n"
        b"BT /F1 12 Tf 50 250 Td (" + text.encode("latin-1", "replace") + b") Tj ET\n"
        b"endstream endobj\n"
        b"5 0 obj <</Type /Font /Subtype /Type1 /BaseFont /Helvetica>> endobj\n"
        b"xref\n0 6\n0000000000 65535 f \n0000000009 00000 n \n0000000058 00000 n \n0000000115 00000 n \n0000000244 00000 n \n0000000338 00000 n \n"
        b"trailer <</Size 6 /Root 1 0 R>>\nstartxref\n417\n%%EOF"
    )
    with tempfile.NamedTemporaryFile(suffix=".pdf", delete=False) as tmp:
        tmp.write(pdf_content)
        tmp_name = tmp.name
    return tmp_name


def test_extract_text_from_valid_pdf():
    """Test extracting text from a valid PDF."""
    path = create_text_pdf("Monthly Financial Report")
    try:
        res = extract_text_from_pdf(path, max_pages=5, char_budget=1000)
        assert "Monthly Financial Report" in res["text"]
        assert res["page_count"] == 1
        assert not res["truncated"]
        assert "error" not in res
    finally:
        os.unlink(path)


def test_char_budget_capping_single_pdf():
    """Test that a character budget caps the extracted text."""
    long_text = "A" * 500
    path = create_text_pdf(long_text)
    try:
        res = extract_text_from_pdf(path, max_pages=5, char_budget=50)
        assert len(res["text"]) <= 50
        assert res["truncated"] is True
        assert res["char_count"] <= 50
    finally:
        os.unlink(path)


def test_cap_pdf_collection_budget():
    """Test multi-PDF collection character capping."""
    docs = [
        {"filename": "doc1.pdf", "text": "X" * 100, "page_count": 1},
        {"filename": "doc2.pdf", "text": "Y" * 100, "page_count": 1},
        {"filename": "doc3.pdf", "text": "Z" * 100, "page_count": 1},
    ]

    result = cap_pdf_collection(docs, max_chars_total=150)
    assert result["total_characters"] == 150
    assert result["budget_exceeded"] is True
    assert len(result["files"]) == 3
    assert len(result["files"][0]["text"]) == 100
    assert len(result["files"][1]["text"]) == 50
    assert result["files"][1].get("truncated") is True
    assert len(result["files"][2]["text"]) == 0


def test_malformed_pdf_handling():
    """Test that malformed/corrupted PDF files are handled cleanly without crashing."""
    with tempfile.NamedTemporaryFile(suffix=".pdf", delete=False) as tmp:
        tmp.write(b"NOT A REAL PDF FILE DATA JUNK CONTENT %%%")
        tmp_name = tmp.name

    try:
        res = extract_text_from_pdf(tmp_name)
        assert "error" in res
        assert res["text"] == ""
    finally:
        os.unlink(tmp_name)


def test_password_protected_pdf_handling():
    """Test that password-protected PDFs are detected and report clear error."""
    writer = PdfWriter()
    _ = writer.add_blank_page(width=100, height=100)
    writer.encrypt("super_secret_password")

    with tempfile.NamedTemporaryFile(suffix=".pdf", delete=False) as tmp:
        writer.write(tmp)
        tmp_name = tmp.name

    try:
        res = extract_text_from_pdf(tmp_name)
        assert "error" in res
        assert "password" in res["error"].lower() or res.get("is_encrypted") is True
        assert res["text"] == ""
    finally:
        os.unlink(tmp_name)


def test_nonexistent_pdf_handling():
    """Test that non-existent PDF files return clear error without crashing."""
    res = extract_text_from_pdf("nonexistent_document_12345.pdf")
    assert "error" in res
    assert res["text"] == ""
