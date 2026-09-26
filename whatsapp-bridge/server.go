package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// HTTPServer handles local-only HTTP/RPC requests from the Python MCP layer.
type HTTPServer struct {
	bridge *WhatsAppBridge
	server *http.Server
	port   int
}

// NewHTTPServer initializes HTTP server bound strictly to 127.0.0.1.
func NewHTTPServer(bridge *WhatsAppBridge, port int) *HTTPServer {
	mux := http.NewServeMux()
	s := &HTTPServer{
		bridge: bridge,
		port:   port,
	}

	// Status & Health
	mux.HandleFunc("GET /status", s.handleStatus)
	mux.HandleFunc("GET /health", s.handleStatus)

	// Contacts
	mux.HandleFunc("GET /contacts", s.handleSearchContacts)
	mux.HandleFunc("GET /contacts/direct", s.handleGetDirectChatByContact)
	mux.HandleFunc("GET /contacts/chats", s.handleGetContactChats)
	mux.HandleFunc("GET /contacts/last_interaction", s.handleGetLastInteraction)

	// Chats
	mux.HandleFunc("GET /chats", s.handleListChats)
	mux.HandleFunc("GET /chats/detail", s.handleGetChat)

	// Messages
	mux.HandleFunc("GET /messages", s.handleListMessages)
	mux.HandleFunc("GET /messages/context", s.handleGetMessageContext)
	mux.HandleFunc("POST /send/message", s.handleSendMessage)
	mux.HandleFunc("POST /send/file", s.handleSendFile)
	mux.HandleFunc("POST /send/audio", s.handleSendAudio)
	mux.HandleFunc("POST /messages/reaction", s.handleSendReaction)
	mux.HandleFunc("POST /messages/delete", s.handleDeleteMessage)
	mux.HandleFunc("POST /messages/mark_read", s.handleMarkAsRead)

	// Media
	mux.HandleFunc("GET /media/download", s.handleDownloadMedia)
	mux.HandleFunc("GET /groups/pdfs", s.handleGetGroupPDFs)

	// Groups
	mux.HandleFunc("POST /groups/create", s.handleCreateGroup)
	mux.HandleFunc("POST /groups/participants/add", s.handleAddParticipant)
	mux.HandleFunc("POST /groups/participants/remove", s.handleRemoveParticipant)
	mux.HandleFunc("GET /groups/invite_link", s.handleGetGroupInviteLink)
	mux.HandleFunc("POST /groups/admin/announce_only", s.handleSetGroupAnnounceOnly)

	// Channels
	mux.HandleFunc("POST /channels/create", s.handleCreateChannel)

	s.server = &http.Server{
		Addr:         fmt.Sprintf("127.0.0.1:%d", port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	return s
}

// Start begins listening on 127.0.0.1.
func (s *HTTPServer) Start() error {
	s.bridge.logger.Infof("Bridge HTTP server listening strictly on 127.0.0.1:%d", s.port)
	return s.server.ListenAndServe()
}

// Shutdown gracefully stops the HTTP server.
func (s *HTTPServer) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// Helpers
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func parseJID(raw string) (types.JID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return types.EmptyJID, fmt.Errorf("JID cannot be empty")
	}

	// If already a valid JID format
	if strings.Contains(raw, "@") {
		parsed, err := types.ParseJID(raw)
		if err == nil {
			return parsed, nil
		}
	}

	// Normalize phone number: strip '+', spaces, dashes
	phone := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, raw)

	if phone == "" {
		return types.EmptyJID, fmt.Errorf("invalid JID or phone number: %s", raw)
	}

	return types.NewJID(phone, types.DefaultUserServer), nil
}

// verifyAdmin checks if the connected account is an admin of the specified group.
func (s *HTTPServer) verifyAdmin(ctx context.Context, groupJID string) error {
	status := s.bridge.GetStatus()
	if !status.LoggedIn || status.JID == "" {
		return fmt.Errorf("permission check failed: not connected to WhatsApp")
	}

	isAdmin, err := s.bridge.storage.IsUserAdmin(ctx, groupJID, status.JID)
	if err != nil {
		return fmt.Errorf("error verifying admin status: %w", err)
	}

	if !isAdmin {
		// Attempt live refresh of group info in case permissions updated
		parsedJID, pErr := parseJID(groupJID)
		if pErr == nil && s.bridge.client != nil && s.bridge.client.IsConnected() {
			info, gErr := s.bridge.client.GetGroupInfo(ctx, parsedJID)
			if gErr == nil && info != nil {
				var parts []GroupParticipant
				for _, p := range info.Participants {
					parts = append(parts, GroupParticipant{
						GroupJID:       groupJID,
						ParticipantJID: p.JID.ToNonAD().String(),
						IsAdmin:        p.IsAdmin,
						IsSuperAdmin:   p.IsSuperAdmin,
					})
				}
				_ = s.bridge.storage.SaveGroupParticipants(ctx, groupJID, parts)
				isAdmin, _ = s.bridge.storage.IsUserAdmin(ctx, groupJID, status.JID)
			}
		}
	}

	if !isAdmin {
		return fmt.Errorf("permission denied: the connected account (%s) is not an admin of group %s", status.JID, groupJID)
	}

	return nil
}

// Handlers

func (s *HTTPServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := s.bridge.GetStatus()
	writeJSON(w, http.StatusOK, status)
}

func (s *HTTPServer) handleSearchContacts(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	contacts, err := s.bridge.storage.SearchContacts(r.Context(), query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if contacts == nil {
		contacts = []Contact{}
	}
	writeJSON(w, http.StatusOK, contacts)
}

func (s *HTTPServer) handleGetDirectChatByContact(w http.ResponseWriter, r *http.Request) {
	contact := r.URL.Query().Get("contact")
	if contact == "" {
		writeError(w, http.StatusBadRequest, "missing 'contact' query parameter")
		return
	}

	chat, err := s.bridge.storage.GetDirectChatByContact(r.Context(), contact)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if chat == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("direct chat for contact %s not found", contact))
		return
	}

	writeJSON(w, http.StatusOK, chat)
}

func (s *HTTPServer) handleGetContactChats(w http.ResponseWriter, r *http.Request) {
	contact := r.URL.Query().Get("contact")
	if contact == "" {
		writeError(w, http.StatusBadRequest, "missing 'contact' query parameter")
		return
	}

	chats, err := s.bridge.storage.GetContactChats(r.Context(), contact)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if chats == nil {
		chats = []Chat{}
	}
	writeJSON(w, http.StatusOK, chats)
}

func (s *HTTPServer) handleGetLastInteraction(w http.ResponseWriter, r *http.Request) {
	contact := r.URL.Query().Get("contact")
	if contact == "" {
		writeError(w, http.StatusBadRequest, "missing 'contact' query parameter")
		return
	}

	msg, err := s.bridge.storage.GetLastInteraction(r.Context(), contact)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if msg == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("no interaction found with contact %s", contact))
		return
	}

	writeJSON(w, http.StatusOK, msg)
}

func (s *HTTPServer) handleListChats(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	chats, err := s.bridge.storage.ListChats(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if chats == nil {
		chats = []Chat{}
	}
	writeJSON(w, http.StatusOK, chats)
}

func (s *HTTPServer) handleGetChat(w http.ResponseWriter, r *http.Request) {
	jid := r.URL.Query().Get("jid")
	if jid == "" {
		writeError(w, http.StatusBadRequest, "missing 'jid' query parameter")
		return
	}

	chat, err := s.bridge.storage.GetChat(r.Context(), jid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if chat == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("chat %s not found", jid))
		return
	}

	writeJSON(w, http.StatusOK, chat)
}

func (s *HTTPServer) handleListMessages(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := MessageFilter{
		ChatJID:   q.Get("chat_jid"),
		SenderJID: q.Get("sender_jid"),
		Query:     q.Get("query"),
		Limit:     50,
	}

	if l := q.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			filter.Limit = parsed
		}
	}
	if o := q.Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			filter.Offset = parsed
		}
	}
	if s := q.Get("since"); s != "" {
		if parsed, err := strconv.ParseInt(s, 10, 64); err == nil {
			filter.Since = &parsed
		}
	}
	if u := q.Get("until"); u != "" {
		if parsed, err := strconv.ParseInt(u, 10, 64); err == nil {
			filter.Until = &parsed
		}
	}

	msgs, err := s.bridge.storage.ListMessages(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if msgs == nil {
		msgs = []Message{}
	}
	writeJSON(w, http.StatusOK, msgs)
}

func (s *HTTPServer) handleGetMessageContext(w http.ResponseWriter, r *http.Request) {
	msgID := r.URL.Query().Get("message_id")
	if msgID == "" {
		writeError(w, http.StatusBadRequest, "missing 'message_id' query parameter")
		return
	}

	before, after := 5, 5
	if b := r.URL.Query().Get("before"); b != "" {
		if parsed, err := strconv.Atoi(b); err == nil && parsed > 0 {
			before = parsed
		}
	}
	if a := r.URL.Query().Get("after"); a != "" {
		if parsed, err := strconv.Atoi(a); err == nil && parsed > 0 {
			after = parsed
		}
	}

	msgs, err := s.bridge.storage.GetMessageContext(r.Context(), msgID, before, after)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, msgs)
}

func (s *HTTPServer) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RecipientJID string `json:"recipient_jid"`
		Text         string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request JSON: "+err.Error())
		return
	}

	if req.RecipientJID == "" || strings.TrimSpace(req.Text) == "" {
		writeError(w, http.StatusBadRequest, "both 'recipient_jid' and non-empty 'text' are required")
		return
	}

	targetJID, err := parseJID(req.RecipientJID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid recipient JID: "+err.Error())
		return
	}

	if s.bridge.client == nil || !s.bridge.client.IsConnected() {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp client is not connected")
		return
	}

	text := req.Text
	msg := &waE2E.Message{Conversation: &text}

	resp, err := s.bridge.client.SendMessage(r.Context(), targetJID, msg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to send message: "+err.Error())
		return
	}

	// Persist sent message into SQLite
	status := s.bridge.GetStatus()
	storedMsg := Message{
		ID:        resp.ID,
		ChatJID:   targetJID.ToNonAD().String(),
		SenderJID: status.JID,
		Text:      text,
		Timestamp: resp.Timestamp.Unix(),
		IsFromMe:  true,
	}
	_ = s.bridge.storage.SaveMessage(r.Context(), storedMsg)

	writeJSON(w, http.StatusOK, map[string]any{
		"success":    true,
		"message_id": resp.ID,
		"timestamp":  resp.Timestamp.Unix(),
		"chat_jid":   targetJID.ToNonAD().String(),
	})
}

func (s *HTTPServer) handleSendFile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RecipientJID string `json:"recipient_jid"`
		FilePath     string `json:"file_path"`
		Caption      string `json:"caption,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request JSON: "+err.Error())
		return
	}

	if req.RecipientJID == "" || req.FilePath == "" {
		writeError(w, http.StatusBadRequest, "both 'recipient_jid' and 'file_path' are required")
		return
	}

	targetJID, err := parseJID(req.RecipientJID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid recipient JID: "+err.Error())
		return
	}

	fileData, err := os.ReadFile(req.FilePath)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("cannot read file %s: %v", req.FilePath, err))
		return
	}

	if s.bridge.client == nil || !s.bridge.client.IsConnected() {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp client is not connected")
		return
	}

	ext := strings.ToLower(filepath.Ext(req.FilePath))
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	fileName := filepath.Base(req.FilePath)
	fileLen := uint64(len(fileData))

	var msg *waE2E.Message
	var mediaType string

	if strings.HasPrefix(mimeType, "image/") {
		mediaType = "image"
		uploaded, upErr := s.bridge.client.Upload(r.Context(), fileData, whatsmeow.MediaImage)
		if upErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to upload image: "+upErr.Error())
			return
		}
		caption := req.Caption
		msg = &waE2E.Message{
			ImageMessage: &waE2E.ImageMessage{
				URL:           &uploaded.URL,
				DirectPath:    &uploaded.DirectPath,
				MediaKey:      uploaded.MediaKey,
				Mimetype:      &mimeType,
				FileEncSHA256: uploaded.FileEncSHA256,
				FileSHA256:    uploaded.FileSHA256,
				FileLength:    &fileLen,
				Caption:       &caption,
			},
		}
	} else {
		mediaType = "document"
		uploaded, upErr := s.bridge.client.Upload(r.Context(), fileData, whatsmeow.MediaDocument)
		if upErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to upload document: "+upErr.Error())
			return
		}
		caption := req.Caption
		msg = &waE2E.Message{
			DocumentMessage: &waE2E.DocumentMessage{
				URL:           &uploaded.URL,
				DirectPath:    &uploaded.DirectPath,
				MediaKey:      uploaded.MediaKey,
				Mimetype:      &mimeType,
				FileName:      &fileName,
				FileEncSHA256: uploaded.FileEncSHA256,
				FileSHA256:    uploaded.FileSHA256,
				FileLength:    &fileLen,
				Caption:       &caption,
			},
		}
	}

	resp, err := s.bridge.client.SendMessage(r.Context(), targetJID, msg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to send file message: "+err.Error())
		return
	}

	status := s.bridge.GetStatus()
	storedMsg := Message{
		ID:            resp.ID,
		ChatJID:       targetJID.ToNonAD().String(),
		SenderJID:     status.JID,
		Text:          req.Caption,
		Timestamp:     resp.Timestamp.Unix(),
		IsFromMe:      true,
		HasMedia:      true,
		MediaType:     mediaType,
		MediaFilename: fileName,
		MediaMimeType: mimeType,
		MediaPath:     req.FilePath,
		MediaSize:     int64(fileLen),
	}
	_ = s.bridge.storage.SaveMessage(r.Context(), storedMsg)

	writeJSON(w, http.StatusOK, map[string]any{
		"success":    true,
		"message_id": resp.ID,
		"file_name":  fileName,
		"mime_type":  mimeType,
		"timestamp":  resp.Timestamp.Unix(),
	})
}

func (s *HTTPServer) handleSendAudio(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RecipientJID string `json:"recipient_jid"`
		FilePath     string `json:"file_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request JSON: "+err.Error())
		return
	}

	if req.RecipientJID == "" || req.FilePath == "" {
		writeError(w, http.StatusBadRequest, "both 'recipient_jid' and 'file_path' are required")
		return
	}

	targetJID, err := parseJID(req.RecipientJID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid recipient JID: "+err.Error())
		return
	}

	fileData, err := os.ReadFile(req.FilePath)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("cannot read audio file %s: %v", req.FilePath, err))
		return
	}

	if s.bridge.client == nil || !s.bridge.client.IsConnected() {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp client is not connected")
		return
	}

	uploaded, err := s.bridge.client.Upload(r.Context(), fileData, whatsmeow.MediaAudio)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to upload audio: "+err.Error())
		return
	}

	isPTT := true
	mimeType := "audio/ogg; codecs=opus"
	fileLen := uint64(len(fileData))

	msg := &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      &mimeType,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    &fileLen,
			PTT:           &isPTT,
		},
	}

	resp, err := s.bridge.client.SendMessage(r.Context(), targetJID, msg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to send audio message: "+err.Error())
		return
	}

	status := s.bridge.GetStatus()
	storedMsg := Message{
		ID:            resp.ID,
		ChatJID:       targetJID.ToNonAD().String(),
		SenderJID:     status.JID,
		Timestamp:     resp.Timestamp.Unix(),
		IsFromMe:      true,
		HasMedia:      true,
		MediaType:     "audio",
		MediaFilename: filepath.Base(req.FilePath),
		MediaMimeType: mimeType,
		MediaPath:     req.FilePath,
		MediaSize:     int64(fileLen),
	}
	_ = s.bridge.storage.SaveMessage(r.Context(), storedMsg)

	writeJSON(w, http.StatusOK, map[string]any{
		"success":    true,
		"message_id": resp.ID,
		"timestamp":  resp.Timestamp.Unix(),
	})
}

func (s *HTTPServer) handleDownloadMedia(w http.ResponseWriter, r *http.Request) {
	msgID := r.URL.Query().Get("message_id")
	chatJID := r.URL.Query().Get("chat_jid")

	if msgID == "" {
		writeError(w, http.StatusBadRequest, "missing 'message_id' query parameter")
		return
	}

	msg, err := s.bridge.storage.GetMessage(r.Context(), msgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if msg == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("message %s not found in local database", msgID))
		return
	}

	if !msg.HasMedia {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("message %s does not contain media attachments", msgID))
		return
	}

	// Check if already downloaded and file exists
	if msg.MediaPath != "" {
		if info, statErr := os.Stat(msg.MediaPath); statErr == nil && info.Size() > 0 {
			writeJSON(w, http.StatusOK, map[string]any{
				"file_path": msg.MediaPath,
				"filename":  msg.MediaFilename,
				"mime_type": msg.MediaMimeType,
				"size":      info.Size(),
				"cached":    true,
			})
			return
		}
	}

	// Need to download via whatsmeow client
	if s.bridge.client == nil || !s.bridge.client.IsConnected() {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp client is not connected to download media")
		return
	}

	if msg.RawData == "" {
		writeError(w, http.StatusBadRequest, "message does not contain raw media metadata needed for download")
		return
	}

	rawBytes, err := base64.StdEncoding.DecodeString(msg.RawData)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to decode raw message data: "+err.Error())
		return
	}

	var msgProto waE2E.Message
	if err := proto.Unmarshal(rawBytes, &msgProto); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to unmarshal message protobuf: "+err.Error())
		return
	}

	var mediaData []byte
	if msgProto.DocumentMessage != nil {
		mediaData, err = s.bridge.client.Download(r.Context(), msgProto.DocumentMessage)
	} else if msgProto.ImageMessage != nil {
		mediaData, err = s.bridge.client.Download(r.Context(), msgProto.ImageMessage)
	} else if msgProto.AudioMessage != nil {
		mediaData, err = s.bridge.client.Download(r.Context(), msgProto.AudioMessage)
	} else if msgProto.VideoMessage != nil {
		mediaData, err = s.bridge.client.Download(r.Context(), msgProto.VideoMessage)
	} else if msgProto.StickerMessage != nil {
		mediaData, err = s.bridge.client.Download(r.Context(), msgProto.StickerMessage)
	} else {
		writeError(w, http.StatusBadRequest, "message does not contain downloadable media")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to download media from WhatsApp: "+err.Error())
		return
	}

	// Save to local media cache directory
	cacheDir := "media_cache"
	_ = os.MkdirAll(cacheDir, 0755)

	fileName := msg.MediaFilename
	if fileName == "" {
		fileName = fmt.Sprintf("%s.bin", msgID)
	}
	safeFileName := fmt.Sprintf("%s_%s", msgID, filepath.Base(fileName))
	localPath := filepath.Join(cacheDir, safeFileName)

	if err := os.WriteFile(localPath, mediaData, 0644); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save downloaded media to disk: "+err.Error())
		return
	}

	absPath, _ := filepath.Abs(localPath)
	_ = s.bridge.storage.UpdateMediaPath(r.Context(), msgID, absPath)

	writeJSON(w, http.StatusOK, map[string]any{
		"file_path": absPath,
		"filename":  fileName,
		"mime_type": msg.MediaMimeType,
		"size":      len(mediaData),
		"chat_jid":  chatJID,
		"cached":    false,
	})
}

func (s *HTTPServer) handleGetGroupPDFs(w http.ResponseWriter, r *http.Request) {
	chatJID := r.URL.Query().Get("chat_jid")
	if chatJID == "" {
		writeError(w, http.StatusBadRequest, "missing 'chat_jid' query parameter")
		return
	}

	var since, until *int64
	if s := r.URL.Query().Get("since"); s != "" {
		if parsed, err := strconv.ParseInt(s, 10, 64); err == nil {
			since = &parsed
		}
	}
	if u := r.URL.Query().Get("until"); u != "" {
		if parsed, err := strconv.ParseInt(u, 10, 64); err == nil {
			until = &parsed
		}
	}

	pdfs, err := s.bridge.storage.GetGroupPDFs(r.Context(), chatJID, since, until)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if pdfs == nil {
		pdfs = []Message{}
	}

	writeJSON(w, http.StatusOK, pdfs)
}

func (s *HTTPServer) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string   `json:"name"`
		Participants []string `json:"participants"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request JSON: "+err.Error())
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "group 'name' cannot be empty")
		return
	}

	if s.bridge.client == nil || !s.bridge.client.IsConnected() {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp client is not connected")
		return
	}

	var participantJIDs []types.JID
	for _, p := range req.Participants {
		j, err := parseJID(p)
		if err == nil {
			participantJIDs = append(participantJIDs, j)
		}
	}

	groupInfo, err := s.bridge.client.CreateGroup(r.Context(), whatsmeow.ReqCreateGroup{
		Name:         req.Name,
		Participants: participantJIDs,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create group: "+err.Error())
		return
	}

	groupJID := groupInfo.JID.ToNonAD().String()
	_ = s.bridge.storage.SaveChat(r.Context(), Chat{
		JID:       groupJID,
		Name:      req.Name,
		IsGroup:   true,
		UpdatedAt: time.Now(),
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"success":   true,
		"group_jid": groupJID,
		"name":      req.Name,
	})
}

func (s *HTTPServer) handleAddParticipant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GroupJID    string `json:"group_jid"`
		Participant string `json:"participant"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request JSON: "+err.Error())
		return
	}

	if req.GroupJID == "" || req.Participant == "" {
		writeError(w, http.StatusBadRequest, "both 'group_jid' and 'participant' are required")
		return
	}

	if err := s.verifyAdmin(r.Context(), req.GroupJID); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	groupJID, err := parseJID(req.GroupJID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid group JID: "+err.Error())
		return
	}

	partJID, err := parseJID(req.Participant)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid participant JID: "+err.Error())
		return
	}

	_, err = s.bridge.client.UpdateGroupParticipants(r.Context(), groupJID, []types.JID{partJID}, whatsmeow.ParticipantChangeAdd)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add participant: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":     true,
		"group_jid":   req.GroupJID,
		"participant": partJID.String(),
	})
}

func (s *HTTPServer) handleRemoveParticipant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GroupJID    string `json:"group_jid"`
		Participant string `json:"participant"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request JSON: "+err.Error())
		return
	}

	if req.GroupJID == "" || req.Participant == "" {
		writeError(w, http.StatusBadRequest, "both 'group_jid' and 'participant' are required")
		return
	}

	if err := s.verifyAdmin(r.Context(), req.GroupJID); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	groupJID, err := parseJID(req.GroupJID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid group JID: "+err.Error())
		return
	}

	partJID, err := parseJID(req.Participant)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid participant JID: "+err.Error())
		return
	}

	_, err = s.bridge.client.UpdateGroupParticipants(r.Context(), groupJID, []types.JID{partJID}, whatsmeow.ParticipantChangeRemove)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove participant: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":     true,
		"group_jid":   req.GroupJID,
		"participant": partJID.String(),
	})
}

func (s *HTTPServer) handleGetGroupInviteLink(w http.ResponseWriter, r *http.Request) {
	groupJIDStr := r.URL.Query().Get("group_jid")
	if groupJIDStr == "" {
		writeError(w, http.StatusBadRequest, "missing 'group_jid' query parameter")
		return
	}

	if err := s.verifyAdmin(r.Context(), groupJIDStr); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	groupJID, err := parseJID(groupJIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid group JID: "+err.Error())
		return
	}

	link, err := s.bridge.client.GetGroupInviteLink(r.Context(), groupJID, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get group invite link: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"group_jid":   groupJIDStr,
		"invite_link": link,
	})
}

func (s *HTTPServer) handleSetGroupAnnounceOnly(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GroupJID string `json:"group_jid"`
		Enabled  bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request JSON: "+err.Error())
		return
	}

	if req.GroupJID == "" {
		writeError(w, http.StatusBadRequest, "missing 'group_jid'")
		return
	}

	if !strings.HasSuffix(req.GroupJID, "@g.us") {
		writeError(w, http.StatusBadRequest, "announce mode can only be configured for groups (@g.us)")
		return
	}

	if err := s.verifyAdmin(r.Context(), req.GroupJID); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	groupJID, err := parseJID(req.GroupJID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid group JID: "+err.Error())
		return
	}

	if err := s.bridge.client.SetGroupAnnounce(r.Context(), groupJID, req.Enabled); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to set announce mode: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":   true,
		"group_jid": req.GroupJID,
		"enabled":   req.Enabled,
	})
}

func (s *HTTPServer) handleSendReaction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MessageID string `json:"message_id"`
		ChatJID   string `json:"chat_jid"`
		Emoji     string `json:"emoji"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request JSON: "+err.Error())
		return
	}

	if req.MessageID == "" || req.ChatJID == "" {
		writeError(w, http.StatusBadRequest, "both 'message_id' and 'chat_jid' are required")
		return
	}

	chatJID, err := parseJID(req.ChatJID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid chat JID: "+err.Error())
		return
	}

	if s.bridge.client == nil || !s.bridge.client.IsConnected() {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp client is not connected")
		return
	}

	senderJID := chatJID
	msgRecord, _ := s.bridge.storage.GetMessage(r.Context(), req.MessageID)
	if msgRecord != nil && msgRecord.SenderJID != "" {
		if sJID, sErr := parseJID(msgRecord.SenderJID); sErr == nil {
			senderJID = sJID
		}
	}

	reactionMsg := s.bridge.client.BuildReaction(chatJID, senderJID, types.MessageID(req.MessageID), req.Emoji)
	_, err = s.bridge.client.SendMessage(r.Context(), chatJID, reactionMsg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to send reaction: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":    true,
		"message_id": req.MessageID,
		"emoji":      req.Emoji,
	})
}

func (s *HTTPServer) handleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MessageID string `json:"message_id"`
		ChatJID   string `json:"chat_jid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request JSON: "+err.Error())
		return
	}

	if req.MessageID == "" {
		writeError(w, http.StatusBadRequest, "missing 'message_id'")
		return
	}

	targetChatJID := req.ChatJID
	if targetChatJID == "" {
		// Look up chat from storage
		msg, _ := s.bridge.storage.GetMessage(r.Context(), req.MessageID)
		if msg != nil {
			targetChatJID = msg.ChatJID
		}
	}

	chatJID, err := parseJID(targetChatJID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "cannot determine valid chat JID for message: "+err.Error())
		return
	}

	if s.bridge.client == nil || !s.bridge.client.IsConnected() {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp client is not connected")
		return
	}

	senderJID := chatJID
	if s.bridge.client.Store != nil && s.bridge.client.Store.ID != nil {
		senderJID = *s.bridge.client.Store.ID
	}
	revokeMsg := s.bridge.client.BuildRevoke(chatJID, senderJID, types.MessageID(req.MessageID))
	_, err = s.bridge.client.SendMessage(r.Context(), chatJID, revokeMsg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to revoke message: "+err.Error())
		return
	}

	_ = s.bridge.storage.MarkMessageRevoked(r.Context(), req.MessageID)

	writeJSON(w, http.StatusOK, map[string]any{
		"success":    true,
		"message_id": req.MessageID,
		"revoked":    true,
	})
}

func (s *HTTPServer) handleMarkAsRead(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MessageID string `json:"message_id"`
		ChatJID   string `json:"chat_jid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request JSON: "+err.Error())
		return
	}

	if req.MessageID == "" {
		writeError(w, http.StatusBadRequest, "missing 'message_id'")
		return
	}

	targetChatJID := req.ChatJID
	senderJIDStr := req.ChatJID
	msg, _ := s.bridge.storage.GetMessage(r.Context(), req.MessageID)
	if msg != nil {
		if targetChatJID == "" {
			targetChatJID = msg.ChatJID
		}
		senderJIDStr = msg.SenderJID
	}

	chatJID, err := parseJID(targetChatJID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid chat JID: "+err.Error())
		return
	}

	senderJID, _ := parseJID(senderJIDStr)

	if s.bridge.client == nil || !s.bridge.client.IsConnected() {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp client is not connected")
		return
	}

	err = s.bridge.client.MarkRead(r.Context(), []types.MessageID{types.MessageID(req.MessageID)}, time.Now(), chatJID, senderJID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to mark message as read: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":    true,
		"message_id": req.MessageID,
	})
}

func (s *HTTPServer) handleCreateChannel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request JSON: "+err.Error())
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "channel 'name' is required")
		return
	}

	if s.bridge.client == nil || !s.bridge.client.IsConnected() {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp client is not connected")
		return
	}

	meta, err := s.bridge.client.CreateNewsletter(r.Context(), whatsmeow.CreateNewsletterParams{
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create newsletter channel: "+err.Error())
		return
	}

	channelJID := meta.ID.ToNonAD().String()
	_ = s.bridge.storage.SaveChat(r.Context(), Chat{
		JID:       channelJID,
		Name:      req.Name,
		IsGroup:   false,
		IsChannel: true,
		UpdatedAt: time.Now(),
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"success":     true,
		"channel_jid": channelJID,
		"name":        req.Name,
		"description": req.Description,
	})
}
