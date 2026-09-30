"""Configuration settings for the WhatsApp MCP Server."""

import os

# Go bridge URL (strictly localhost)
BRIDGE_URL: str = os.getenv("WHATSAPP_BRIDGE_URL", "http://127.0.0.1:8080")

# Request timeout in seconds
REQUEST_TIMEOUT: float = float(os.getenv("WHATSAPP_REQUEST_TIMEOUT", "20.0"))

# Maximum retries on transient connection errors
MAX_RETRIES: int = int(os.getenv("WHATSAPP_MAX_RETRIES", "3"))

# PDF Extraction limits
DEFAULT_MAX_PAGES_PER_PDF: int = int(os.getenv("WHATSAPP_PDF_MAX_PAGES", "10"))
DEFAULT_MAX_CHARS_TOTAL: int = int(os.getenv("WHATSAPP_PDF_MAX_CHARS", "50000"))

# Sentry Observability Configuration
SENTRY_DSN: str | None = os.getenv("SENTRY_DSN")
SENTRY_ENVIRONMENT: str = os.getenv("SENTRY_ENVIRONMENT", "production")
SENTRY_RELEASE: str | None = os.getenv("SENTRY_RELEASE")
SENTRY_TRACES_SAMPLE_RATE: float = float(os.getenv("SENTRY_TRACES_SAMPLE_RATE", "0.1"))
