package models

// PlayerAccount holds information about a player that persists between sessions.
type PlayerAccount struct {
	Username       string `json:"username"`
	HashedPassword string `json:"hashed_password"` // bcrypted
	EXP            int    `json:"exp"`
	Level          int    `json:"level"`
	GameID         string `json:"game_id,omitempty"` // Added to store current game ID if in a session
}
