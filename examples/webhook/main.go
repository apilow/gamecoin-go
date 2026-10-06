// Command webhook receives the webhooks of GameCoin and verifies their signature. Run it with the signing secret of an
// endpoint (the dashboard shows it once), then point the endpoint at http://localhost:8080/gamecoin/webhook in the test
// environment and send a "webhook.test" event from the dashboard:
//
//	GAMECOIN_WEBHOOK_SECRET=whsec_gc_... go run ./examples/webhook
package main

import (
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"sync"

	gamecoin "github.com/apilow/gamecoin-go"
)

// processed remembers the events already handled: GameCoin delivers at least once, so the same id can come twice. In
// a real game, keep this in your database.
type processed struct {
	mu   sync.Mutex
	seen map[string]bool
}

// first reports whether id is seen for the first time, and remembers it.
func (p *processed) first(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.seen[id] {
		return false
	}
	p.seen[id] = true
	return true
}

func main() {
	secret := os.Getenv("GAMECOIN_WEBHOOK_SECRET")
	if secret == "" {
		log.Fatal("set GAMECOIN_WEBHOOK_SECRET to the signing secret of your endpoint (whsec_gc_...)")
	}
	events := &processed{seen: map[string]bool{}}

	http.HandleFunc("/gamecoin/webhook", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// The RAW body: read it before any JSON parsing, the signature covers its exact bytes.
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			http.Error(w, "cannot read the body", http.StatusBadRequest)
			return
		}
		event, err := gamecoin.ConstructEvent(body, r.Header.Get(gamecoin.WebhookSignatureHeader), secret)
		if err != nil {
			var sigErr *gamecoin.WebhookSignatureError
			if errors.As(err, &sigErr) {
				log.Printf("webhook refused: %s", sigErr.Reason)
				http.Error(w, "invalid signature", http.StatusBadRequest)
				return
			}
			log.Printf("webhook error: %v", err)
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if !events.first(event.ID) {
			w.WriteHeader(http.StatusOK) // a repeated event: nothing to do
			return
		}

		switch event.Type {
		case gamecoin.EventOrderFulfilled:
			order := event.Order()
			log.Printf("order %s delivered to player %s (pack %s)", order.ID, order.PlayerID, order.Pack.SKU)
		case gamecoin.EventOrderRefunded, gamecoin.EventOrderChargeback:
			log.Printf("order %s is now %s", event.Order().ID, event.Order().Status)
		case gamecoin.EventWalletCredited:
			for _, credit := range event.WalletCredited().Credits {
				log.Printf("player %s got %d+%d %s", event.WalletCredited().PlayerID, credit.Paid, credit.Bonus, credit.Currency)
			}
		case gamecoin.EventWebhookTest:
			log.Printf("test event: %s", event.Test().Message)
		default:
			log.Printf("event %s of a type this SDK does not know yet: %s", event.ID, event.Type)
		}
		w.WriteHeader(http.StatusOK) // answer within 10 seconds: do the heavy work after, or elsewhere
	})

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
