package tracker

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0xsamyy/solwatch-v2/internal/util"
	"github.com/gorilla/websocket"
)

// V2 Change: The callback now includes the address of the wallet that was triggered.
// This is essential for the handler to know which wallet to associate the signature with.
var SignatureNotify func(signature string, trackedAddr string)

// logsNotification defines the structure of a `logsSubscribe` message from the RPC.
type logsNotification struct {
	Method string `json:"method"`
	Params struct {
		Result struct {
			Value struct {
				Signature string `json:"signature"`
				Err       any    `json:"err"`
			} `json:"value"`
		} `json:"result"`
	} `json:"params"`
}

// Subscriber maintains a single logsSubscribe connection for one wallet.
type Subscriber struct {
	wss        string
	addr       string
	commitment string

	open       atomic.Bool
	shouldOpen atomic.Bool

	dedupeCache map[string]time.Time
	dedupeMutex sync.Mutex

	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewSubscriber creates a new Subscriber. Call Run() to start it.
func NewSubscriber(wss, commitment, addr string) *Subscriber {
	s := &Subscriber{
		wss:         strings.TrimSpace(wss),
		addr:        strings.TrimSpace(addr),
		commitment:  strings.TrimSpace(commitment),
		stopCh:      make(chan struct{}),
		dedupeCache: make(map[string]time.Time),
	}
	s.shouldOpen.Store(true)
	return s
}

func (s *Subscriber) IsOpen() bool       { return s.open.Load() }
func (s *Subscriber) ShouldBeOpen() bool { return s.shouldOpen.Load() }

func (s *Subscriber) Stop() {
	s.stopOnce.Do(func() {
		s.shouldOpen.Store(false)
		close(s.stopCh)
	})
}

func (s *Subscriber) isDuplicate(signature string) bool {
	s.dedupeMutex.Lock()
	defer s.dedupeMutex.Unlock()

	if ts, found := s.dedupeCache[signature]; found {
		if time.Since(ts) < 30*time.Second {
			return true
		}
	}
	s.dedupeCache[signature] = time.Now()
	return false
}

func (s *Subscriber) cleanCache(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.dedupeMutex.Lock()
			for sig, ts := range s.dedupeCache {
				if time.Since(ts) > 1*time.Minute {
					delete(s.dedupeCache, sig)
				}
			}
			s.dedupeMutex.Unlock()
		}
	}
}

func (s *Subscriber) Run(ctx context.Context) {
	bo := util.NewBackoff(1*time.Second, 30*time.Second, 2.0, 0.2)
	go s.cleanCache(ctx)

	for {
		if !s.ShouldBeOpen() {
			return
		}

		conn, _, err := websocket.DefaultDialer.DialContext(ctx, s.wss, http.Header{})
		if err != nil {
			wait := bo.Next()
			log.Printf("[sub %s] dial error: %v; retrying in %s", s.prettyAddr(), err, wait)
			time.Sleep(wait)
			continue
		}

		s.open.Store(true)
		bo.Reset()

		connCtx, connCancel := context.WithCancel(ctx)
		go func() {
			select {
			case <-s.stopCh:
			case <-connCtx.Done():
			}
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "stopping"), time.Now().Add(2*time.Second))
			_ = conn.Close()
		}()

		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		conn.SetPongHandler(func(string) error {
			return conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		})

		subMsg := map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "logsSubscribe",
			"params": []any{
				map[string]any{"mentions": []string{s.addr}},
				map[string]any{"commitment": s.commitment},
			},
		}
		if err := conn.WriteJSON(subMsg); err != nil {
			log.Printf("[sub %s] subscribe error: %v", s.prettyAddr(), err)
			connCancel()
			continue
		}

		go func() {
			ticker := time.NewTicker(20 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-connCtx.Done():
					return
				case <-ticker.C:
					if err := conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(5*time.Second)); err != nil {
						return
					}
				}
			}
		}()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				log.Printf("[sub %s] read error: %v", s.prettyAddr(), err)
				break
			}

			var notif logsNotification
			if err := json.Unmarshal(msg, &notif); err != nil {
				continue
			}

			if notif.Method != "logsNotification" || notif.Params.Result.Value.Signature == "" || notif.Params.Result.Value.Err != nil {
				continue
			}

			signature := notif.Params.Result.Value.Signature
			if s.isDuplicate(signature) {
				continue
			}

			log.Printf("[sub %s] new signature detected: %s...", s.prettyAddr(), signature[:16])

			if SignatureNotify != nil {
				// V2 Change: Pass both the signature AND the address of this subscriber.
				SignatureNotify(signature, s.addr)
			}
		}

		s.open.Store(false)
		connCancel()
	}
}

func (s *Subscriber) prettyAddr() string {
	if len(s.addr) <= 8 {
		return s.addr
	}
	return s.addr[:4] + "..." + s.addr[len(s.addr)-4:]
}
