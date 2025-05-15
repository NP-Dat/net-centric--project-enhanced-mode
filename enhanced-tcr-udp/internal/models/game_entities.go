package models

import (
	"net"
	"time"
)

// TowerInstance represents a tower currently in a game session.
type TowerInstance struct {
	SpecID      string `json:"spec_id"`  // References TowerSpec.ID
	OwnerID     string `json:"owner_id"` // Player ID who owns this tower
	CurrentHP   int    `json:"current_hp"`
	MaxHP       int    `json:"max_hp"`      // Max HP considering player level
	CurrentATK  int    `json:"current_atk"` // ATK considering player level
	CurrentDEF  int    `json:"current_def"` // DEF considering player level
	IsDestroyed bool   `json:"is_destroyed"`
	// Potentially add position/ID for targeting, e.g., guard_tower_1, guard_tower_2, king_tower
	GameSpecificID string `json:"game_specific_id"` // e.g. "player1_king_tower"
}

// ActiveTroop represents a troop deployed on the game field.
// Queen is a special case as per plan: "Deployment is a one-time action... Does not persist on the board."
// So, ActiveTroop will mainly represent other troops that do persist and attack.
type ActiveTroop struct {
	InstanceID string    `json:"instance_id"` // Unique ID for this troop instance in the game
	SpecID     string    `json:"spec_id"`     // References TroopSpec.ID
	OwnerID    string    `json:"owner_id"`    // Player ID who owns this troop
	CurrentHP  int       `json:"current_hp"`  // HP considering player level
	MaxHP      int       `json:"max_hp"`
	CurrentATK int       `json:"current_atk"` // ATK considering player level
	CurrentDEF int       `json:"current_def"` // DEF considering player level (though it only attacks towers)
	TargetID   string    `json:"target_id"`   // ID of the TowerInstance it's targeting
	DeployedAt time.Time `json:"deployed_at"`
	// Position might be needed later if we have a more complex board
}

// PlayerInGame represents a player's state within an active game session.
type PlayerInGame struct {
	Account        PlayerAccount           `json:"account"` // Copy of player account details, including current level
	TCPConn        net.Conn                `json:"-"`       // TCP connection (for reliable messages, not for JSON marshal)
	UDPAddr        *net.UDPAddr            `json:"-"`       // UDP address (not for JSON marshal)
	CurrentMana    int                     `json:"current_mana"`
	Towers         []*TowerInstance        `json:"towers"`           // Player's towers in the game
	DeployedTroops map[string]*ActiveTroop `json:"deployed_troops"`  // Keyed by ActiveTroop.InstanceID
	LastActionTime time.Time               `json:"last_action_time"` // For timeouts or other logic
	SessionToken   string                  `json:"session_token"`    // Token to identify player in UDP messages
}

// GameSession represents an active game between two players.
type GameSession struct {
	SessionID      string        `json:"session_id"`
	Player1        *PlayerInGame `json:"player1"`
	Player2        *PlayerInGame `json:"player2"`
	GameConfig     *GameConfig   `json:"game_config"` // Reference to the loaded game configuration
	StartTime      time.Time     `json:"start_time"`
	GameTimer      *time.Timer   `json:"-"` // Server-side timer for game duration
	LastUpdateTime time.Time     `json:"last_update_time"`
	GameState      string        `json:"game_state"` // e.g., "Initializing", "InProgress", "Finished"
	// Potentially include a list of all active troops from both players for easier global access if needed
	// AllActiveTroops map[string]*ActiveTroop `json:"all_active_troops"`

	// UDP related fields
	UDPPort        int          `json:"udp_port"` // Dedicated UDP port for this game session
	ClientUDPAddr1 *net.UDPAddr `json:"-"`        // Caching client UDP address for player 1
	ClientUDPAddr2 *net.UDPAddr `json:"-"`        // Caching client UDP address for player 2
}
