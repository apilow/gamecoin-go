package gamecoin

import (
	"context"
	"net/http"
)

// CodesService redeems gift codes (POST /players/{player}/codes/redeem). Get it from Client.Codes.
type CodesService struct{ c *core }

// codeBody is the body of a redemption.
type codeBody struct {
	Code string `json:"code"`
}

// Redeem lets a player use a gift code: the free coins (bonus bucket) and the items of the code are given to the player.
// An unknown "ext:" player is created.
//
// The code is sent as it is: the API ignores case, spaces and dashes, so a code typed by a player needs no cleaning
// first. A code that is unknown, malformed, expired, used up, disabled or already used by this player is always the same
// error, CodeValidationFailed with Details["fieldErrors"] holding "code": ["CODE_INVALID"] (nothing tells a guess from a
// real code); it is never retried. A blocked player is CodePlayerBlocked; CodeLimitReached and CodeConflict mean that the
// player can no longer receive what the code holds (the code is not used up then); CodeRateLimited means too many
// attempts.
//
// The call is idempotent: it carries an Idempotency-Key, generated once per call unless WithIdempotencyKey gives one, and
// Redemption.Replayed tells whether the response was a replay (nothing is credited twice).
func (s *CodesService) Redeem(ctx context.Context, player PlayerRef, code string, opts ...CallOption) (*Redemption, error) {
	path, err := playerPath(player)
	if err != nil {
		return nil, err
	}
	if code == "" {
		return nil, argError("the code must not be empty")
	}
	key, err := resolveIdempotencyKey(opts)
	if err != nil {
		return nil, err
	}
	encoded, err := marshalBody(codeBody{Code: code})
	if err != nil {
		return nil, err
	}
	var out Redemption
	replayed, err := s.c.do(ctx, request{method: http.MethodPost, path: path + "/codes/redeem", body: encoded, idempotencyKey: key}, &out)
	if err != nil {
		return nil, err
	}
	out.Replayed = replayed
	return &out, nil
}
