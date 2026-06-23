package data

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/conf"
)

// Compile-time assertions.
var _ biz.TelegramSession = (*telegramSession)(nil)

// telegramSession is the gotd-backed userbot push connection (FT-009 DEC-08).
// gotd types live only here; it yields transport-agnostic biz.TelegramMessage.
// Connecting/authorizing touches a live account — gated by AG-02; never auto-login.
type telegramSession struct {
	cfg     *conf.TelegramCollector
	storage session.Storage
}

// NewTelegramSession constructs the gotd session adapter.
// When TG_SESSION_KEY is set the session file is encrypted with AES-256-GCM;
// otherwise a plain FileStorage is used (backward-compatible for local dev).
//
//nolint:revive // unexported return type is intentional for Wire injection
func NewTelegramSession(cfg *conf.Collectors) (*telegramSession, error) {
	tgCfg := cfg.GetTelegram()
	stor, err := sessionStorageFromEnv(tgCfg.GetSessionPath())
	if err != nil {
		return nil, err
	}
	return &telegramSession{cfg: tgCfg, storage: stor}, nil
}

// Run opens the userbot connection and streams new channel/group messages to
// handle until ctx is cancelled. It never performs first-time login: if the
// stored session is missing/unauthorized it returns biz.ErrTelegramNotAuthorized
// (AG-02 — phone-code login is a manual human step).
func (s *telegramSession) Run(ctx context.Context, handle func(context.Context, biz.TelegramMessage) error) error {
	dispatcher := tg.NewUpdateDispatcher()
	mgr := updates.New(updates.Config{Handler: dispatcher})

	emit := func(peerID int64, m *tg.Message, ents tg.Entities) error {
		if m.Out {
			return nil
		}
		return handle(ctx, biz.TelegramMessage{
			TgID: peerID,
			Item: biz.CollectedItem{
				ExternalID:  strconv.Itoa(m.ID),
				PublishedAt: time.Unix(int64(m.Date), 0).UTC(),
				Author:      authorFrom(m, ents),
				Text:        m.Message,
				Metadata:    map[string]string{"chat_title": chatTitleFrom(peerID, ents)},
			},
		})
	}

	dispatcher.OnNewChannelMessage(func(_ context.Context, e tg.Entities, u *tg.UpdateNewChannelMessage) error {
		m, ok := u.Message.(*tg.Message)
		if !ok {
			return nil
		}
		peer, ok := m.PeerID.(*tg.PeerChannel)
		if !ok {
			return nil
		}
		return emit(peer.ChannelID, m, e)
	})
	dispatcher.OnNewMessage(func(_ context.Context, e tg.Entities, u *tg.UpdateNewMessage) error {
		m, ok := u.Message.(*tg.Message)
		if !ok {
			return nil
		}
		peer, ok := m.PeerID.(*tg.PeerChat)
		if !ok {
			return nil
		}
		return emit(peer.ChatID, m, e)
	})

	client := telegram.NewClient(int(s.cfg.GetApiId()), s.cfg.GetApiHash(), telegram.Options{
		SessionStorage: s.storage,
		UpdateHandler:  mgr,
	})

	return client.Run(ctx, func(ctx context.Context) error {
		status, err := client.Auth().Status(ctx)
		if err != nil {
			return fmt.Errorf("telegram auth status: %w", err)
		}
		if !status.Authorized {
			// First-time login needs a phone code — manual, AG-02. Never auto-login.
			return biz.ErrTelegramNotAuthorized
		}
		self, err := client.Self(ctx)
		if err != nil {
			return fmt.Errorf("telegram self: %w", err)
		}
		// mgr.Run blocks until ctx is cancelled (then returns ctx.Err) or a fatal
		// error occurs; it owns the update loop for the connection's lifetime.
		return mgr.Run(ctx, client.API(), self.ID, updates.AuthOptions{})
	})
}

// authorFrom resolves a display name for the message sender from the update
// entities. It returns nil when the sender is not a user or cannot be resolved
// (defensive against nil maps and missing keys). Pure.
func authorFrom(m *tg.Message, e tg.Entities) *string {
	peer, ok := m.FromID.(*tg.PeerUser)
	if !ok {
		return nil
	}
	user, ok := e.Users[peer.UserID]
	if !ok || user == nil {
		return nil
	}
	if name := strings.TrimSpace(user.Username); name != "" {
		return &name
	}
	name := strings.TrimSpace(strings.TrimSpace(user.FirstName) + " " + strings.TrimSpace(user.LastName))
	if name == "" {
		return nil
	}
	return &name
}

// chatTitleFrom returns the channel or chat title for peerID from the update
// entities, or "" if absent. Defensive against nil maps and missing keys. Pure.
func chatTitleFrom(peerID int64, e tg.Entities) string {
	if ch, ok := e.Channels[peerID]; ok && ch != nil {
		return ch.Title
	}
	if chat, ok := e.Chats[peerID]; ok && chat != nil {
		return chat.Title
	}
	return ""
}
