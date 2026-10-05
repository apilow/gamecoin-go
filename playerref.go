package gamecoin

// PlayerRef designates a player: either a GameCoin player id, or your game's own id through Ext ("ext:user-42"). An
// "ext:" player is created the first time a write call names it.
//
// Every call takes a PlayerRef. Because it is a string type, an untyped string constant converts implicitly
// (client.Wallet.Get(ctx, "6ac40ac0…")); use PlayerID for a string variable.
type PlayerRef string

// Ext is your game's own id for the player: Ext("user-42") is the GameCoin reference "ext:user-42". Any non-empty string
// works; spaces, slashes or non-ASCII characters are URL-encoded once by the SDK.
func Ext(externalID string) PlayerRef { return PlayerRef("ext:" + externalID) }

// PlayerID is a GameCoin player id, as returned in Player.ID.
func PlayerID(id string) PlayerRef { return PlayerRef(id) }

// String returns the reference as the API names it: a GameCoin id or "ext:<your id>".
func (p PlayerRef) String() string { return string(p) }

// valid reports whether the reference can be sent: not empty, and not a bare "ext:" prefix (Ext of an empty id).
func (p PlayerRef) valid() bool { return p != "" && p != "ext:" }
