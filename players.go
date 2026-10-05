package gamecoin

import (
	"context"
	"net/http"
)

// PlayersService manages players (PUT and GET /players/{player}, POST /players/{player}/tokens). Get it from
// Client.Players.
type PlayersService struct{ c *core }

// playerPath validates a player reference and returns the path of the player, URL-encoded once.
func playerPath(player PlayerRef) (string, error) {
	if !player.valid() {
		return "", argError("the player must not be empty: use gamecoin.Ext(...) or gamecoin.PlayerID(...)")
	}
	return "/players/" + escapeSegment(string(player)), nil
}

// Upsert creates the player if it does not exist (an "ext:" player is created on first use) and updates the fields of
// profile. A nil profile changes nothing; calling Upsert again with the same profile is harmless.
func (s *PlayersService) Upsert(ctx context.Context, player PlayerRef, profile *PlayerProfile) (*Player, error) {
	path, err := playerPath(player)
	if err != nil {
		return nil, err
	}
	body := map[string]any{}
	if profile != nil {
		if body, err = profileBody(profile); err != nil {
			return nil, err
		}
	}
	encoded, err := marshalBody(body)
	if err != nil {
		return nil, err
	}
	var out Player
	if _, err := s.c.do(ctx, request{method: http.MethodPut, path: path, body: encoded}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get returns a player. An unknown player is CodeNotFound, even for an "ext:" reference.
func (s *PlayersService) Get(ctx context.Context, player PlayerRef) (*Player, error) {
	path, err := playerPath(player)
	if err != nil {
		return nil, err
	}
	var out Player
	if _, err := s.c.do(ctx, request{method: http.MethodGet, path: path}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateToken creates a short-lived player token, to hand to the browser of the player (browser SDK). An unknown "ext:"
// player is created.
func (s *PlayersService) CreateToken(ctx context.Context, player PlayerRef) (*PlayerToken, error) {
	path, err := playerPath(player)
	if err != nil {
		return nil, err
	}
	var out PlayerToken
	if _, err := s.c.do(ctx, request{method: http.MethodPost, path: path + "/tokens"}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func profileBody(p *PlayerProfile) (map[string]any, error) {
	body := map[string]any{}
	set := func(field ProfileField, isSet bool, value any) {
		if isSet {
			body[string(field)] = value
		}
	}
	for field, value := range map[ProfileField]*string{FieldDisplayName: p.DisplayName, FieldEmail: p.Email, FieldCountry: p.Country} {
		if value != nil && *value == "" {
			return nil, argError("the profile " + string(field) + " must not be empty (to erase it, list it in Clear)")
		}
	}
	if p.BirthYear != nil && *p.BirthYear < 1 {
		return nil, argError("the profile birthYear must be an integer of at least 1")
	}
	set(FieldDisplayName, p.DisplayName != nil, p.DisplayName)
	set(FieldEmail, p.Email != nil, p.Email)
	set(FieldCountry, p.Country != nil, p.Country)
	set(FieldBirthYear, p.BirthYear != nil, p.BirthYear)
	for _, field := range p.Clear {
		switch field {
		case FieldDisplayName, FieldEmail, FieldCountry, FieldBirthYear:
		default:
			return nil, argError("Clear lists an unknown profile field")
		}
		if value, alsoSet := body[string(field)]; alsoSet && value != nil {
			return nil, argError("the profile " + string(field) + " cannot be both set and cleared")
		}
		body[string(field)] = nil
	}
	return body, nil
}
