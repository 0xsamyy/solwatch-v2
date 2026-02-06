package telegram

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/0xsamyy/solwatch-v2/internal/analyzer"
	"github.com/0xsamyy/solwatch-v2/internal/health"
	"github.com/0xsamyy/solwatch-v2/internal/tracker"
	tg "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type WalletStore interface {
	AddWallet(ctx context.Context, addr string) error
	RemoveWallet(ctx context.Context, addr string) error
	ListWallets(ctx context.Context) ([]string, error)
}

// Handler coordinates Telegram <-> tracker/store/health.
type Handler struct {
	bot      *tg.Bot
	adminID  int64
	tm       *tracker.Manager
	st       WalletStore
	hlth     *health.Health
	analyzer *analyzer.Analyzer
	killFn   func()
}

// New constructs the Telegram Handler and wires the notification callback.
func New(bot *tg.Bot, tm *tracker.Manager, st WalletStore, hlth *health.Health, an *analyzer.Analyzer, adminID int64, killFn func()) *Handler {
	h := &Handler{
		bot:      bot,
		adminID:  adminID,
		tm:       tm,
		st:       st,
		hlth:     hlth,
		analyzer: an,
		killFn:   killFn,
	}

	tracker.SignatureNotify = func(signature string, trackedAddr string) {
		log.Printf("[handler] analyzing signature %s for wallet %s", signature, trackedAddr)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		summary, err := h.analyzer.AnalyzeSignature(ctx, signature, trackedAddr)
		if err != nil {
			log.Printf("[analyzer] error for %s: %v", signature, err)
			return
		}

		if summary == "" {
			log.Printf("[analyzer] signature %s filtered, no notification sent.", signature)
			return
		}

		shortAddr := trackedAddr[:4] + "..." + trackedAddr[len(trackedAddr)-4:]
		finalMessage := fmt.Sprintf("🚨 <b>Activity on %s</b>\n\n%s", shortAddr, summary)
		h.sendHTML(ctx, h.adminID, finalMessage)
	}

	return h
}

// Run starts long-polling and handles updates until ctx is done.
func (h *Handler) Run(ctx context.Context) {
	h.bot.RegisterHandler(tg.HandlerTypeMessageText, "", tg.MatchTypePrefix, func(c context.Context, b *tg.Bot, u *models.Update) {
		if u.Message == nil || u.Message.Chat.ID != h.adminID {
			return
		}
		h.handleCommand(c, u.Message)
	})
	h.bot.Start(ctx)
}

func (h *Handler) handleCommand(ctx context.Context, m *models.Message) {
	raw := strings.TrimSpace(m.Text)
	lower := strings.ToLower(raw)
	if idx := strings.IndexRune(lower, '@'); idx != -1 {
		lower = lower[:idx]
		raw = raw[:idx]
	}
	switch {
	case lower == "/help":
		h.replyHelp(ctx, m.Chat.ID)

	case strings.HasPrefix(lower, "/test "):
		args := strings.Fields(raw[len("/test "):])
		if len(args) != 2 {
			h.sendHTML(ctx, m.Chat.ID, "usage: <code>/test &lt;signature&gt; &lt;wallet_address&gt;</code>")
			return
		}
		signature := args[0]
		walletAddr := args[1]

		h.sendHTML(ctx, m.Chat.ID, fmt.Sprintf("🔬 Analyzing signature <code>%s...</code> for wallet <code>%s...</code>", signature[:10], walletAddr[:4]))

		summary, err := h.analyzer.AnalyzeSignature(ctx, signature, walletAddr)
		if err != nil {
			errMsg := fmt.Sprintf("<b>Analysis Failed:</b>\n<code>%v</code>", err)
			h.sendHTML(ctx, m.Chat.ID, errMsg)
			return
		}

		if summary == "" {
			h.sendHTML(ctx, m.Chat.ID, "✅ <b>Analysis Complete:</b>\nTransaction was filtered (likely spam or dust).")
			return
		}

		shortAddr := walletAddr[:4] + "..." + walletAddr[len(walletAddr)-4:]
		finalMessage := fmt.Sprintf("🧪 <b>Test Result for %s</b>\n\n%s", shortAddr, summary)
		h.sendHTML(ctx, m.Chat.ID, finalMessage)

	case strings.HasPrefix(lower, "/track "):
		arg := strings.TrimSpace(raw[len("/track"):])
		if arg == "" {
			h.sendHTML(ctx, m.Chat.ID, "usage: <code>/track &lt;address&gt;</code>")
			return
		}
		if err := h.st.AddWallet(ctx, arg); err != nil {
			h.sendHTML(ctx, m.Chat.ID, fmt.Sprintf("track failed: <code>%v</code>", err))
			return
		}
		if err := h.tm.Track(ctx, arg); err != nil {
			h.sendHTML(ctx, m.Chat.ID, fmt.Sprintf("subscriber failed: <code>%v</code>", err))
			return
		}
		h.sendHTML(ctx, m.Chat.ID, "tracking <b>"+escapeHTML(arg)+"</b>")

	case strings.HasPrefix(lower, "/untrack "):
		arg := strings.TrimSpace(raw[len("/untrack"):])
		if arg == "" {
			h.sendHTML(ctx, m.Chat.ID, "usage: <code>/untrack &lt;address&gt;</code>")
			return
		}
		_ = h.tm.Untrack(ctx, arg)
		if err := h.st.RemoveWallet(ctx, arg); err != nil {
			h.sendHTML(ctx, m.Chat.ID, fmt.Sprintf("untrack failed: <code>%v</code>", err))
			return
		}
		h.sendHTML(ctx, m.Chat.ID, "untracked <b>"+escapeHTML(arg)+"</b>")

	case strings.HasPrefix(lower, "/trackmany "):
		args := strings.Fields(raw[len("/trackmany"):])
		if len(args) == 0 {
			h.sendHTML(ctx, m.Chat.ID, "usage: <code>/trackmany &lt;addr1&gt; &lt;addr2&gt; ...</code>")
			return
		}
		var added, failed int
		for _, addr := range args {
			if err := h.st.AddWallet(ctx, addr); err != nil {
				failed++
				continue
			}
			if err := h.tm.Track(ctx, addr); err != nil {
				_ = h.st.RemoveWallet(ctx, addr)
				failed++
				continue
			}
			added++
		}
		summary := fmt.Sprintf("trackmany done: added=%d failed=%d", added, failed)
		h.sendHTML(ctx, m.Chat.ID, summary)

	case strings.HasPrefix(lower, "/untrackmany "):
		args := strings.Fields(raw[len("/untrackmany"):])
		if len(args) == 0 {
			h.sendHTML(ctx, m.Chat.ID, "usage: <code>/untrackmany &lt;addr1&gt; &lt;addr2&gt; ...</code>")
			return
		}
		var removed, failed int
		for _, addr := range args {
			_ = h.tm.Untrack(ctx, addr)
			if err := h.st.RemoveWallet(ctx, addr); err != nil {
				failed++
				continue
			}
			removed++
		}
		summary := fmt.Sprintf("untrackmany done: removed=%d failed=%d", removed, failed)
		h.sendHTML(ctx, m.Chat.ID, summary)

	case lower == "/tracked":
		list := h.tm.List()
		if len(list) == 0 {
			h.sendHTML(ctx, m.Chat.ID, "<b>No wallets tracked.</b>")
			return
		}
		var b strings.Builder
		b.WriteString("📋 <b>Tracked Wallets:</b>\n")
		for _, a := range list {
			b.WriteString("- <code>")
			b.WriteString(escapeHTML(a))
			b.WriteString("</code>\n")
		}
		h.sendHTML(ctx, m.Chat.ID, b.String())

	case lower == "/health":
		rep := h.hlth.Snapshot(ctx)
		msg := fmt.Sprintf(
			"📊 <b>Health Report</b>\n"+
				"- Tracked (memory): <code>%d</code>\n"+
				"- Open subs: <code>%d</code>\n"+
				"- Dropped: <code>%d</code>\n"+
				"- Tracked (store): <code>%d</code>\n"+
				"- Time: <code>%s</code>",
			rep.Tracked, rep.Open, len(rep.Dropped), rep.TrackedPersisted, rep.GeneratedAt.Format(time.RFC3339),
		)
		h.sendHTML(ctx, m.Chat.ID, msg)

	case lower == "/kill":
		h.sendHTML(ctx, m.Chat.ID, "🛑 shutting down...")
		go func() {
			time.Sleep(200 * time.Millisecond)
			if h.killFn != nil {
				h.killFn()
			} else {
				log.Println("[telegram] killFn not set")
			}
		}()

	default:
		h.sendHTML(ctx, m.Chat.ID, "unknown command. try <code>/help</code>")
	}
}

func (h *Handler) replyHelp(ctx context.Context, chatID int64) {
	help := strings.TrimSpace(`
🛠 <b>solwatch v2</b>

<b>Commands:</b>
- <code>/track &lt;address&gt;</code> - Start tracking a wallet
- <code>/untrack &lt;address&gt;</code> - Stop tracking a wallet
- <code>/trackmany &lt;...&gt;</code> - Add multiple wallets
- <code>/untrackmany &lt;...&gt;</code> - Remove multiple wallets
- <code>/tracked</code> - List tracked wallets
- <code>/health</code> - Show service health
- <code>/kill</code> - Shutdown the service

<b>Debug:</b>
- <code>/test &lt;sig&gt; &lt;addr&gt;</code> - Test analysis of a signature for a given wallet
`)
	h.sendHTML(ctx, chatID, help)
}

func (h *Handler) sendHTML(ctx context.Context, chatID int64, html string) {
	disable := true
	_, err := h.bot.SendMessage(ctx, &tg.SendMessageParams{
		ChatID:    chatID,
		Text:      html,
		ParseMode: models.ParseModeHTML,
		LinkPreviewOptions: &models.LinkPreviewOptions{
			IsDisabled: &disable,
		},
	})
	if err != nil {
		log.Printf("[telegram] send error: %v", err)
	}
}

func escapeHTML(s string) string {
	replacer := strings.NewReplacer(
		`&`, "&amp;",
		`<`, "&lt;",
		`>`, "&gt;",
		`"`, "&quot;",
	)
	return replacer.Replace(s)
}
