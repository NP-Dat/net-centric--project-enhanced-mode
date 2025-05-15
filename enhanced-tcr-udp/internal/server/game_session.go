package server

import (
	"enhanced-tcr-udp/internal/models"
	"enhanced-tcr-udp/internal/persistence"
	"log"
	"net"
	"sync"
	"time"
	// "enhanced-tcr-udp/internal/game" // For actual game logic
	// "enhanced-tcr-udp/internal/persistence" // For loading game config
)

// GameSession represents an active game between two players.
type GameSession struct {
	ID        string
	Player1   *models.PlayerInGame // Extended struct with in-game state
	Player2   *models.PlayerInGame
	Config    models.GameConfig // Loaded game configuration (troops, towers)
	udpPort   int
	udpConn   *net.UDPConn // Server-side UDP connection for this session
	startTime time.Time
	mu        sync.RWMutex
	// Add channels for player actions, game events, etc.
	// Config models.GameConfig // Loaded troop/tower specs
}

// NewGameSession creates a new game session.
func NewGameSession(id string, p1Acc, p2Acc *models.PlayerAccount, udpPort int) *GameSession {
	towerConf, err := persistence.LoadTowerConfig()
	if err != nil {
		log.Printf("[GameSession %s] Error loading tower config: %v. Aborting session.", id, err)
		return nil
	}
	troopConf, err := persistence.LoadTroopConfig()
	if err != nil {
		log.Printf("[GameSession %s] Error loading troop config: %v. Aborting session.", id, err)
		return nil
	}

	gameCfg := models.GameConfig{
		Towers: towerConf,
		Troops: troopConf,
	}

	// TODO: Initialize PlayerInGame with stats based on PlayerAccount level and gameCfg
	gs := &GameSession{
		ID:        id,
		Player1:   &models.PlayerInGame{Account: *p1Acc},
		Player2:   &models.PlayerInGame{Account: *p2Acc},
		Config:    gameCfg,
		udpPort:   udpPort,
		startTime: time.Now(),
	}
	log.Printf("Initializing GameSession %s for %s and %s with loaded config.", id, p1Acc.Username, p2Acc.Username)
	// gs.setupUDPListener() //TODO
	return gs
}

// Start begins the game loop for the session.
func (gs *GameSession) Start() {
	log.Printf("Game session %s started.", gs.ID)
	// TODO: Implement game loop (timer, mana regen, processing inputs, sending updates)
	// This will run in its own goroutine.
}

// Stop ends the game session.
func (gs *GameSession) Stop() {
	log.Printf("Game session %s stopped.", gs.ID)
	if gs.udpConn != nil {
		gs.udpConn.Close()
	}
	// TODO: Persist player EXP/level changes, notify SessionManager to remove session.
}

// setupUDPListener sets up the UDP listener for this game session.
/* func (gs *GameSession) setupUDPListener() {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", gs.udpPort))
	if err != nil {
		log.Printf("[GameSession %s] Failed to resolve UDP address: %v", gs.ID, err)
		// TODO: Handle error, maybe abort session creation
		return
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Printf("[GameSession %s] Failed to listen on UDP port %d: %v", gs.ID, gs.udpPort, err)
		// TODO: Handle error
		return
	}
	gs.udpConn = conn
	log.Printf("[GameSession %s] Listening for UDP on port %d", gs.ID, gs.udpPort)

	// TODO: Start a goroutine here to read UDP packets from players
	// go gs.readUDPMessages()
}*/

// TODO: Add methods for handling player actions received via UDP, updating game state, etc.
