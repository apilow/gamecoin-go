// Command quickstart grants gems to a player, spends some and starts the purchase of a pack. Run it with a test key:
//
//	GAMECOIN_SECRET_KEY=gc_sk_test_... GAMECOIN_BASE_URL=http://localhost:3400 go run ./examples/quickstart
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	gamecoin "github.com/apilow/gamecoin-go"
)

func main() {
	ctx := context.Background()
	baseURL := os.Getenv("GAMECOIN_BASE_URL")
	if baseURL == "" {
		baseURL = gamecoin.DefaultBaseURL
	}
	client, err := gamecoin.New(os.Getenv("GAMECOIN_SECRET_KEY"), gamecoin.WithBaseURL(baseURL))
	if err != nil {
		log.Fatal(err)
	}
	player := gamecoin.Ext(fmt.Sprintf("quickstart-%d", time.Now().UnixNano())) // your own id for the player (a fresh one: every run starts at zero)

	granted, err := client.Wallet.Grant(ctx, player, gamecoin.GrantParams{Currency: "gems", Amount: 100, Reason: "level_up"})
	if err != nil {
		log.Fatal(err)
	}
	spent, err := client.Wallet.Spend(ctx, player, gamecoin.SpendParams{Currency: "gems", Amount: 30})
	if err != nil {
		log.Fatal(err)
	}
	checkout, err := client.Checkouts.Create(ctx, player, gamecoin.CheckoutParams{PackSKU: "gems-500"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("gems: %d -> %d; the player pays at %s\n", granted.Balance.Total, spent.Balance.Total, checkout.CheckoutURL)
}
