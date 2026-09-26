package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Storage handles local SQLite persistence for contacts, chats, messages, and groups.
type Storage struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewStorage opens or creates the SQLite storage database and initializes schema.
func NewStorage(dbPath string) (*Storage, error) {
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000", dbPath))
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// Optimize connection pool for local concurrent access
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	s := &Storage{db: db}
	if err := s.initSchema(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return s, nil
}

func (s *Storage) initSchema() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS contacts (
			jid TEXT PRIMARY KEY,
			phone_number TEXT,
			name TEXT,
			push_name TEXT,
			is_business INTEGER DEFAULT 0,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS chats (
			jid TEXT PRIMARY KEY,
			name TEXT,
			is_group INTEGER DEFAULT 0,
			is_channel INTEGER DEFAULT 0,
			unread_count INTEGER DEFAULT 0,
			last_message_id TEXT,
			last_message_text TEXT,
			last_message_timestamp INTEGER DEFAULT 0,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			chat_jid TEXT NOT NULL,
			sender_jid TEXT NOT NULL,
			text TEXT,
			timestamp INTEGER NOT NULL,
			is_from_me INTEGER DEFAULT 0,
			has_media INTEGER DEFAULT 0,
			media_type TEXT,
			media_filename TEXT,
			media_mimetype TEXT,
			media_path TEXT,
			media_size INTEGER DEFAULT 0,
			is_revoked INTEGER DEFAULT 0,
			raw_data TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS group_participants (
			group_jid TEXT NOT NULL,
			participant_jid TEXT NOT NULL,
			is_admin INTEGER DEFAULT 0,
			is_superadmin INTEGER DEFAULT 0,
			PRIMARY KEY (group_jid, participant_jid)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_messages_chat_ts ON messages(chat_jid, timestamp DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_messages_sender ON messages(sender_jid);`,
		`CREATE INDEX IF NOT EXISTS idx_messages_text ON messages(text);`,
		`CREATE INDEX IF NOT EXISTS idx_chats_last_ts ON chats(last_message_timestamp DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_group_parts_part ON group_participants(participant_jid);`,
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("executing %s: %w", q, err)
		}
	}

	return nil
}

// Close closes the database connection.
func (s *Storage) Close() error {
	return s.db.Close()
}

// SaveContact inserts or updates a contact.
func (s *Storage) SaveContact(ctx context.Context, c Contact) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `INSERT INTO contacts (jid, phone_number, name, push_name, is_business, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(jid) DO UPDATE SET
			phone_number = COALESCE(NULLIF(excluded.phone_number, ''), contacts.phone_number),
			name = COALESCE(NULLIF(excluded.name, ''), contacts.name),
			push_name = COALESCE(NULLIF(excluded.push_name, ''), contacts.push_name),
			is_business = excluded.is_business,
			updated_at = CURRENT_TIMESTAMP;`

	isBiz := 0
	if c.IsBusiness {
		isBiz = 1
	}

	_, err := s.db.ExecContext(ctx, query, c.JID, c.PhoneNumber, c.Name, c.PushName, isBiz)
	return err
}

// SaveChat inserts or updates chat summary.
func (s *Storage) SaveChat(ctx context.Context, c Chat) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `INSERT INTO chats (jid, name, is_group, is_channel, unread_count, last_message_id, last_message_text, last_message_timestamp, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(jid) DO UPDATE SET
			name = COALESCE(NULLIF(excluded.name, ''), chats.name),
			is_group = excluded.is_group,
			is_channel = excluded.is_channel,
			unread_count = excluded.unread_count,
			last_message_id = COALESCE(NULLIF(excluded.last_message_id, ''), chats.last_message_id),
			last_message_text = COALESCE(NULLIF(excluded.last_message_text, ''), chats.last_message_text),
			last_message_timestamp = MAX(chats.last_message_timestamp, excluded.last_message_timestamp),
			updated_at = CURRENT_TIMESTAMP;`

	isGrp, isChn := 0, 0
	if c.IsGroup {
		isGrp = 1
	}
	if c.IsChannel {
		isChn = 1
	}

	_, err := s.db.ExecContext(ctx, query, c.JID, c.Name, isGrp, isChn, c.UnreadCount, c.LastMessageID, c.LastMessageText, c.LastMessageTimestamp)
	return err
}

// SaveMessage inserts or updates an ingested message and updates corresponding chat.
func (s *Storage) SaveMessage(ctx context.Context, m Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	msgQuery := `INSERT INTO messages (
			id, chat_jid, sender_jid, text, timestamp, is_from_me, has_media,
			media_type, media_filename, media_mimetype, media_path, media_size, is_revoked, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			text = excluded.text,
			has_media = excluded.has_media,
			media_type = excluded.media_type,
			media_filename = excluded.media_filename,
			media_mimetype = excluded.media_mimetype,
			media_path = COALESCE(NULLIF(excluded.media_path, ''), messages.media_path),
			media_size = excluded.media_size,
			is_revoked = excluded.is_revoked;`

	fromMe, hasMed, isRev := 0, 0, 0
	if m.IsFromMe {
		fromMe = 1
	}
	if m.HasMedia {
		hasMed = 1
	}
	if m.IsRevoked {
		isRev = 1
	}

	_, err = tx.ExecContext(ctx, msgQuery,
		m.ID, m.ChatJID, m.SenderJID, m.Text, m.Timestamp,
		fromMe, hasMed, m.MediaType, m.MediaFilename, m.MediaMimeType,
		m.MediaPath, m.MediaSize, isRev,
	)
	if err != nil {
		return fmt.Errorf("inserting message: %w", err)
	}

	// Update chat record
	chatQuery := `INSERT INTO chats (jid, name, is_group, is_channel, unread_count, last_message_id, last_message_text, last_message_timestamp, updated_at)
		VALUES (?, ?, ?, ?, 0, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(jid) DO UPDATE SET
			last_message_id = excluded.last_message_id,
			last_message_text = excluded.last_message_text,
			last_message_timestamp = MAX(chats.last_message_timestamp, excluded.last_message_timestamp),
			updated_at = CURRENT_TIMESTAMP;`

	isGroup := strings.HasSuffix(m.ChatJID, "@g.us")
	isChannel := strings.HasSuffix(m.ChatJID, "@newsletter")
	isGrp, isChn := 0, 0
	if isGroup {
		isGrp = 1
	}
	if isChannel {
		isChn = 1
	}

	_, err = tx.ExecContext(ctx, chatQuery, m.ChatJID, "", isGrp, isChn, m.ID, m.Text, m.Timestamp)
	if err != nil {
		return fmt.Errorf("updating chat from message: %w", err)
	}

	return tx.Commit()
}

// MarkMessageRevoked sets is_revoked = 1 for a deleted message.
func (s *Storage) MarkMessageRevoked(ctx context.Context, messageID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, `UPDATE messages SET is_revoked = 1, text = '[Message deleted]' WHERE id = ?`, messageID)
	return err
}

// UpdateMediaPath updates the local file path for a downloaded media message.
func (s *Storage) UpdateMediaPath(ctx context.Context, messageID, path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, `UPDATE messages SET media_path = ? WHERE id = ?`, path, messageID)
	return err
}

// SaveGroupParticipants replaces or updates group members.
func (s *Storage) SaveGroupParticipants(ctx context.Context, groupJID string, participants []GroupParticipant) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `DELETE FROM group_participants WHERE group_jid = ?`, groupJID)
	if err != nil {
		return err
	}

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO group_participants (group_jid, participant_jid, is_admin, is_superadmin) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, p := range participants {
		isAdmin, isSuper := 0, 0
		if p.IsAdmin {
			isAdmin = 1
		}
		if p.IsSuperAdmin {
			isSuper = 1
		}
		if _, err := stmt.ExecContext(ctx, groupJID, p.ParticipantJID, isAdmin, isSuper); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// SearchContacts finds contacts matching query in name, push_name, phone_number, or JID.
func (s *Storage) SearchContacts(ctx context.Context, query string) ([]Contact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	q := strings.TrimSpace(query)
	searchPattern := "%" + q + "%"

	sqlQuery := `SELECT jid, phone_number, name, push_name, is_business, updated_at
		FROM contacts
		WHERE jid LIKE ? OR phone_number LIKE ? OR name LIKE ? OR push_name LIKE ?
		ORDER BY name ASC, push_name ASC
		LIMIT 50;`

	rows, err := s.db.QueryContext(ctx, sqlQuery, searchPattern, searchPattern, searchPattern, searchPattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contacts []Contact
	for rows.Next() {
		var c Contact
		var isBiz int
		var phone, name, push sql.NullString
		if err := rows.Scan(&c.JID, &phone, &name, &push, &isBiz, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.PhoneNumber = phone.String
		c.Name = name.String
		c.PushName = push.String
		c.IsBusiness = isBiz == 1
		contacts = append(contacts, c)
	}

	return contacts, rows.Err()
}

// GetContact retrieves a contact by exact JID or phone number or closest match.
func (s *Storage) GetContact(ctx context.Context, identifier string) (*Contact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cleanID := strings.TrimSpace(identifier)
	sqlQuery := `SELECT jid, phone_number, name, push_name, is_business, updated_at
		FROM contacts
		WHERE jid = ? OR phone_number = ? OR jid LIKE ? OR name LIKE ?
		LIMIT 1;`

	var c Contact
	var isBiz int
	var phone, name, push sql.NullString

	pattern := "%" + cleanID + "%"
	err := s.db.QueryRowContext(ctx, sqlQuery, cleanID, cleanID, pattern, pattern).
		Scan(&c.JID, &phone, &name, &push, &isBiz, &c.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	c.PhoneNumber = phone.String
	c.Name = name.String
	c.PushName = push.String
	c.IsBusiness = isBiz == 1
	return &c, nil
}

// ListChats retrieves all chats ordered by most recent activity.
func (s *Storage) ListChats(ctx context.Context, limit int) ([]Chat, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > 200 {
		limit = 50
	}

	query := `SELECT jid, name, is_group, is_channel, unread_count, last_message_id, last_message_text, last_message_timestamp, updated_at
		FROM chats
		ORDER BY last_message_timestamp DESC, updated_at DESC
		LIMIT ?;`

	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chats []Chat
	for rows.Next() {
		var c Chat
		var isGrp, isChn int
		var name, lastID, lastText sql.NullString
		if err := rows.Scan(&c.JID, &name, &isGrp, &isChn, &c.UnreadCount, &lastID, &lastText, &c.LastMessageTimestamp, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.Name = name.String
		c.IsGroup = isGrp == 1
		c.IsChannel = isChn == 1
		c.LastMessageID = lastID.String
		c.LastMessageText = lastText.String
		chats = append(chats, c)
	}

	return chats, rows.Err()
}

// GetChat retrieves a single chat by JID with participants if it's a group.
func (s *Storage) GetChat(ctx context.Context, jid string) (*ChatWithParticipants, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT jid, name, is_group, is_channel, unread_count, last_message_id, last_message_text, last_message_timestamp, updated_at
		FROM chats WHERE jid = ?;`

	var c ChatWithParticipants
	var isGrp, isChn int
	var name, lastID, lastText sql.NullString

	err := s.db.QueryRowContext(ctx, query, jid).Scan(
		&c.JID, &name, &isGrp, &isChn, &c.UnreadCount,
		&lastID, &lastText, &c.LastMessageTimestamp, &c.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	c.Name = name.String
	c.IsGroup = isGrp == 1
	c.IsChannel = isChn == 1
	c.LastMessageID = lastID.String
	c.LastMessageText = lastText.String

	if c.IsGroup {
		partRows, err := s.db.QueryContext(ctx, `SELECT group_jid, participant_jid, is_admin, is_superadmin FROM group_participants WHERE group_jid = ?`, jid)
		if err == nil {
			defer partRows.Close()
			for partRows.Next() {
				var p GroupParticipant
				var isAdmin, isSuper int
				if err := partRows.Scan(&p.GroupJID, &p.ParticipantJID, &isAdmin, &isSuper); err == nil {
					p.IsAdmin = isAdmin == 1
					p.IsSuperAdmin = isSuper == 1
					c.Participants = append(c.Participants, p)
				}
			}
		}
	}

	return &c, nil
}

// GetDirectChatByContact finds the 1-on-1 direct chat with a contact.
func (s *Storage) GetDirectChatByContact(ctx context.Context, contactIdentifier string) (*Chat, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	contact, err := s.GetContact(ctx, contactIdentifier)
	if err != nil {
		return nil, err
	}

	targetJID := contactIdentifier
	if contact != nil && contact.JID != "" {
		targetJID = contact.JID
	}
	if !strings.Contains(targetJID, "@") {
		targetJID = targetJID + "@s.whatsapp.net"
	}

	query := `SELECT jid, name, is_group, is_channel, unread_count, last_message_id, last_message_text, last_message_timestamp, updated_at
		FROM chats WHERE jid = ? AND is_group = 0 AND is_channel = 0;`

	var c Chat
	var isGrp, isChn int
	var name, lastID, lastText sql.NullString

	err = s.db.QueryRowContext(ctx, query, targetJID).Scan(
		&c.JID, &name, &isGrp, &isChn, &c.UnreadCount,
		&lastID, &lastText, &c.LastMessageTimestamp, &c.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			// Return a minimal constructed chat if contact is valid but no messages exchanged yet
			if contact != nil {
				chatName := contact.Name
				if chatName == "" {
					chatName = contact.PushName
				}
				return &Chat{
					JID:       targetJID,
					Name:      chatName,
					IsGroup:   false,
					IsChannel: false,
				}, nil
			}
			return nil, nil
		}
		return nil, err
	}

	c.Name = name.String
	c.IsGroup = isGrp == 1
	c.IsChannel = isChn == 1
	c.LastMessageID = lastID.String
	c.LastMessageText = lastText.String

	return &c, nil
}

// GetContactChats returns all chats involving a contact (direct and group chats).
func (s *Storage) GetContactChats(ctx context.Context, contactIdentifier string) ([]Chat, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	contact, err := s.GetContact(ctx, contactIdentifier)
	if err != nil {
		return nil, err
	}

	targetJID := contactIdentifier
	if contact != nil && contact.JID != "" {
		targetJID = contact.JID
	}
	if !strings.Contains(targetJID, "@") {
		targetJID = targetJID + "@s.whatsapp.net"
	}

	query := `SELECT DISTINCT c.jid, c.name, c.is_group, c.is_channel, c.unread_count, c.last_message_id, c.last_message_text, c.last_message_timestamp, c.updated_at
		FROM chats c
		LEFT JOIN group_participants gp ON c.jid = gp.group_jid
		WHERE c.jid = ? OR gp.participant_jid = ?
		ORDER BY c.last_message_timestamp DESC;`

	rows, err := s.db.QueryContext(ctx, query, targetJID, targetJID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chats []Chat
	for rows.Next() {
		var c Chat
		var isGrp, isChn int
		var name, lastID, lastText sql.NullString
		if err := rows.Scan(&c.JID, &name, &isGrp, &isChn, &c.UnreadCount, &lastID, &lastText, &c.LastMessageTimestamp, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.Name = name.String
		c.IsGroup = isGrp == 1
		c.IsChannel = isChn == 1
		c.LastMessageID = lastID.String
		c.LastMessageText = lastText.String
		chats = append(chats, c)
	}

	return chats, rows.Err()
}

// GetLastInteraction retrieves the most recent message involving this contact.
func (s *Storage) GetLastInteraction(ctx context.Context, contactIdentifier string) (*Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	contact, err := s.GetContact(ctx, contactIdentifier)
	if err != nil {
		return nil, err
	}

	targetJID := contactIdentifier
	if contact != nil && contact.JID != "" {
		targetJID = contact.JID
	}
	if !strings.Contains(targetJID, "@") {
		targetJID = targetJID + "@s.whatsapp.net"
	}

	query := `SELECT id, chat_jid, sender_jid, text, timestamp, is_from_me, has_media,
			media_type, media_filename, media_mimetype, media_path, media_size, is_revoked, created_at
		FROM messages
		WHERE sender_jid = ? OR chat_jid = ?
		ORDER BY timestamp DESC
		LIMIT 1;`

	return s.scanSingleMessage(s.db.QueryRowContext(ctx, query, targetJID, targetJID))
}

// ListMessages retrieves messages matching filter.
func (s *Storage) ListMessages(ctx context.Context, f MessageFilter) ([]Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var conditions []string
	var args []interface{}

	if f.ChatJID != "" {
		conditions = append(conditions, "chat_jid = ?")
		args = append(args, f.ChatJID)
	}
	if f.SenderJID != "" {
		conditions = append(conditions, "sender_jid = ?")
		args = append(args, f.SenderJID)
	}
	if f.Query != "" {
		conditions = append(conditions, "text LIKE ?")
		args = append(args, "%"+strings.TrimSpace(f.Query)+"%")
	}
	if f.Since != nil {
		conditions = append(conditions, "timestamp >= ?")
		args = append(args, *f.Since)
	}
	if f.Until != nil {
		conditions = append(conditions, "timestamp <= ?")
		args = append(args, *f.Until)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	query := fmt.Sprintf(`SELECT id, chat_jid, sender_jid, text, timestamp, is_from_me, has_media,
			media_type, media_filename, media_mimetype, media_path, media_size, is_revoked, created_at
		FROM messages
		%s
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?;`, whereClause)

	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanMessages(rows)
}

// GetMessage retrieves a message by its ID.
func (s *Storage) GetMessage(ctx context.Context, id string) (*Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT id, chat_jid, sender_jid, text, timestamp, is_from_me, has_media,
			media_type, media_filename, media_mimetype, media_path, media_size, is_revoked, created_at
		FROM messages WHERE id = ?;`

	return s.scanSingleMessage(s.db.QueryRowContext(ctx, query, id))
}

// GetMessageContext retrieves surrounding messages before and after a specific message.
func (s *Storage) GetMessageContext(ctx context.Context, messageID string, before, after int) ([]Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	target, err := s.GetMessage(ctx, messageID)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, fmt.Errorf("message with id %s not found", messageID)
	}

	if before <= 0 {
		before = 5
	}
	if after <= 0 {
		after = 5
	}

	// Fetch before messages (timestamp < target.timestamp)
	beforeQuery := `SELECT id, chat_jid, sender_jid, text, timestamp, is_from_me, has_media,
			media_type, media_filename, media_mimetype, media_path, media_size, is_revoked, created_at
		FROM messages
		WHERE chat_jid = ? AND timestamp < ?
		ORDER BY timestamp DESC
		LIMIT ?;`

	beforeRows, err := s.db.QueryContext(ctx, beforeQuery, target.ChatJID, target.Timestamp, before)
	if err != nil {
		return nil, err
	}
	defer beforeRows.Close()

	beforeMsgs, err := s.scanMessages(beforeRows)
	if err != nil {
		return nil, err
	}

	// Reverse beforeMsgs so they are in chronological order
	var ordered []Message
	for i := len(beforeMsgs) - 1; i >= 0; i-- {
		ordered = append(ordered, beforeMsgs[i])
	}

	// Append target message
	ordered = append(ordered, *target)

	// Fetch after messages (timestamp > target.timestamp)
	afterQuery := `SELECT id, chat_jid, sender_jid, text, timestamp, is_from_me, has_media,
			media_type, media_filename, media_mimetype, media_path, media_size, is_revoked, created_at
		FROM messages
		WHERE chat_jid = ? AND timestamp > ?
		ORDER BY timestamp ASC
		LIMIT ?;`

	afterRows, err := s.db.QueryContext(ctx, afterQuery, target.ChatJID, target.Timestamp, after)
	if err != nil {
		return nil, err
	}
	defer afterRows.Close()

	afterMsgs, err := s.scanMessages(afterRows)
	if err != nil {
		return nil, err
	}

	ordered = append(ordered, afterMsgs...)
	return ordered, nil
}

// GetGroupPDFs retrieves all PDF document attachments from a chat within optional date boundaries.
func (s *Storage) GetGroupPDFs(ctx context.Context, chatJID string, since, until *int64) ([]Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var conditions []string
	var args []interface{}

	conditions = append(conditions, "chat_jid = ?")
	args = append(args, chatJID)

	conditions = append(conditions, "has_media = 1")
	conditions = append(conditions, "(LOWER(media_mimetype) LIKE '%pdf%' OR LOWER(media_filename) LIKE '%.pdf' OR media_type = 'document')")

	if since != nil {
		conditions = append(conditions, "timestamp >= ?")
		args = append(args, *since)
	}
	if until != nil {
		conditions = append(conditions, "timestamp <= ?")
		args = append(args, *until)
	}

	query := fmt.Sprintf(`SELECT id, chat_jid, sender_jid, text, timestamp, is_from_me, has_media,
			media_type, media_filename, media_mimetype, media_path, media_size, is_revoked, created_at
		FROM messages
		WHERE %s
		ORDER BY timestamp DESC;`, strings.Join(conditions, " AND "))

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanMessages(rows)
}

// IsUserAdmin checks whether the given participant is an admin or superadmin of the group.
func (s *Storage) IsUserAdmin(ctx context.Context, groupJID, userJID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Normalize userJID: strip device suffix
	cleanUser := userJID
	if idx := strings.Index(cleanUser, ":"); idx != -1 {
		cleanUser = cleanUser[:idx] + "@s.whatsapp.net"
	}

	query := `SELECT is_admin, is_superadmin FROM group_participants WHERE group_jid = ? AND (participant_jid = ? OR participant_jid LIKE ?);`
	pattern := "%" + strings.Split(cleanUser, "@")[0] + "%"

	var isAdmin, isSuper int
	err := s.db.QueryRowContext(ctx, query, groupJID, cleanUser, pattern).Scan(&isAdmin, &isSuper)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}

	return isAdmin == 1 || isSuper == 1, nil
}

// Helper to scan a single message row.
func (s *Storage) scanSingleMessage(row *sql.Row) (*Message, error) {
	var m Message
	var fromMe, hasMed, isRev int
	var text, mType, mFile, mMime, mPath sql.NullString
	var mSize sql.NullInt64

	err := row.Scan(
		&m.ID, &m.ChatJID, &m.SenderJID, &text, &m.Timestamp,
		&fromMe, &hasMed, &mType, &mFile, &mMime, &mPath, &mSize,
		&isRev, &m.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	m.Text = text.String
	m.IsFromMe = fromMe == 1
	m.HasMedia = hasMed == 1
	m.MediaType = mType.String
	m.MediaFilename = mFile.String
	m.MediaMimeType = mMime.String
	m.MediaPath = mPath.String
	m.MediaSize = mSize.Int64
	m.IsRevoked = isRev == 1

	return &m, nil
}

// Helper to scan multiple message rows.
func (s *Storage) scanMessages(rows *sql.Rows) ([]Message, error) {
	var messages []Message
	for rows.Next() {
		var m Message
		var fromMe, hasMed, isRev int
		var text, mType, mFile, mMime, mPath sql.NullString
		var mSize sql.NullInt64

		err := rows.Scan(
			&m.ID, &m.ChatJID, &m.SenderJID, &text, &m.Timestamp,
			&fromMe, &hasMed, &mType, &mFile, &mMime, &mPath, &mSize,
			&isRev, &m.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		m.Text = text.String
		m.IsFromMe = fromMe == 1
		m.HasMedia = hasMed == 1
		m.MediaType = mType.String
		m.MediaFilename = mFile.String
		m.MediaMimeType = mMime.String
		m.MediaPath = mPath.String
		m.MediaSize = mSize.Int64
		m.IsRevoked = isRev == 1

		messages = append(messages, m)
	}

	return messages, rows.Err()
}
