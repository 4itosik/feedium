//nolint:testpackage // test needs access to unexported functions authorFrom/chatTitleFrom
package data

import (
	"testing"

	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthorFrom(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  *tg.Message
		ents tg.Entities
		want *string
	}{
		{
			name: "username preferred",
			msg:  &tg.Message{FromID: &tg.PeerUser{UserID: 7}},
			ents: tg.Entities{Users: map[int64]*tg.User{7: {Username: "alice", FirstName: "Alice", LastName: "Smith"}}},
			want: new("alice"),
		},
		{
			name: "falls back to first+last name",
			msg:  &tg.Message{FromID: &tg.PeerUser{UserID: 7}},
			ents: tg.Entities{Users: map[int64]*tg.User{7: {FirstName: "Alice", LastName: "Smith"}}},
			want: new("Alice Smith"),
		},
		{
			name: "first name only",
			msg:  &tg.Message{FromID: &tg.PeerUser{UserID: 7}},
			ents: tg.Entities{Users: map[int64]*tg.User{7: {FirstName: "Alice"}}},
			want: new("Alice"),
		},
		{
			name: "user with no name resolves to nil",
			msg:  &tg.Message{FromID: &tg.PeerUser{UserID: 7}},
			ents: tg.Entities{Users: map[int64]*tg.User{7: {}}},
			want: nil,
		},
		{
			name: "sender not in entities resolves to nil",
			msg:  &tg.Message{FromID: &tg.PeerUser{UserID: 7}},
			ents: tg.Entities{Users: map[int64]*tg.User{}},
			want: nil,
		},
		{
			name: "non-user sender resolves to nil",
			msg:  &tg.Message{FromID: &tg.PeerChannel{ChannelID: 7}},
			ents: tg.Entities{},
			want: nil,
		},
		{
			name: "nil FromID resolves to nil",
			msg:  &tg.Message{},
			ents: tg.Entities{},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := authorFrom(tt.msg, tt.ents)
			if tt.want == nil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, *tt.want, *got)
		})
	}
}

func TestChatTitleFrom(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		peerID int64
		ents   tg.Entities
		want   string
	}{
		{
			name:   "channel title",
			peerID: 100,
			ents:   tg.Entities{Channels: map[int64]*tg.Channel{100: {Title: "My Channel"}}},
			want:   "My Channel",
		},
		{
			name:   "chat title",
			peerID: 200,
			ents:   tg.Entities{Chats: map[int64]*tg.Chat{200: {Title: "My Group"}}},
			want:   "My Group",
		},
		{
			name:   "channel preferred over chat for same id",
			peerID: 300,
			ents: tg.Entities{
				Channels: map[int64]*tg.Channel{300: {Title: "Chan"}},
				Chats:    map[int64]*tg.Chat{300: {Title: "Chat"}},
			},
			want: "Chan",
		},
		{
			name:   "absent peer resolves to empty",
			peerID: 999,
			ents:   tg.Entities{},
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, chatTitleFrom(tt.peerID, tt.ents))
		})
	}
}
