# WhatsApp MCP Server

A Model Context Protocol (MCP) server that connects to a personal WhatsApp account through the unofficial WhatsApp Web multi-device protocol using `whatsmeow` and the official MCP Python SDK.

## Architecture

- **`whatsapp-bridge/`**: Go bridge using `whatsmeow` for QR pairing, multi-device session management, SQLite persistence, and local HTTP/RPC endpoints.
- **`whatsapp-mcp-server/`**: Python MCP server exposing tools to LLM clients (such as Claude Desktop).
