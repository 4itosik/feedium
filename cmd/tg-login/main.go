// Command tg-login performs the one-time, interactive Telegram userbot login
// that the feedium server deliberately never does on its own (AG-02): first-time
// phone-code authorization is a manual human step.
//
// It reuses the same config and the same on-disk session storage as the running
// app (data.NewSessionStorage — AES-256-GCM-encrypted when TG_SESSION_KEY is set),
// so the authorized session it writes to collectors.telegram.session_path is read
// back verbatim by the userbot collector on the next start.
//
// Run it once, in the same container/volume the app uses, with a TTY attached:
//
//	docker compose run --rm -it app tg-login -conf /etc/feedium/config.yaml
//
// It prompts for the login code (and the 2FA password, if the account has one),
// then exits. The phone number and api_id/api_hash come from the config.
//
// Read-only inspect modes (never log in; stop the app first so two clients do
// not write the same session file):
//
//	tg-login -conf … -dialogs               # chats the userbot is a member of
//	tg-login -conf … -resolve user1,user2   # real tg_id + membership per username
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"golang.org/x/term"

	"github.com/4itosik/feedium/internal/conf"
	"github.com/4itosik/feedium/internal/data"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("tg-login failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	flagconf := flag.String("conf", "/etc/feedium/config.yaml", "config path (file or directory)")
	flagDialogs := flag.Bool("dialogs", false, "read-only: list chats the userbot is a member of (TSV) and exit")
	flagResolve := flag.String("resolve", "",
		"read-only: comma-separated usernames to resolve to tg_id + membership (TSV)")
	flag.Parse()

	tg, err := loadTelegramConf(*flagconf)
	if err != nil {
		return err
	}

	storage, err := data.NewSessionStorage(tg.GetSessionPath())
	if err != nil {
		return fmt.Errorf("session storage: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := telegram.NewClient(int(tg.GetApiId()), tg.GetApiHash(), telegram.Options{
		SessionStorage: storage,
	})

	return client.Run(ctx, func(ctx context.Context) error {
		switch {
		case *flagDialogs:
			return listDialogs(ctx, client, os.Stdout)
		case *flagResolve != "":
			return resolveUsernames(ctx, client, strings.Split(*flagResolve, ","), os.Stdout)
		default:
			return login(ctx, client, tg, logger)
		}
	})
}

// login authorizes the userbot if needed and reports the resulting identity.
// An already-authorized session is a no-op (idempotent re-runs are safe).
func login(ctx context.Context, client *telegram.Client, tg *conf.TelegramCollector, logger *slog.Logger) error {
	status, err := client.Auth().Status(ctx)
	if err != nil {
		return fmt.Errorf("auth status: %w", err)
	}
	if !status.Authorized {
		flow := auth.NewFlow(termAuth{phone: tg.GetPhone()}, auth.SendCodeOptions{})
		if ferr := client.Auth().IfNecessary(ctx, flow); ferr != nil {
			return fmt.Errorf("login flow: %w", ferr)
		}
	}

	self, err := client.Self(ctx)
	if err != nil {
		return fmt.Errorf("self: %w", err)
	}
	if status.Authorized {
		logger.InfoContext(ctx, "session already authorized — nothing to do",
			"user_id", self.ID, "username", self.Username, "session_path", tg.GetSessionPath())
	} else {
		logger.InfoContext(ctx, "login successful — session saved",
			"user_id", self.ID, "username", self.Username, "session_path", tg.GetSessionPath())
	}
	return nil
}

// loadTelegramConf loads the bootstrap config and returns the Telegram collector
// section, rejecting the idle/placeholder state (api_id=0, empty hash/phone/path)
// up front so the failure is a clear config error rather than an opaque API error.
func loadTelegramConf(path string) (*conf.TelegramCollector, error) {
	c := config.New(config.WithSource(file.NewSource(path)))
	if err := c.Load(); err != nil {
		return nil, fmt.Errorf("load config %q: %w", path, err)
	}
	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		return nil, fmt.Errorf("scan config: %w", err)
	}

	tg := bc.GetCollectors().GetTelegram()
	if tg == nil {
		return nil, errors.New("collectors.telegram is not configured")
	}
	switch {
	case tg.GetApiId() == 0:
		return nil, errors.New("collectors.telegram.api_id is 0 — set it from my.telegram.org")
	case strings.TrimSpace(tg.GetApiHash()) == "":
		return nil, errors.New("collectors.telegram.api_hash is empty — set it from my.telegram.org")
	case strings.TrimSpace(tg.GetPhone()) == "":
		return nil, errors.New("collectors.telegram.phone is empty — set the userbot's phone number")
	case strings.TrimSpace(tg.GetSessionPath()) == "":
		return nil, errors.New("collectors.telegram.session_path is empty")
	}
	return tg, nil
}

// termAuth is the interactive UserAuthenticator: phone comes from config, the
// code and (only if the account has 2FA) the password are read from the terminal.
// Sign-up is explicitly refused — this provisions an existing account, never a new one.
type termAuth struct {
	phone string
}

func (a termAuth) Phone(context.Context) (string, error) { return a.phone, nil }

func (termAuth) Code(_ context.Context, _ *tg.AuthSentCode) (string, error) {
	fmt.Fprint(os.Stderr, "Enter the login code Telegram just sent: ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read code: %w", err)
	}
	return strings.TrimSpace(line), nil
}

func (termAuth) Password(context.Context) (string, error) {
	fmt.Fprint(os.Stderr, "Enter the 2FA password: ")
	pw, err := term.ReadPassword(int(os.Stdin.Fd())) //nolint:gosec // os.Stdin.Fd() always fits int
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read 2FA password: %w", err)
	}
	return strings.TrimSpace(string(pw)), nil
}

func (termAuth) SignUp(context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, errors.New("sign-up is not supported: the userbot account must already exist")
}

func (termAuth) AcceptTermsOfService(_ context.Context, tos tg.HelpTermsOfService) error {
	return &auth.SignUpRequired{TermsOfService: tos}
}
