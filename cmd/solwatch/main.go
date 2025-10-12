package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/0xsamyy/solwatch-v2/internal/analyzer" // V2 Import
	"github.com/0xsamyy/solwatch-v2/internal/config"
	"github.com/0xsamyy/solwatch-v2/internal/health"
	"github.com/0xsamyy/solwatch-v2/internal/store"
	"github.com/0xsamyy/solwatch-v2/internal/telegram"
	"github.com/0xsamyy/solwatch-v2/internal/tracker"
	tg "github.com/go-telegram/bot"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lmsgprefix)
	log.SetPrefix("solwatch ")

	cfg := config.MustLoad()
	log.Println(cfg.RedactedSummary())

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	st, err := store.NewBolt(cfg.DBPath)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer func() {
		if e := st.Close(); e != nil {
			log.Printf("store close: %v", e)
		}
	}()

	// V2 Change: Initialize the new Analyzer
	an := analyzer.New(cfg.HeliusAPIURL, cfg.SolanaRPCURL)

	tm := tracker.NewManager(cfg.HeliusWSS, cfg.Commitment)
	hlth := health.New(tm, st)

	bot, err := tg.New(cfg.TelegramBotToken)
	if err != nil {
		log.Fatalf("telegram init: %v", err)
	}

	// V2 Change: Pass the analyzer instance to the Telegram handler
	th := telegram.New(bot, tm, st, hlth, an, cfg.TelegramAdminChatID, cancel)

	if addrs, err := st.ListWallets(ctx); err != nil {
		log.Printf("store list: %v", err)
	} else {
		for _, a := range addrs {
			if err := tm.Track(ctx, a); err != nil {
				log.Printf("track %s: %v", a, err)
			}
		}
	}

	log.Println("started; awaiting Telegram commands")
	th.Run(ctx)
	log.Println("shutdown complete")
}
