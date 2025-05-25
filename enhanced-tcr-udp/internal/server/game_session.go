package server

import (
	"encoding/json"
	"enhanced-tcr-udp/internal/models"
	"enhanced-tcr-udp/internal/network"
	"enhanced-tcr-udp/internal/persistence"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"
	// "enhanced-tcr-udp/internal/game" // For actual game logic
	// "enhanced-tcr-udp/internal/persistence" // For loading game config
)

// GameSession represents an active game between two players.
type GameSession struct {
	ID          string
	Player1     *models.PlayerInGame // Extended struct with in-game state
	Player2     *models.PlayerInGame
	Config      models.GameConfig // Loaded game configuration (troops, towers)
	udpPort     int
	udpConn     *net.UDPConn // Server-side UDP connection for this session
	startTime   time.Time
	gameEndTime time.Time
	mu          sync.RWMutex

	player1Quit bool
	player2Quit bool

	playerClientAddresses map[string]*net.UDPAddr // Maps PlayerToken to their last known UDP address for targeted responses

	// Add channels for player actions, game events, etc.
	playerActions chan network.UDPMessage // Channel to receive player actions
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

	startTime := time.Now()
	// TODO: Initialize PlayerInGame with stats based on PlayerAccount level and gameCfg
	gs := &GameSession{
		ID:                    id,
		Player1:               &models.PlayerInGame{Account: *p1Acc},
		Player2:               &models.PlayerInGame{Account: *p2Acc},
		Config:                gameCfg,
		udpPort:               udpPort,
		startTime:             startTime,
		gameEndTime:           startTime.Add(3 * time.Minute),
		playerActions:         make(chan network.UDPMessage, 10), // Buffered channel
		playerClientAddresses: make(map[string]*net.UDPAddr),
	}
	log.Printf("Initializing GameSession %s for %s and %s on UDP port %d.", id, p1Acc.Username, p2Acc.Username, gameCfg.Towers != nil)

	if err := gs.setupUDPConnectionAndListener(); err != nil {
		log.Printf("[GameSession %s] Failed to setup UDP listener: %v. Aborting session.", gs.ID, err)
		return nil // Session cannot function without UDP
	}

	// go gs.listenForUDPPackets() // This is now called by setupUDPConnectionAndListener
	return gs
}

// Start begins the game loop for the session.
func (gs *GameSession) Start() {
	log.Printf("Game session %s started. Game will end at %v", gs.ID, gs.gameEndTime)
	// TODO: Implement game loop (timer, mana regen, processing inputs, sending updates)
	// This will run in its own goroutine.

	ticker := time.NewTicker(1 * time.Second) // Tick every second for timer updates
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			gs.mu.Lock()
			if time.Now().After(gs.gameEndTime) {
				log.Printf("Game session %s timer ended.", gs.ID)
				gs.mu.Unlock()
				gs.Stop() // Or a more specific game end function
				return
			}
			// TODO: Implement mana regeneration here (Sprint 3)

			// Send game state update
			timeRemaining := gs.gameEndTime.Sub(time.Now()).Seconds()
			// TODO: Populate with actual mana, tower, and troop data
			// gameStateUpdate := network.GameStateUpdateUDP{
			// 	GameTimeRemainingSeconds: int(timeRemaining),
			// 	Player1Mana:              gs.Player1.CurrentMana, // Placeholder
			// 	Player2Mana:              gs.Player2.CurrentMana, // Placeholder
			// 	// Towers:                   // Placeholder
			// 	// ActiveTroops:             // Placeholder
			// }
			// gs.broadcastUDPMessage(gameStateUpdate) // TODO: Implement broadcastUDPMessage

			log.Printf("Game session %s: Time remaining: %d seconds", gs.ID, int(timeRemaining))
			gs.mu.Unlock()
			// TODO: Add case for <-gs.stopChannel (or similar mechanism for forceful stop)
			// TODO: Add case for processing player inputs from a channel
		case action := <-gs.playerActions:
			gs.mu.Lock()
			gs.handlePlayerAction(action)
			// Check if game should end due to quits after handling action
			if gs.player1Quit && gs.player2Quit {
				log.Printf("Both players have quit game session %s. Stopping.", gs.ID)
				gs.mu.Unlock()
				gs.Stop()
				return
			}
			gs.mu.Unlock()
		}
	}
}

// handlePlayerAction processes a UDP message received from a player.
func (gs *GameSession) handlePlayerAction(msg network.UDPMessage) {
	// Assumes gs.mu is locked by caller if state modification occurs
	log.Printf("[GameSession %s] Received action: Type=%s, Player=%s", gs.ID, msg.Type, msg.PlayerToken)

	switch msg.Type {
	case network.UDPMsgTypePlayerQuit:
		if msg.PlayerToken == gs.Player1.Account.Username {
			gs.player1Quit = true
			log.Printf("Player %s has quit session %s.", gs.Player1.Account.Username, gs.ID)
		} else if msg.PlayerToken == gs.Player2.Account.Username {
			gs.player2Quit = true
			log.Printf("Player %s has quit session %s.", gs.Player2.Account.Username, gs.ID)
		}
	// TODO: Add other cases like UDPMsgTypeDeployTroop (Sprint 3)
	default:
		log.Printf("[GameSession %s] Received unhandled player action type: %s", gs.ID, msg.Type)
	}
}

// Stop ends the game session.
func (gs *GameSession) Stop() {
	log.Printf("Game session %s stopped.", gs.ID)
	if gs.udpConn != nil {
		gs.udpConn.Close()
	}
	// TODO: Persist player EXP/level changes, notify SessionManager to remove session.
}

// setupUDPConnectionAndListener sets up the UDP listener for this game session.
func (gs *GameSession) setupUDPConnectionAndListener() error {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", gs.udpPort))
	if err != nil {
		log.Printf("[GameSession %s] Failed to resolve UDP address for port %d: %v", gs.ID, gs.udpPort, err)
		return err
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Printf("[GameSession %s] Failed to listen on UDP port %d: %v", gs.ID, gs.udpPort, err)
		return err
	}
	gs.udpConn = conn
	log.Printf("[GameSession %s] Listening for UDP on port %d (%s)", gs.ID, gs.udpPort, gs.udpConn.LocalAddr().String())

	go gs.readUDPMessages() // Start the dedicated reader for this session
	return nil
}

// readUDPMessages continuously reads messages from the session's UDP connection
// and forwards them to the playerActions channel.
func (gs *GameSession) readUDPMessages() {
	defer func() {
		if gs.udpConn != nil {
			log.Printf("[GameSession %s] Closing UDP connection on port %d.", gs.ID, gs.udpPort)
			gs.udpConn.Close() // Ensure connection is closed when goroutine exits
		}
	}()

	buffer := make([]byte, 2048) // Buffer for incoming UDP packets

	for {
		n, remoteAddr, err := gs.udpConn.ReadFromUDP(buffer)
		if err != nil {
			// Check if the error is due to the connection being closed (e.g., by gs.Stop())
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Printf("[GameSession %s] UDP read timeout on port %d. Continuing...", gs.ID, gs.udpPort)
				continue
			}
			if strings.Contains(err.Error(), "use of closed network connection") {
				log.Printf("[GameSession %s] UDP listener on port %d stopped (connection closed).", gs.ID, gs.udpPort)
				return // Exit goroutine
			}
			log.Printf("[GameSession %s] Error reading from UDP on port %d: %v. Listener stopping.", gs.ID, gs.udpPort, err)
			return
		}

		var udpMsg network.UDPMessage
		if err := json.Unmarshal(buffer[:n], &udpMsg); err != nil {
			log.Printf("[GameSession %s] Error unmarshalling UDP message from %s: %v. Raw: %s", gs.ID, remoteAddr.String(), err, string(buffer[:n]))
			continue
		}

		// Store/update client address for potential direct responses
		gs.mu.Lock() // Lock for writing to playerClientAddresses
		gs.playerClientAddresses[udpMsg.PlayerToken] = remoteAddr
		log.Printf("[GameSession %s] Stored/Updated remote UDP address for %s to %s", gs.ID, udpMsg.PlayerToken, remoteAddr.String())
		gs.mu.Unlock()

		// Send to actions channel for processing by the game loop
		// Add a non-blocking send with timeout or select to prevent deadlocks if channel is full
		select {
		case gs.playerActions <- udpMsg:
			// log.Printf("[GameSession %s] Forwarded UDP message from %s to playerActions channel.", gs.ID, udpMsg.PlayerToken)
		default:
			log.Printf("[GameSession %s] Warning: playerActions channel full for player %s. Discarding message type %s.", gs.ID, udpMsg.PlayerToken, udpMsg.Type)
		}
	}
}

// This is a simplified listener. In a real server, you might have a central UDP listener
// that dispatches packets to game sessions based on session ID or player tokens.
// For now, this simulates a session-specific listener for incoming actions.
/* func (gs *GameSession) listenForUDPPackets() {
	// This is a placeholder. The actual UDP listening is done by the global UDPServer
	// in server.go. That server needs to route messages to the correct GameSession's
	// playerActions channel.
	// For now, to test the quit logic, we won't implement full routing.
	// We assume that if a message reaches this session's playerActions channel, it's for this session.
	log.Printf("[GameSession %s] Placeholder: listenForUDPPackets started. Real routing TBD.", gs.ID)
}
*/

// TODO: Add methods for handling player actions received via UDP, updating game state, etc.
// TODO: Implement broadcastUDPMessage to send GameStateUpdateUDP to both players using their stored UDP addresses.
