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

	playerActions chan network.UDPMessage // Channel to receive player actions
	lastManaRegen time.Time               // For mana regeneration timing
}

// NewGameSession creates a new game session.
func NewGameSession(id string, p1Acc, p2Acc *models.PlayerAccount, p1Token, p2Token string, udpPort int) *GameSession {
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
	gs := &GameSession{
		ID:                    id,
		Player1:               &models.PlayerInGame{Account: *p1Acc, SessionToken: p1Token, CurrentMana: 5, DeployedTroops: make(map[string]*models.ActiveTroop), Towers: make([]*models.TowerInstance, 0)},
		Player2:               &models.PlayerInGame{Account: *p2Acc, SessionToken: p2Token, CurrentMana: 5, DeployedTroops: make(map[string]*models.ActiveTroop), Towers: make([]*models.TowerInstance, 0)},
		Config:                gameCfg,
		udpPort:               udpPort,
		startTime:             startTime,
		gameEndTime:           startTime.Add(3 * time.Minute),
		playerActions:         make(chan network.UDPMessage, 10),
		playerClientAddresses: make(map[string]*net.UDPAddr),
		lastManaRegen:         startTime,
	}
	log.Printf("Initializing GameSession %s for %s and %s on UDP port %d.", id, p1Acc.Username, p2Acc.Username, gameCfg.Towers != nil)

	if err := gs.setupUDPConnectionAndListener(); err != nil {
		log.Printf("[GameSession %s] Failed to setup UDP listener: %v. Aborting session.", gs.ID, err)
		return nil // Session cannot function without UDP
	}

	return gs
}

// Start begins the game loop for the session.
func (gs *GameSession) Start() {
	log.Printf("Game session %s started. Game will end at %v. Player1: %s (Token: %s), Player2: %s (Token: %s)", gs.ID, gs.gameEndTime, gs.Player1.Account.Username, gs.Player1.SessionToken, gs.Player2.Account.Username, gs.Player2.SessionToken)
	// TODO: Implement game loop (timer, mana regen, processing inputs, sending updates)
	// This will run in its own goroutine.

	ticker := time.NewTicker(1 * time.Second) // Tick every second for timer updates & coarse mana regen check
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

			// Mana Regeneration
			if time.Since(gs.lastManaRegen) >= 2*time.Second {
				if gs.Player1.CurrentMana < 10 {
					gs.Player1.CurrentMana++
				}
				if gs.Player2.CurrentMana < 10 {
					gs.Player2.CurrentMana++
				}
				gs.lastManaRegen = time.Now()
				log.Printf("[GameSession %s] Mana Regen: P1: %d, P2: %d", gs.ID, gs.Player1.CurrentMana, gs.Player2.CurrentMana)
			}

			// Send game state update
			timeRemaining := gs.gameEndTime.Sub(time.Now()).Seconds()

			allActiveTroops := make(map[string]models.ActiveTroop)
			for id, troop := range gs.Player1.DeployedTroops {
				allActiveTroops[id] = *troop
			}
			for id, troop := range gs.Player2.DeployedTroops {
				allActiveTroops[id] = *troop
			}

			// TODO: Populate gs.Player1.Towers and gs.Player2.Towers during session setup
			// For now, sending empty tower list.
			var allTowers []models.TowerInstance // Placeholder

			gameStateUpdatePayload := network.GameStateUpdateUDP{
				GameTimeRemainingSeconds: int(timeRemaining),
				Player1Mana:              gs.Player1.CurrentMana,
				Player2Mana:              gs.Player2.CurrentMana,
				Towers:                   allTowers, // Placeholder for actual tower data
				ActiveTroops:             allActiveTroops,
			}
			// Construct UDPMessage envelope
			// TODO: Sequence numbers for server messages
			seq := uint32(time.Now().UnixNano())

			// Create a slice of player tokens for broadcast iteration
			playerTokens := []string{gs.Player1.SessionToken, gs.Player2.SessionToken}

			for _, token := range playerTokens {
				if addr, ok := gs.playerClientAddresses[token]; ok {
					// Create a new message for each player to potentially customize later (e.g. lastProcessedClientSeq)
					msgForPlayer := network.UDPMessage{
						Seq:         seq, // Same sequence for this snapshot
						Timestamp:   time.Now(),
						SessionID:   gs.ID,
						PlayerToken: token, // This field might be used by client to confirm it's for them, or not needed if server manages target address
						Type:        network.UDPMsgTypeGameStateUpdate,
						Payload:     gameStateUpdatePayload,
					}
					gs.sendUDPMessageToAddress(msgForPlayer, addr)
				} else {
					// This case should ideally not happen if clients are properly registered on first contact
					log.Printf("[GameSession %s] No UDP address found for player token %s during game state broadcast.", gs.ID, token)
				}
			}

			gs.mu.Unlock()
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
	// gs.mu is already locked by the caller (the game loop)
	log.Printf("[GameSession %s] Handling action: Type=%s, PlayerToken=%s, SessionID=%s", gs.ID, msg.Type, msg.PlayerToken, msg.SessionID)

	if msg.SessionID != gs.ID {
		log.Printf("[GameSession %s] Discarding message from token %s with incorrect SessionID %s (expected %s)", gs.ID, msg.PlayerToken, msg.SessionID, gs.ID)
		return
	}

	switch msg.Type {
	case network.UDPMsgTypePlayerQuit:
		if msg.PlayerToken == gs.Player1.SessionToken {
			gs.player1Quit = true
			log.Printf("Player %s (Token: %s) has quit session %s.", gs.Player1.Account.Username, gs.Player1.SessionToken, gs.ID)
		} else if msg.PlayerToken == gs.Player2.SessionToken {
			gs.player2Quit = true
			log.Printf("Player %s (Token: %s) has quit session %s.", gs.Player2.Account.Username, gs.Player2.SessionToken, gs.ID)
		} else {
			log.Printf("[GameSession %s] Received quit message from unknown or mismatched token: %s", gs.ID, msg.PlayerToken)
		}

	case network.UDPMsgTypeDeployTroop:
		var deployPayload network.DeployTroopCommandUDP
		payloadMap, ok := msg.Payload.(map[string]interface{})
		if !ok {
			log.Printf("[GameSession %s] Error: DeployTroop payload is not map[string]interface{}. Type: %T", gs.ID, msg.Payload)
			return
		}
		payloadBytes, err := json.Marshal(payloadMap)
		if err != nil {
			log.Printf("[GameSession %s] Error re-marshalling DeployTroop payload map: %v", gs.ID, err)
			return
		}

		if err := json.Unmarshal(payloadBytes, &deployPayload); err != nil {
			log.Printf("[GameSession %s] Error unmarshalling DeployTroopCommandUDP payload: %v. Raw: %s", gs.ID, err, string(payloadBytes))
			return
		}

		log.Printf("[GameSession %s] Processing DeployTroop: PlayerToken=%s, TroopID=%s", gs.ID, msg.PlayerToken, deployPayload.TroopID)

		var player *models.PlayerInGame
		if msg.PlayerToken == gs.Player1.SessionToken {
			player = gs.Player1
		} else if msg.PlayerToken == gs.Player2.SessionToken {
			player = gs.Player2
		} else {
			log.Printf("[GameSession %s] Error: Unknown player token %s for DeployTroop.", gs.ID, msg.PlayerToken)
			return
		}

		troopSpec, troopFound := gs.Config.Troops[deployPayload.TroopID]
		if !troopFound {
			log.Printf("[GameSession %s] Error: TroopID \"%s\" not found in game config for player %s.", gs.ID, deployPayload.TroopID, player.Account.Username)
			gs.sendGameEventToPlayer(player.SessionToken, "DeployFailed", map[string]interface{}{
				"reason":   fmt.Sprintf("Troop ID '%s' not found", deployPayload.TroopID),
				"troop_id": deployPayload.TroopID,
			})
			return
		}

		if player.CurrentMana < troopSpec.ManaCost {
			log.Printf("[GameSession %s] Player %s has insufficient mana (%d) to deploy %s (cost %d).", gs.ID, player.Account.Username, player.CurrentMana, troopSpec.Name, troopSpec.ManaCost)
			gs.sendGameEventToPlayer(player.SessionToken, "DeployFailed", map[string]interface{}{
				"reason":       fmt.Sprintf("Insufficient Mana (%d) for %s (cost %d)", player.CurrentMana, troopSpec.Name, troopSpec.ManaCost),
				"troop_id":     deployPayload.TroopID,
				"mana_needed":  troopSpec.ManaCost,
				"mana_current": player.CurrentMana,
			})
			return
		}

		// Special handling for Queen
		if strings.ToLower(troopSpec.ID) == "queen" || strings.ToLower(troopSpec.Name) == "queen" {
			player.CurrentMana -= troopSpec.ManaCost
			log.Printf("[GameSession %s] Player %s deployed Queen. Mana deducted. Current Mana: %d", gs.ID, player.Account.Username, player.CurrentMana)
			// TODO: Implement Queen's heal ability (Sprint 4)
			gs.sendGameEventToAllPlayers("TroopDeployed", map[string]interface{}{
				"player_id":  player.Account.Username,
				"troop_id":   troopSpec.ID,
				"troop_name": troopSpec.Name,
				"is_queen":   true,
			})
		} else {
			// Standard troop deployment
			player.CurrentMana -= troopSpec.ManaCost

			newInstanceID := fmt.Sprintf("%s_%s_%d", player.Account.Username, troopSpec.ID, time.Now().UnixNano())
			activeTroop := &models.ActiveTroop{
				InstanceID: newInstanceID,
				SpecID:     troopSpec.ID,
				OwnerID:    player.Account.Username,
				CurrentHP:  troopSpec.BaseHP,  // TODO: Adjust with player level (Sprint 5)
				MaxHP:      troopSpec.BaseHP,  // TODO: Adjust with player level (Sprint 5)
				CurrentATK: troopSpec.BaseATK, // TODO: Adjust with player level (Sprint 5)
				CurrentDEF: troopSpec.BaseDEF, // TODO: Adjust with player level (Sprint 5)
				DeployedAt: time.Now(),
				// TargetID will be set by combat logic (Sprint 4)
			}
			player.DeployedTroops[newInstanceID] = activeTroop
			log.Printf("[GameSession %s] Player %s deployed %s (ID: %s). Mana: %d. Active Troops: %d", gs.ID, player.Account.Username, troopSpec.Name, newInstanceID, player.CurrentMana, len(player.DeployedTroops))
			gs.sendGameEventToAllPlayers("TroopDeployed", map[string]interface{}{
				"player_id":   player.Account.Username,
				"troop_id":    troopSpec.ID,
				"troop_name":  troopSpec.Name,
				"instance_id": newInstanceID,
				"is_queen":    false,
			})
		}

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

// sendUDPMessageToAddress sends a UDPMessage to a specific client UDP address.
func (gs *GameSession) sendUDPMessageToAddress(msg network.UDPMessage, addr *net.UDPAddr) {
	if gs.udpConn == nil {
		log.Printf("[GameSession %s] Cannot send UDP message, udpConn is nil.", gs.ID)
		return
	}
	if addr == nil {
		log.Printf("[GameSession %s] Cannot send UDP message, target address is nil for PlayerToken %s.", gs.ID, msg.PlayerToken)
		return
	}

	bytes, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[GameSession %s] Error marshalling UDP message for %s (Type: %s): %v", gs.ID, addr.String(), msg.Type, err)
		return
	}

	_, err = gs.udpConn.WriteToUDP(bytes, addr)
	if err != nil {
		log.Printf("[GameSession %s] Error sending UDP message to %s (Type: %s): %v", gs.ID, addr.String(), msg.Type, err)
	} else {
		// log.Printf("[GameSession %s] Sent UDP message type %s to %s (PlayerToken: %s)", gs.ID, msg.Type, addr.String(), msg.PlayerToken)
	}
}

// sendGameEventToAllPlayers broadcasts a game event to both players in the session.
func (gs *GameSession) sendGameEventToAllPlayers(eventType string, details map[string]interface{}) {
	eventPayload := network.GameEventUDP{
		EventType: eventType,
		Details:   details,
	}
	// TODO: Proper sequence numbers for server events
	msg := network.UDPMessage{
		Seq:       uint32(time.Now().UnixNano()),
		Timestamp: time.Now(),
		SessionID: gs.ID,
		Type:      network.UDPMsgTypeGameEvent,
		Payload:   eventPayload,
	}

	if addr1, ok1 := gs.playerClientAddresses[gs.Player1.SessionToken]; ok1 {
		// PlayerToken in msg can be generic or specific if needed by client to filter
		msg.PlayerToken = gs.Player1.SessionToken
		gs.sendUDPMessageToAddress(msg, addr1)
	}
	if addr2, ok2 := gs.playerClientAddresses[gs.Player2.SessionToken]; ok2 {
		msg.PlayerToken = gs.Player2.SessionToken
		gs.sendUDPMessageToAddress(msg, addr2)
	}
	log.Printf("[GameSession %s] Broadcasted GameEvent: Type=%s, Details=%v", gs.ID, eventType, details)
}

// sendGameEventToPlayer sends a game event to a specific player.
func (gs *GameSession) sendGameEventToPlayer(playerToken string, eventType string, details map[string]interface{}) {
	if addr, ok := gs.playerClientAddresses[playerToken]; ok {
		eventPayload := network.GameEventUDP{
			EventType: eventType,
			Details:   details,
		}
		msg := network.UDPMessage{
			Seq:         uint32(time.Now().UnixNano()), // TODO: Proper sequence numbers
			Timestamp:   time.Now(),
			SessionID:   gs.ID,
			PlayerToken: playerToken, // Target specific player
			Type:        network.UDPMsgTypeGameEvent,
			Payload:     eventPayload,
		}
		gs.sendUDPMessageToAddress(msg, addr)
		log.Printf("[GameSession %s] Sent GameEvent to %s: Type=%s, Details=%v", gs.ID, playerToken, eventType, details)
	} else {
		log.Printf("[GameSession %s] Failed to send GameEvent to %s: address not found.", gs.ID, playerToken)
	}
}
