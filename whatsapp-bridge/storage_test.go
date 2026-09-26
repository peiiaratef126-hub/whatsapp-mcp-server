package main

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestStorageLifecycleAndQueries(t *testing.T) {
	tmpDB := "test_storage.db"
	defer os.Remove(tmpDB)

	s, err := NewStorage(tmpDB)
	if err != nil {
		t.Fatalf("Failed to initialize storage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// 1. Test Contact storage & search
	contact := Contact{
		JID:         "1234567890@s.whatsapp.net",
		PhoneNumber: "1234567890",
		Name:        "Alice Smith",
		PushName:    "Alice",
		IsBusiness:  false,
		UpdatedAt:   time.Now(),
	}
	if err := s.SaveContact(ctx, contact); err != nil {
		t.Fatalf("SaveContact failed: %v", err)
	}

	foundContacts, err := s.SearchContacts(ctx, "Alice")
	if err != nil {
		t.Fatalf("SearchContacts failed: %v", err)
	}
	if len(foundContacts) != 1 || foundContacts[0].Name != "Alice Smith" {
		t.Fatalf("Expected 1 contact named Alice Smith, got: %+v", foundContacts)
	}

	fetchedContact, err := s.GetContact(ctx, "1234567890")
	if err != nil || fetchedContact == nil {
		t.Fatalf("GetContact by phone failed: %v", err)
	}

	// 2. Test Message storage
	msg1 := Message{
		ID:        "MSG001",
		ChatJID:   "1234567890@s.whatsapp.net",
		SenderJID: "1234567890@s.whatsapp.net",
		Text:      "Hello world!",
		Timestamp: 1000,
		IsFromMe:  false,
	}
	msg2 := Message{
		ID:        "MSG002",
		ChatJID:   "1234567890@s.whatsapp.net",
		SenderJID: "myself@s.whatsapp.net",
		Text:      "Hey Alice!",
		Timestamp: 1005,
		IsFromMe:  true,
	}
	msg3 := Message{
		ID:            "MSG003",
		ChatJID:       "1234567890@s.whatsapp.net",
		SenderJID:     "1234567890@s.whatsapp.net",
		Text:          "Check this report",
		Timestamp:     1010,
		IsFromMe:      false,
		HasMedia:      true,
		MediaType:     "document",
		MediaFilename: "report.pdf",
		MediaMimeType: "application/pdf",
		MediaSize:     2048,
	}

	for _, m := range []Message{msg1, msg2, msg3} {
		if err := s.SaveMessage(ctx, m); err != nil {
			t.Fatalf("SaveMessage failed: %v", err)
		}
	}

	// 3. Test ListMessages & filters
	msgs, err := s.ListMessages(ctx, MessageFilter{ChatJID: "1234567890@s.whatsapp.net"})
	if err != nil {
		t.Fatalf("ListMessages failed: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("Expected 3 messages, got %d", len(msgs))
	}

	// 4. Test GetMessageContext
	ctxMsgs, err := s.GetMessageContext(ctx, "MSG002", 1, 1)
	if err != nil {
		t.Fatalf("GetMessageContext failed: %v", err)
	}
	if len(ctxMsgs) != 3 || ctxMsgs[1].ID != "MSG002" {
		t.Fatalf("Expected 3 context messages centered at MSG002, got %d", len(ctxMsgs))
	}

	// 5. Test GetGroupPDFs
	pdfs, err := s.GetGroupPDFs(ctx, "1234567890@s.whatsapp.net", nil, nil)
	if err != nil {
		t.Fatalf("GetGroupPDFs failed: %v", err)
	}
	if len(pdfs) != 1 || pdfs[0].MediaFilename != "report.pdf" {
		t.Fatalf("Expected 1 PDF message, got %+v", pdfs)
	}

	// 6. Test Group participants and Admin check
	groupJID := "123456789-987654@g.us"
	participants := []GroupParticipant{
		{GroupJID: groupJID, ParticipantJID: "admin@s.whatsapp.net", IsAdmin: true},
		{GroupJID: groupJID, ParticipantJID: "member@s.whatsapp.net", IsAdmin: false},
	}
	if err := s.SaveGroupParticipants(ctx, groupJID, participants); err != nil {
		t.Fatalf("SaveGroupParticipants failed: %v", err)
	}

	isAdmin, err := s.IsUserAdmin(ctx, groupJID, "admin@s.whatsapp.net")
	if err != nil || !isAdmin {
		t.Fatalf("Expected admin to be true, got %v (err: %v)", isAdmin, err)
	}

	isMemberAdmin, err := s.IsUserAdmin(ctx, groupJID, "member@s.whatsapp.net")
	if err != nil || isMemberAdmin {
		t.Fatalf("Expected member admin to be false, got %v (err: %v)", isMemberAdmin, err)
	}

	// 7. Test GetLastInteraction
	lastInt, err := s.GetLastInteraction(ctx, "1234567890@s.whatsapp.net")
	if err != nil || lastInt == nil {
		t.Fatalf("GetLastInteraction failed: %v", err)
	}
	if lastInt.ID != "MSG003" {
		t.Fatalf("Expected last interaction to be MSG003, got %s", lastInt.ID)
	}

	// 8. Test Revoke message
	if err := s.MarkMessageRevoked(ctx, "MSG001"); err != nil {
		t.Fatalf("MarkMessageRevoked failed: %v", err)
	}
	revoked, err := s.GetMessage(ctx, "MSG001")
	if err != nil || revoked == nil || !revoked.IsRevoked {
		t.Fatalf("Expected message to be revoked, got %+v", revoked)
	}
}
