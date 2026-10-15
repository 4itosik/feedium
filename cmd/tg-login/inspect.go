package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/query"
	"github.com/gotd/td/tg"
)

// dialogsBatchSize is the messages.getDialogs page size (Telegram's maximum).
const dialogsBatchSize = 100

// errNotAuthorized is returned by the read-only inspect modes: they never log in.
var errNotAuthorized = errors.New("session is not authorized — run tg-login without inspect flags first")

// listDialogs prints every chat the userbot is a member of as TSV
// (kind, tg_id, username, title). The push collector only receives messages
// from these chats, so a configured source whose tg_id is absent never collects.
func listDialogs(ctx context.Context, client *telegram.Client, out io.Writer) error {
	if err := requireAuthorized(ctx, client); err != nil {
		return err
	}
	iter := query.GetDialogs(client.API()).BatchSize(dialogsBatchSize).Iter()
	for iter.Next(ctx) {
		el := iter.Value()
		switch p := el.Peer.(type) {
		case *tg.InputPeerChannel:
			if ch, ok := el.Entities.Channel(p.ChannelID); ok {
				fmt.Fprintf(out, "%s\t%d\t%s\t%s\n", channelKind(ch), ch.ID, ch.Username, ch.Title)
			}
		case *tg.InputPeerChat:
			if chat, ok := el.Entities.Chat(p.ChatID); ok {
				fmt.Fprintf(out, "chat\t%d\t\t%s\n", chat.ID, chat.Title)
			}
		}
	}
	return iter.Err()
}

// resolveUsernames prints the real tg_id of each public username as TSV
// (username, kind, tg_id, member, title), where member=false means the userbot
// is not subscribed. Unresolvable usernames are reported, not fatal.
func resolveUsernames(ctx context.Context, client *telegram.Client, usernames []string, out io.Writer) error {
	if err := requireAuthorized(ctx, client); err != nil {
		return err
	}
	for _, u := range usernames {
		u = strings.TrimPrefix(strings.TrimSpace(u), "@")
		if u == "" {
			continue
		}
		res, err := client.API().ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: u})
		if err != nil {
			fmt.Fprintf(out, "%s\terror\t\t\t%v\n", u, err)
			continue
		}
		found := false
		for _, c := range res.Chats {
			if ch, ok := c.(*tg.Channel); ok {
				fmt.Fprintf(out, "%s\t%s\t%d\t%t\t%s\n", u, channelKind(ch), ch.ID, !ch.Left, ch.Title)
				found = true
			}
		}
		if !found {
			fmt.Fprintf(out, "%s\tnot-a-channel\t\t\t\n", u)
		}
	}
	return nil
}

func requireAuthorized(ctx context.Context, client *telegram.Client) error {
	status, err := client.Auth().Status(ctx)
	if err != nil {
		return fmt.Errorf("auth status: %w", err)
	}
	if !status.Authorized {
		return errNotAuthorized
	}
	return nil
}

func channelKind(ch *tg.Channel) string {
	if ch.Megagroup {
		return "supergroup"
	}
	return "channel"
}
