package network

import "enhanced-tcr-udp/internal/models"

// Standard envelope for all TCP messages to define message type
const (
	MsgTypeLoginRequest       = "login_request"
	MsgTypeLoginResponse      = "login_response"
	MsgTypeMatchmakingRequest = "matchmaking_request"
	MsgTypeMatchFoundResponse = "match_found_response"
	MsgTypeGameConfigData     = "game_config_data"
	MsgTypeGameOverResults    = "game_over_results"
	// Add other TCP message types here as needed
)

type TCPMessage struct {
	Type    string      `json:"type"`    // e.g., MsgTypeLoginRequest
	Payload interface{} `json:"payload"` // The actual data structure for the message type
}

// --- Client to Server (C2S) TCP Messages ---

// LoginRequest is the structure for a client's login attempt.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// MatchmakingRequest is sent by the client to find a game.
type MatchmakingRequest struct {
	PlayerID string `json:"player_id"` // Username or a session token
}

// MatchmakingResponse is sent by the server when a match is found or status update.
type MatchmakingResponse struct {
	Status          string `json:"status"` // e.g., "searching", "match_found", "error"
	Message         string `json:"message"`
	OpponentName    string `json:"opponent_name,omitempty"`
	GameID          string `json:"game_id,omitempty"`           // Unique ID for the game session
	AssignedUDPPort int    `json:"assigned_udp_port,omitempty"` // UDP port for this game
}

// --- Server to Client (S2C) TCP Messages ---

// LoginResponse is the structure for the server's response to a login attempt.
type LoginResponse struct {
	Success bool                  `json:"success"`
	Message string                `json:"message"`
	Player  *models.PlayerAccount `json:"player,omitempty"` // Sent on successful login
}

// MatchFoundResponse is sent when a match is made.
type MatchFoundResponse struct {
	GameID      string               `json:"game_id"`
	Opponent    models.PlayerAccount `json:"opponent"`      // Basic info about the opponent
	UDPPort     int                  `json:"udp_port"`      // UDP port for this game session
	IsPlayerOne bool                 `json:"is_player_one"` // To help client identify its role initially
	// May include initial turn info or other specific game start details
}

// GameConfigData contains the initial game configuration.
type GameConfigData struct {
	Config models.GameConfig `json:"config"`
}

// GameOverResults contains the results of the game.
type GameOverResults struct {
	WinnerID        string         `json:"winner_id,omitempty"` // Empty if draw
	Outcome         string         `json:"outcome"`             // e.g., "Win", "Loss", "Draw"
	EXPChange       int            `json:"exp_change"`
	NewEXP          int            `json:"new_exp"`
	NewLevel        int            `json:"new_level"`
	LevelUp         bool           `json:"level_up"`
	DestroyedTowers map[string]int `json:"destroyed_towers"` // map[playerID]count
}
