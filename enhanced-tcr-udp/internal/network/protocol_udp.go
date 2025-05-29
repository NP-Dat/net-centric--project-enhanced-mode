package network

import (
	"enhanced-tcr-udp/internal/models"
	"time"
)

// General structure for UDP messages for identification and ordering
type UDPMessage struct {
	Seq         uint32      `json:"seq"`          // Sequence number
	Timestamp   time.Time   `json:"timestamp"`    // Client or Server timestamp
	SessionID   string      `json:"session_id"`   // Game Session ID
	PlayerToken string      `json:"player_token"` // Player identifier within the session (e.g., from PlayerInGame.SessionToken)
	Type        string      `json:"type"`         // e.g., UDPMsgTypeDeployTroop
	Payload     interface{} `json:"payload"`      // Actual data for the message type
}

// UDP Message Types
const (
	UDPMsgTypeDeployTroop     = "deploy_troop_command_udp"
	UDPMsgTypePlayerInput     = "player_input_udp" // Generic placeholder
	UDPMsgTypeGameStateUpdate = "game_state_update_udp"
	UDPMsgTypeGameEvent       = "game_event_udp"
	UDPMsgTypePlayerQuit      = "player_quit_udp" // New: Client signals quit
	UDPMsgTypeCommandAck      = "command_ack_udp" // New: Server acknowledges a critical client command
	// Add other UDP message types here

	// Game Event Types (for GameEventUDP.EventType and server-side gs.sendGameEventToAllPlayers)
	GameEventTowerDamaged   = "event_tower_damaged"
	GameEventTroopDamaged   = "event_troop_damaged"
	GameEventTowerDestroyed = "event_tower_destroyed"
	GameEventTroopDefeated  = "event_troop_defeated"
	GameEventCritHit        = "event_crit_hit"
	GameEventQueenHeal      = "event_queen_heal"
	GameEventTroopDeployed  = "event_troop_deployed"
	GameEventError          = "event_error" // For sending errors to a specific player
)

// --- Client to Server (C2S) UDP Messages ---

// DeployTroopCommandUDP is sent by a client to deploy a troop.
type DeployTroopCommandUDP struct {
	TroopID string `json:"troop_id"` // TroopSpec.ID of the troop to deploy
	// Optional: Lane or position if the game board becomes more complex
}

// PlayerInputUDP is a generic structure for other player inputs.
// This can be expanded or replaced with more specific input types.
type PlayerInputUDP struct {
	InputType string      `json:"input_type"` // e.g., "use_special_ability"
	Details   interface{} `json:"details"`    // Command-specific details
}

// PlayerQuitUDP is sent by a client to signal they are quitting the game session.
// It currently has no additional payload beyond what's in UDPMessage.
type PlayerQuitUDP struct {
	// No specific fields needed for now, PlayerToken in UDPMessage is enough
}

// --- Server to Client (S2C) UDP Messages ---

// CommandAckUDP is sent by the server to acknowledge a critical command from the client.
type CommandAckUDP struct {
	AckSeq uint32 `json:"ack_seq"` // Sequence number of the client's command being acknowledged
}

// GameStateUpdateUDP contains the current state of the game.
// This can be a full snapshot or a delta.
// For simplicity, starting with a fuller snapshot.
type GameStateUpdateUDP struct {
	GameTimeRemainingSeconds int                           `json:"game_time_remaining_seconds"`
	Player1Mana              int                           `json:"player1_mana"`
	Player2Mana              int                           `json:"player2_mana"`
	Towers                   []models.TowerInstance        `json:"towers"`                              // All towers from both players
	ActiveTroops             map[string]models.ActiveTroop `json:"active_troops"`                       // All active troops from both players, keyed by InstanceID
	PlayerScores             map[string]int                `json:"player_scores,omitempty"`             // e.g., towers destroyed by each player
	LastProcessedClientSeq   map[string]uint32             `json:"last_processed_client_seq,omitempty"` // map[PlayerToken]sequence_number, for client-side prediction/reconciliation
}

// GameEventUDP is for broadcasting significant one-off events.
type GameEventUDP struct {
	EventType string      `json:"event_type"` // e.g., "TowerDestroyed", "CritialHit", "QueenHealUsed"
	Details   interface{} `json:"details"`    // Event-specific details
	// Example Detail for TowerDestroyed:
	// TowerDestroyedDetails struct {
	//     TowerID string `json:"tower_id"`
	//     DestroyerID string `json:"destroyer_id"` // Troop or Player ID
	// }
}
