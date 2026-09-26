package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func setupTestServer(t *testing.T) (*HTTPServer, *Storage, func()) {
	tmpDB := "test_server_storage.db"
	s, err := NewStorage(tmpDB)
	if err != nil {
		t.Fatalf("Failed to initialize test storage: %v", err)
	}

	bridge := &WhatsAppBridge{
		storage: s,
	}
	srv := NewHTTPServer(bridge, 0)

	cleanup := func() {
		_ = s.Close()
		_ = os.Remove(tmpDB)
	}

	return srv, s, cleanup
}

func TestHTTPServerReadEndpoints(t *testing.T) {
	srv, s, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Seed data
	_ = s.SaveContact(ctx, Contact{
		JID:         "111222333@s.whatsapp.net",
		PhoneNumber: "111222333",
		Name:        "Bob Jones",
		PushName:    "Bob",
		UpdatedAt:   time.Now(),
	})

	_ = s.SaveChat(ctx, Chat{
		JID:                  "111222333@s.whatsapp.net",
		Name:                 "Bob Jones",
		IsGroup:              false,
		LastMessageText:      "Hey there!",
		LastMessageTimestamp: 1700000000,
	})

	_ = s.SaveMessage(ctx, Message{
		ID:        "M1",
		ChatJID:   "111222333@s.whatsapp.net",
		SenderJID: "111222333@s.whatsapp.net",
		Text:      "Hey there!",
		Timestamp: 1700000000,
	})

	// 1. Test /status
	req := httptest.NewRequest("GET", "/status", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 on /status, got %d", w.Code)
	}

	// 2. Test /contacts?query=Bob
	req = httptest.NewRequest("GET", "/contacts?query=Bob", nil)
	w = httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Bob Jones") {
		t.Fatalf("Expected Bob Jones in /contacts response, got %s", w.Body.String())
	}

	// 3. Test /contacts/direct?contact=111222333
	req = httptest.NewRequest("GET", "/contacts/direct?contact=111222333", nil)
	w = httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "111222333") {
		t.Fatalf("Expected direct chat in response, got %s", w.Body.String())
	}

	// 4. Test /contacts/chats?contact=111222333
	req = httptest.NewRequest("GET", "/contacts/chats?contact=111222333", nil)
	w = httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 on /contacts/chats, got %d", w.Code)
	}

	// 5. Test /contacts/last_interaction?contact=111222333
	req = httptest.NewRequest("GET", "/contacts/last_interaction?contact=111222333", nil)
	w = httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "M1") {
		t.Fatalf("Expected M1 in last interaction, got %s", w.Body.String())
	}

	// 6. Test /chats
	req = httptest.NewRequest("GET", "/chats", nil)
	w = httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "111222333@s.whatsapp.net") {
		t.Fatalf("Expected chats list, got %s", w.Body.String())
	}

	// 7. Test /messages?chat_jid=111222333@s.whatsapp.net
	req = httptest.NewRequest("GET", "/messages?chat_jid=111222333@s.whatsapp.net", nil)
	w = httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Hey there!") {
		t.Fatalf("Expected message in /messages, got %s", w.Body.String())
	}

	// 8. Test /messages/context?message_id=M1
	req = httptest.NewRequest("GET", "/messages/context?message_id=M1", nil)
	w = httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "M1") {
		t.Fatalf("Expected context with M1, got %s", w.Body.String())
	}
}

func TestHTTPServerAdminGating(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	// When user is not logged in / not admin, admin actions must return 403 Forbidden
	payload := `{"group_jid": "123456-789@g.us", "participant": "555@s.whatsapp.net"}`
	req := httptest.NewRequest("POST", "/groups/participants/add", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden when not admin, got %d (body: %s)", w.Code, w.Body.String())
	}

	if !strings.Contains(strings.ToLower(w.Body.String()), "permission") && !strings.Contains(w.Body.String(), "failed") {
		t.Fatalf("Expected clear permission error message, got: %s", w.Body.String())
	}

	// Test announce_only admin gating
	announcePayload := `{"group_jid": "123456-789@g.us", "enabled": true}`
	req = httptest.NewRequest("POST", "/groups/admin/announce_only", strings.NewReader(announcePayload))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden for announce_only when not admin, got %d", w.Code)
	}
}
