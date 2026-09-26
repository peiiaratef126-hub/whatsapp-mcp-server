package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

// BridgeStatus represents the current state of the WhatsApp bridge.
type BridgeStatus struct {
	Connected    bool   `json:"connected"`
	LoggedIn     bool   `json:"logged_in"`
	JID          string `json:"jid,omitempty"`
	Phone        string `json:"phone,omitempty"`
	PushName     string `json:"push_name,omitempty"`
	NeedsReauth  bool   `json:"needs_reauth"`
	LastError    string `json:"last_error,omitempty"`
	LastSeenTime string `json:"last_seen_time,omitempty"`
}

// WhatsAppBridge manages the whatsmeow client connection, QR pairing, session persistence, and message ingestion.
type WhatsAppBridge struct {
	client      *whatsmeow.Client
	container   *sqlstore.Container
	deviceStore *store.Device
	storage     *Storage
	dbPath      string
	logger      waLog.Logger

	mu          sync.RWMutex
	isConnected atomic.Bool
	isLoggedIn  atomic.Bool
	needsReauth atomic.Bool
	lastError   string
	userJID     types.JID
	pushName    string

	eventHandlers []func(evt interface{})
}

// NewWhatsAppBridge creates a new bridge instance, initializes session and local SQLite storage.
func NewWhatsAppBridge(ctx context.Context, sessionDBPath, storageDBPath, logLevel string) (*WhatsAppBridge, error) {
	logger := waLog.Stdout("Bridge", logLevel, true)
	dbLog := waLog.Stdout("Database", "WARN", true)

	container, err := sqlstore.New(ctx, "sqlite3", fmt.Sprintf("file:%s?_foreign_keys=on", sessionDBPath), dbLog)
	if err != nil {
		return nil, fmt.Errorf("failed to open session store: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get device store: %w", err)
	}

	if deviceStore == nil {
		logger.Infof("No existing device session found. Creating a new device session...")
		deviceStore = container.NewDevice()
	} else {
		logger.Infof("Loaded existing device session: %s", deviceStore.ID.String())
	}

	storage, err := NewStorage(storageDBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize local storage: %w", err)
	}

	clientLog := waLog.Stdout("Client", logLevel, true)
	client := whatsmeow.NewClient(deviceStore, clientLog)

	bridge := &WhatsAppBridge{
		client:      client,
		container:   container,
		deviceStore: deviceStore,
		storage:     storage,
		dbPath:      sessionDBPath,
		logger:      logger,
	}

	client.AddEventHandler(bridge.handleEvent)

	return bridge, nil
}

// Storage returns the underlying SQLite storage instance.
func (b *WhatsAppBridge) Storage() *Storage {
	return b.storage
}

// Client returns the underlying whatsmeow client.
func (b *WhatsAppBridge) Client() *whatsmeow.Client {
	return b.client
}

// AddEventHandler registers an external handler for WhatsApp events.
func (b *WhatsAppBridge) AddEventHandler(handler func(evt interface{})) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.eventHandlers = append(b.eventHandlers, handler)
}

// Connect initiates connection to WhatsApp, handling QR code generation if needed.
func (b *WhatsAppBridge) Connect(ctx context.Context) error {
	if b.client.Store.ID == nil {
		// Device not paired yet: initiate QR flow
		b.logger.Infof("Starting QR code pairing flow...")
		qrChan, err := b.client.GetQRChannel(ctx)
		if err != nil {
			if errors.Is(err, whatsmeow.ErrQRStoreContainsID) {
				b.logger.Infof("Device already has an ID, connecting directly...")
			} else {
				return fmt.Errorf("failed to get QR channel: %w", err)
			}
		} else {
			err = b.client.Connect()
			if err != nil {
				return fmt.Errorf("failed to connect client for QR pairing: %w", err)
			}

			// Handle QR code channel in background
			go b.listenQRChannel(qrChan)
			return nil
		}
	}

	// Already paired: connect directly
	b.logger.Infof("Connecting with saved session...")
	err := b.client.Connect()
	if err != nil {
		b.setLastError(fmt.Sprintf("Failed to connect: %v", err))
		return fmt.Errorf("failed to connect: %w", err)
	}

	return nil
}

// listenQRChannel handles incoming QR codes and prints them to terminal.
func (b *WhatsAppBridge) listenQRChannel(qrChan <-chan whatsmeow.QRChannelItem) {
	for evt := range qrChan {
		switch evt.Event {
		case "code":
			b.logger.Infof("=== NEW WHATSAPP QR CODE ===")
			b.logger.Infof("Scan this QR code in WhatsApp -> Linked Devices -> Link a Device:")
			qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
			b.logger.Infof("============================")
		case "success":
			b.logger.Infof("Successfully paired with WhatsApp device!")
			b.isLoggedIn.Store(true)
			b.needsReauth.Store(false)
		case "timeout":
			b.logger.Warnf("QR code pairing timed out. Restart the bridge or call re-auth to try again.")
			b.setLastError("QR pairing timed out")
		default:
			b.logger.Infof("QR channel event: %s", evt.Event)
		}
	}
}

// handleEvent processes internal WhatsApp events like connection state, incoming messages, and session invalidation.
func (b *WhatsAppBridge) handleEvent(rawEvt interface{}) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch evt := rawEvt.(type) {
	case *events.Connected:
		b.isConnected.Store(true)
		b.isLoggedIn.Store(true)
		b.needsReauth.Store(false)
		b.mu.Lock()
		if b.client.Store.ID != nil {
			b.userJID = *b.client.Store.ID
			b.pushName = b.client.Store.PushName
		}
		b.mu.Unlock()
		b.logger.Infof("Connected to WhatsApp as %s (PushName: %s)", b.userJID.String(), b.pushName)

		// Sync initial joined groups in background
		go b.syncGroups(context.Background())

	case *events.Disconnected:
		b.isConnected.Store(false)
		b.logger.Warnf("Disconnected from WhatsApp. Automatic reconnect will be attempted...")

	case *events.LoggedOut:
		b.isConnected.Store(false)
		b.isLoggedIn.Store(false)
		b.needsReauth.Store(true)
		reason := fmt.Sprintf("%v", evt.Reason)
		b.setLastError(fmt.Sprintf("Session invalidated (reason: %s). Re-authentication required.", reason))
		b.logger.Warnf("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
		b.logger.Warnf("WhatsApp session was invalidated: %s", reason)
		b.logger.Warnf("Please re-authenticate by restarting the bridge to scan a new QR code.")
		b.logger.Warnf("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")

	case *events.ConnectFailure:
		b.isConnected.Store(false)
		b.setLastError(fmt.Sprintf("Connect failure: %v", evt.Reason))
		b.logger.Errorf("Connect failure: %v", evt.Reason)

	case *events.TemporaryBan:
		b.setLastError(fmt.Sprintf("Temporary ban: %v (code: %d)", evt.String(), evt.Code))
		b.logger.Errorf("WhatsApp temporary ban: %v", evt.String())

	case *events.StreamReplaced:
		b.logger.Warnf("WhatsApp Web stream replaced by another session.")

	case *events.Message:
		b.ingestMessage(ctx, evt)

	case *events.GroupInfo:
		b.ingestGroupInfo(ctx, evt)
	}

	// Dispatch to external registered handlers
	b.mu.RLock()
	handlers := make([]func(evt interface{}), len(b.eventHandlers))
	copy(handlers, b.eventHandlers)
	b.mu.RUnlock()

	for _, handler := range handlers {
		handler(rawEvt)
	}
}

// ingestMessage stores incoming and outgoing messages into SQLite.
func (b *WhatsAppBridge) ingestMessage(ctx context.Context, evt *events.Message) {
	if evt == nil || evt.Message == nil {
		return
	}

	chatJID := evt.Info.Chat.ToNonAD().String()
	senderJID := evt.Info.Sender.ToNonAD().String()

	text := ""
	mediaType := ""
	mediaFilename := ""
	mediaMimeType := ""
	var mediaSize int64
	hasMedia := false

	m := evt.Message
	if m.Conversation != nil {
		text = *m.Conversation
	} else if m.ExtendedTextMessage != nil && m.ExtendedTextMessage.Text != nil {
		text = *m.ExtendedTextMessage.Text
	} else if m.DocumentMessage != nil {
		hasMedia = true
		mediaType = "document"
		mediaMimeType = m.DocumentMessage.GetMimetype()
		mediaFilename = m.DocumentMessage.GetFileName()
		if mediaFilename == "" {
			mediaFilename = m.DocumentMessage.GetTitle()
		}
		text = m.DocumentMessage.GetCaption()
		mediaSize = int64(m.DocumentMessage.GetFileLength())
	} else if m.ImageMessage != nil {
		hasMedia = true
		mediaType = "image"
		mediaMimeType = m.ImageMessage.GetMimetype()
		text = m.ImageMessage.GetCaption()
		mediaSize = int64(m.ImageMessage.GetFileLength())
	} else if m.AudioMessage != nil {
		hasMedia = true
		mediaType = "audio"
		mediaMimeType = m.AudioMessage.GetMimetype()
		mediaSize = int64(m.AudioMessage.GetFileLength())
	} else if m.VideoMessage != nil {
		hasMedia = true
		mediaType = "video"
		mediaMimeType = m.VideoMessage.GetMimetype()
		text = m.VideoMessage.GetCaption()
		mediaSize = int64(m.VideoMessage.GetFileLength())
	}

	var rawDataStr string
	if rawBytes, err := proto.Marshal(evt.Message); err == nil {
		rawDataStr = base64.StdEncoding.EncodeToString(rawBytes)
	}

	msg := Message{
		ID:            evt.Info.ID,
		ChatJID:       chatJID,
		SenderJID:     senderJID,
		Text:          text,
		Timestamp:     evt.Info.Timestamp.Unix(),
		IsFromMe:      evt.Info.IsFromMe,
		HasMedia:      hasMedia,
		MediaType:     mediaType,
		MediaFilename: mediaFilename,
		MediaMimeType: mediaMimeType,
		MediaSize:     mediaSize,
		IsRevoked:     false,
		RawData:       rawDataStr,
	}

	if err := b.storage.SaveMessage(ctx, msg); err != nil {
		b.logger.Errorf("Failed to persist message %s: %v", evt.Info.ID, err)
	}

	// Update contact info if push name is available
	if evt.Info.PushName != "" && !evt.Info.IsFromMe {
		contact := Contact{
			JID:         senderJID,
			PhoneNumber: evt.Info.Sender.User,
			PushName:    evt.Info.PushName,
			UpdatedAt:   time.Now(),
		}
		_ = b.storage.SaveContact(ctx, contact)
	}
}

// ingestGroupInfo updates group metadata and participants in local storage.
func (b *WhatsAppBridge) ingestGroupInfo(ctx context.Context, evt *events.GroupInfo) {
	if evt == nil {
		return
	}
	groupJID := evt.JID.ToNonAD().String()

	if evt.Name != nil {
		_ = b.storage.SaveChat(ctx, Chat{
			JID:       groupJID,
			Name:      evt.Name.Name,
			IsGroup:   true,
			IsChannel: false,
			UpdatedAt: time.Now(),
		})
	}

	// Refresh group metadata and participants in background
	go func(jid types.JID) {
		bgCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		info, err := b.client.GetGroupInfo(bgCtx, jid)
		if err != nil {
			return
		}
		var parts []GroupParticipant
		for _, p := range info.Participants {
			parts = append(parts, GroupParticipant{
				GroupJID:       groupJID,
				ParticipantJID: p.JID.ToNonAD().String(),
				IsAdmin:        p.IsAdmin,
				IsSuperAdmin:   p.IsSuperAdmin,
			})
		}
		_ = b.storage.SaveGroupParticipants(bgCtx, groupJID, parts)
	}(evt.JID)
}

// syncGroups fetches joined groups from WhatsApp and populates storage.
func (b *WhatsAppBridge) syncGroups(ctx context.Context) {
	groups, err := b.client.GetJoinedGroups(ctx)
	if err != nil {
		b.logger.Warnf("Could not fetch joined groups: %v", err)
		return
	}

	for _, g := range groups {
		groupJID := g.JID.ToNonAD().String()
		_ = b.storage.SaveChat(ctx, Chat{
			JID:       groupJID,
			Name:      g.Name,
			IsGroup:   true,
			IsChannel: false,
			UpdatedAt: time.Now(),
		})

		var parts []GroupParticipant
		for _, p := range g.Participants {
			parts = append(parts, GroupParticipant{
				GroupJID:       groupJID,
				ParticipantJID: p.JID.ToNonAD().String(),
				IsAdmin:        p.IsAdmin,
				IsSuperAdmin:   p.IsSuperAdmin,
			})
		}
		_ = b.storage.SaveGroupParticipants(ctx, groupJID, parts)
	}
	b.logger.Infof("Synchronized %d joined groups to local database.", len(groups))
}

// Disconnect gracefully shuts down the WhatsApp connection and session container.
func (b *WhatsAppBridge) Disconnect() {
	b.logger.Infof("Disconnecting WhatsApp client...")
	if b.client != nil {
		b.client.Disconnect()
	}
	if b.container != nil {
		_ = b.container.Close()
	}
	if b.storage != nil {
		_ = b.storage.Close()
	}
	b.isConnected.Store(false)
}

func (b *WhatsAppBridge) setLastError(err string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastError = err
}

// GetStatus returns the current connection and authentication status.
func (b *WhatsAppBridge) GetStatus() BridgeStatus {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var phone string
	jidStr := ""
	isLoggedIn := false
	if b.client != nil && b.client.Store != nil && b.client.Store.ID != nil {
		jidStr = b.client.Store.ID.ToNonAD().String()
		phone = b.client.Store.ID.User
		isLoggedIn = b.isLoggedIn.Load()
	}

	return BridgeStatus{
		Connected:    b.isConnected.Load(),
		LoggedIn:     isLoggedIn,
		JID:          jidStr,
		Phone:        phone,
		PushName:     b.pushName,
		NeedsReauth:  b.needsReauth.Load(),
		LastError:    b.lastError,
		LastSeenTime: time.Now().UTC().Format(time.RFC3339),
	}
}
