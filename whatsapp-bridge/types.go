package main

import (
	"time"
)

// Contact represents a WhatsApp contact.
type Contact struct {
	JID         string    `json:"jid"`
	PhoneNumber string    `json:"phone_number"`
	Name        string    `json:"name"`
	PushName    string    `json:"push_name"`
	IsBusiness  bool      `json:"is_business"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Chat represents a 1-on-1 chat, group chat, or broadcast channel.
type Chat struct {
	JID                  string    `json:"jid"`
	Name                 string    `json:"name"`
	IsGroup              bool      `json:"is_group"`
	IsChannel            bool      `json:"is_channel"`
	UnreadCount          int       `json:"unread_count"`
	LastMessageID        string    `json:"last_message_id,omitempty"`
	LastMessageText      string    `json:"last_message_text,omitempty"`
	LastMessageTimestamp int64     `json:"last_message_timestamp,omitempty"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// GroupParticipant represents a member in a group.
type GroupParticipant struct {
	GroupJID       string `json:"group_jid"`
	ParticipantJID string `json:"participant_jid"`
	IsAdmin        bool   `json:"is_admin"`
	IsSuperAdmin   bool   `json:"is_superadmin"`
}

// ChatWithParticipants is a detailed chat view with participant list.
type ChatWithParticipants struct {
	Chat
	Participants []GroupParticipant `json:"participants,omitempty"`
}

// Message represents an ingested or sent WhatsApp message.
type Message struct {
	ID            string    `json:"id"`
	ChatJID       string    `json:"chat_jid"`
	SenderJID     string    `json:"sender_jid"`
	Text          string    `json:"text"`
	Timestamp     int64     `json:"timestamp"`
	IsFromMe      bool      `json:"is_from_me"`
	HasMedia      bool      `json:"has_media"`
	MediaType     string    `json:"media_type,omitempty"`
	MediaFilename string    `json:"media_filename,omitempty"`
	MediaMimeType string    `json:"media_mimetype,omitempty"`
	MediaPath     string    `json:"media_path,omitempty"`
	MediaSize     int64     `json:"media_size,omitempty"`
	IsRevoked     bool      `json:"is_revoked"`
	CreatedAt     time.Time `json:"created_at"`
}

// MessageFilter defines criteria for querying message history.
type MessageFilter struct {
	ChatJID   string `json:"chat_jid,omitempty"`
	SenderJID string `json:"sender_jid,omitempty"`
	Query     string `json:"query,omitempty"`
	Since     *int64 `json:"since,omitempty"`
	Until     *int64 `json:"until,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
}
