package server

import (
	"enhanced-tcr-udp/internal/models"
	"log"
	"sync"
)

// Manages game sessions

// GameSessionManager manages all active game sessions.
type GameSessionManager struct {
	sessions map[string]*GameSession // gameID -> GameSession
	mu       sync.RWMutex
	// Config can be added here later, e.g., reference to game rules, troop/tower specs
}

// NewGameSessionManager creates a new manager for game sessions.
func NewGameSessionManager() *GameSessionManager {
	return &GameSessionManager{
		sessions: make(map[string]*GameSession),
	}
}

// CreateSession creates a new game session for two players.
func (gsm *GameSessionManager) CreateSession(gameID string, player1, player2 *models.PlayerAccount, udpPort int) *GameSession {
	gsm.mu.Lock()
	defer gsm.mu.Unlock()

	if _, exists := gsm.sessions[gameID]; exists {
		log.Printf("Error: Game session %s already exists.", gameID)
		return nil // Or handle error appropriately
	}

	// TODO: Load full game config (troops, towers) here or pass it to NewGameSession
	// For now, NewGameSession will be simple.
	// Use player usernames as session tokens for now.
	// In a more robust system, these tokens might be generated uniquely.
	p1Token := player1.Username
	p2Token := player2.Username
	session := NewGameSession(gameID, player1, player2, p1Token, p2Token, udpPort)
	if session == nil { // NewGameSession can return nil if config loading fails
		log.Printf("Failed to create new game session %s due to initialization error.", gameID)
		return nil
	}
	gsm.sessions[gameID] = session

	log.Printf("Game session %s created for %s and %s on UDP port %d", gameID, player1.Username, player2.Username, udpPort)
	go session.Start() // Start the game loop in a new goroutine
	return session
}

// GetSession retrieves an active game session by its ID.
func (gsm *GameSessionManager) GetSession(gameID string) (*GameSession, bool) {
	gsm.mu.RLock()
	defer gsm.mu.RUnlock()
	session, exists := gsm.sessions[gameID]
	return session, exists
}

// RemoveSession removes a game session, e.g., after it has ended.
func (gsm *GameSessionManager) RemoveSession(gameID string) {
	gsm.mu.Lock()
	defer gsm.mu.Unlock()
	delete(gsm.sessions, gameID)
	log.Printf("Game session %s removed.", gameID)
}
