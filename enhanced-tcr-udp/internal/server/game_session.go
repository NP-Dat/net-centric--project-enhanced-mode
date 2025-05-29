package server

import (
	"encoding/json"
	"enhanced-tcr-udp/internal/game" // Added for game logic
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
	// Add timers for troop and tower attacks
	lastTroopAttack map[string]time.Time           // Key: Troop InstanceID
	lastTowerAttack map[string]time.Time           // Key: Tower GameSpecificID
	activeTroops    map[string]*models.ActiveTroop // Centralized map for all active troops
	towers          []*models.TowerInstance        // Centralized list of all towers
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
		lastTroopAttack:       make(map[string]time.Time),
		lastTowerAttack:       make(map[string]time.Time),
		activeTroops:          make(map[string]*models.ActiveTroop), // Initialize centralized map
		towers:                make([]*models.TowerInstance, 0),     // Initialize centralized list
	}

	// Initialize towers for Player 1
	initializePlayerTowers(gs.Player1, gs.Config.Towers, "player1", gs.Player1.Account.Level) // Pass player level
	// Initialize towers for Player 2
	initializePlayerTowers(gs.Player2, gs.Config.Towers, "player2", gs.Player2.Account.Level) // Pass player level

	// Populate the centralized towers list
	gs.towers = append(gs.towers, gs.Player1.Towers...)
	gs.towers = append(gs.towers, gs.Player2.Towers...)

	// Initialize lastAttack times for towers
	now := time.Now()
	for _, tower := range gs.towers {
		gs.lastTowerAttack[tower.GameSpecificID] = now
	}

	log.Printf("Initializing GameSession %s for %s and %s. Player1 Towers: %d, Player2 Towers: %d. Total towers: %d", id, p1Acc.Username, p2Acc.Username, len(gs.Player1.Towers), len(gs.Player2.Towers), len(gs.towers))

	if err := gs.setupUDPConnectionAndListener(); err != nil {
		log.Printf("[GameSession %s] Failed to setup UDP listener: %v. Aborting session.", gs.ID, err)
		return nil // Session cannot function without UDP
	}

	return gs
}

// initializePlayerTowers creates tower instances for a player based on config.
func initializePlayerTowers(player *models.PlayerInGame, towerSpecs map[string]models.TowerSpec, playerPrefix string, playerLevel int) {
	// Calculate stat multiplier based on player level (10% cumulative per level)
	// Level 1 = base stats (multiplier 1.0)
	// Level 2 = base stats * 1.1
	// Level N = base stats * (1.1)^(N-1)
	levelMultiplier := 1.0
	if playerLevel > 1 {
		for i := 1; i < playerLevel; i++ {
			levelMultiplier *= 1.1
		}
	}

	log.Printf("[GameSession] Initializing towers for %s (Level %d) with multiplier %.2f", player.Account.Username, playerLevel, levelMultiplier)
	for specID, spec := range towerSpecs {
		log.Printf("[GameSession] Processing tower specID: '%s', Name: '%s', BaseHP: %d", specID, spec.Name, spec.BaseHP)
		gameSpecificID := fmt.Sprintf("%s_%s", playerPrefix, strings.ToLower(strings.ReplaceAll(spec.Name, " ", "_")))
		if spec.Name == "" {
			// If spec.Name is empty, use specID to make GameSpecificID more robust and prevent it from being just "playerPrefix_"
			gameSpecificID = fmt.Sprintf("%s_%s", playerPrefix, strings.ToLower(strings.ReplaceAll(specID, " ", "_")))
			log.Printf("[GameSession] Warning: spec.Name is empty for specID '%s'. Using specID for GameSpecificID part: %s", specID, gameSpecificID)
		}

		instance := &models.TowerInstance{
			SpecID:         specID,
			OwnerID:        player.Account.Username, // Use Username as OwnerID
			MaxHP:          int(float64(spec.BaseHP) * levelMultiplier),
			CurrentHP:      int(float64(spec.BaseHP) * levelMultiplier),
			CurrentATK:     int(float64(spec.BaseATK) * levelMultiplier),
			CurrentDEF:     int(float64(spec.BaseDEF) * levelMultiplier),
			IsDestroyed:    false,
			GameSpecificID: gameSpecificID,
		}
		if instance.MaxHP == 0 && spec.BaseHP != 0 { // Log if MaxHP ended up 0 but BaseHP was not
			log.Printf("[GameSession] Warning: Tower %s (SpecID: %s) initialized with MaxHP 0 despite BaseHP %d and multiplier %.2f", instance.GameSpecificID, specID, spec.BaseHP, levelMultiplier)
		}
		player.Towers = append(player.Towers, instance)
	}
	log.Printf("Initialized %d towers for player %s (Level %d) with multiplier %.2f", len(player.Towers), player.Account.Username, playerLevel, levelMultiplier)
}

// Start begins the game loop for the session.
func (gs *GameSession) Start() {
	log.Printf("Game session %s started. Game will end at %v. Player1: %s (Token: %s), Player2: %s (Token: %s)", gs.ID, gs.gameEndTime, gs.Player1.Account.Username, gs.Player1.SessionToken, gs.Player2.Account.Username, gs.Player2.SessionToken)

	ticker := time.NewTicker(1 * time.Second) // Tick every second
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			gs.mu.Lock()
			if time.Now().After(gs.gameEndTime) {
				log.Printf("Game session %s timer ended.", gs.ID)
				// TODO: Determine winner based on rules (King Tower or most towers destroyed)
				gs.mu.Unlock()
				gs.Stop()
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
			}

			// --- Continuous Attack Logic ---
			// Troops attack towers (1 per 2 seconds, as per plan)
			currentTime := time.Now()
			for troopID, troop := range gs.activeTroops {
				if troop.CurrentHP > 0 && currentTime.Sub(gs.lastTroopAttack[troopID]) >= 2*time.Second {
					targetTower := game.FindLowestHPTower(troop.OwnerID, gs.toModelGameSession()) // Pass models.GameSession
					if targetTower != nil && targetTower.CurrentHP > 0 {
						// TroopSpec needed for ATK. Assuming troop.CurrentATK is already set based on level.
						damage := game.CalculateDamage(troop.CurrentATK, targetTower.CurrentDEF, false, 0) // Troops have 0% CRIT
						if damage > 0 {
							originalHP := targetTower.CurrentHP
							game.ApplyDamageToTower(targetTower, damage)
							log.Printf("[GameSession %s] Troop %s (Owner: %s) attacked Tower %s (Owner: %s) for %d damage. HP %d -> %d",
								gs.ID, troop.SpecID, troop.OwnerID, targetTower.GameSpecificID, targetTower.OwnerID, damage, originalHP, targetTower.CurrentHP)
							gs.sendGameEventToAllPlayers(network.GameEventTowerDamaged, map[string]interface{}{
								"attacker_id": troop.InstanceID, "attacker_spec": troop.SpecID, "defender_id": targetTower.GameSpecificID, "defender_spec": targetTower.SpecID, "damage": damage, "new_hp": targetTower.CurrentHP,
							})
							if targetTower.CurrentHP == 0 {
								targetTower.IsDestroyed = true
								log.Printf("[GameSession %s] Tower %s (Owner: %s) DESTROYED by Troop %s (Owner: %s)!",
									gs.ID, targetTower.GameSpecificID, targetTower.OwnerID, troop.SpecID, troop.OwnerID)
								gs.sendGameEventToAllPlayers(network.GameEventTowerDestroyed, map[string]interface{}{
									"tower_id": targetTower.GameSpecificID, "tower_spec": targetTower.SpecID, "owner_id": targetTower.OwnerID, "destroyed_by_troop_id": troop.InstanceID,
								})
								// TODO: Check for King Tower destruction for instant win
							}
						}
					}
					gs.lastTroopAttack[troopID] = currentTime
				}
			}

			// Towers attack troops (1 per 2 seconds, as per plan)
			for _, tower := range gs.towers {
				if tower.CurrentHP > 0 && currentTime.Sub(gs.lastTowerAttack[tower.GameSpecificID]) >= 2*time.Second {
					// TowerSpec needed for CRIT chance. Find it from gs.Config.Towers using tower.SpecID
					towerSpec, specOk := gs.Config.Towers[tower.SpecID]
					critChance := 0.0
					if specOk {
						critChance = towerSpec.CritChance // Assuming CritChance is float64 (0.0 to 1.0)
					}

					targetTroop := game.FindTroopToAttack(tower.OwnerID, gs.toModelGameSession()) // Pass models.GameSession
					if targetTroop != nil && targetTroop.CurrentHP > 0 {
						damage := game.CalculateDamage(tower.CurrentATK, targetTroop.CurrentDEF, true, critChance)
						if damage > 0 {
							originalHP := targetTroop.CurrentHP
							game.ApplyDamageToTroop(targetTroop, damage)
							log.Printf("[GameSession %s] Tower %s (Owner: %s) attacked Troop %s (ID: %s, Owner: %s) for %d damage. HP %d -> %d",
								gs.ID, tower.GameSpecificID, tower.OwnerID, targetTroop.SpecID, targetTroop.InstanceID, targetTroop.OwnerID, damage, originalHP, targetTroop.CurrentHP)
							eventData := map[string]interface{}{
								"attacker_id": tower.GameSpecificID, "attacker_spec": tower.SpecID, "defender_id": targetTroop.InstanceID, "defender_spec": targetTroop.SpecID, "damage": damage, "new_hp": targetTroop.CurrentHP,
							}
							if damage > tower.CurrentATK-targetTroop.CurrentDEF { // Indicates a CRIT occurred
								gs.sendGameEventToAllPlayers(network.GameEventCritHit, eventData)
							} else {
								gs.sendGameEventToAllPlayers(network.GameEventTroopDamaged, eventData)
							}

							if targetTroop.CurrentHP == 0 {
								log.Printf("[GameSession %s] Troop %s (ID: %s, Owner: %s) DEFEATED by Tower %s (Owner: %s)!",
									gs.ID, targetTroop.SpecID, targetTroop.InstanceID, targetTroop.OwnerID, tower.GameSpecificID, tower.OwnerID)
								gs.sendGameEventToAllPlayers(network.GameEventTroopDefeated, map[string]interface{}{
									"troop_id": targetTroop.InstanceID, "troop_spec": targetTroop.SpecID, "owner_id": targetTroop.OwnerID, "defeated_by_tower_id": tower.GameSpecificID,
								})
								// Remove defeated troop from activeTroops
								delete(gs.activeTroops, targetTroop.InstanceID)
								// Also remove from player's DeployedTroops map
								if troopOwner := gs.getPlayerByUsername(targetTroop.OwnerID); troopOwner != nil {
									delete(troopOwner.DeployedTroops, targetTroop.InstanceID)
								}
							}
						}
					}
					gs.lastTowerAttack[tower.GameSpecificID] = currentTime
				}
			}
			// --- End Continuous Attack Logic ---

			// Send game state update
			timeRemaining := gs.gameEndTime.Sub(time.Now()).Seconds()

			// Collect all active troops for the game state update
			activeTroopsForState := make(map[string]models.ActiveTroop)
			for id, troop := range gs.activeTroops { // Use the centralized gs.activeTroops
				activeTroopsForState[id] = *troop
			}

			// Collect all tower instances for the game state update
			towersForState := make([]models.TowerInstance, 0, len(gs.towers))
			for _, tower := range gs.towers { // Use the centralized gs.towers
				towersForState = append(towersForState, *tower)
			}

			gameStateUpdatePayload := network.GameStateUpdateUDP{
				GameTimeRemainingSeconds: int(timeRemaining),
				Player1Mana:              gs.Player1.CurrentMana,
				Player2Mana:              gs.Player2.CurrentMana,
				Towers:                   towersForState,       // Use updated list
				ActiveTroops:             activeTroopsForState, // Use updated map
			}

			seq := uint32(time.Now().UnixNano())

			playerTokens := []string{gs.Player1.SessionToken, gs.Player2.SessionToken}

			for _, token := range playerTokens {
				if addr, ok := gs.playerClientAddresses[token]; ok {
					msgForPlayer := network.UDPMessage{
						Seq:         seq,
						Timestamp:   time.Now(),
						SessionID:   gs.ID,
						PlayerToken: token,
						Type:        network.UDPMsgTypeGameStateUpdate,
						Payload:     gameStateUpdatePayload,
					}
					gs.sendUDPMessageToAddress(msgForPlayer, addr)
				} else {
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
			// Try to unmarshal directly if not a map (e.g., if client sends the struct directly)
			payloadBytes, err := json.Marshal(msg.Payload)
			if err != nil {
				log.Printf("[GameSession %s] Error marshalling payload to bytes for DeployTroop: %v", gs.ID, err)
				return
			}
			if err := json.Unmarshal(payloadBytes, &deployPayload); err != nil {
				log.Printf("[GameSession %s] Error unmarshalling DeployTroopCommandUDP from payload bytes: %v", gs.ID, err)
				log.Printf("[GameSession %s] Received payload: %s", gs.ID, string(payloadBytes))
				return
			}
		} else { // Original logic for map[string]interface{}
			troopIDInterface, idOk := payloadMap["troop_id"]
			if !idOk {
				log.Printf("[GameSession %s] 'troop_id' not found in DeployTroop payload: %+v", gs.ID, payloadMap)
				return
			}
			troopID, troopIDStrOk := troopIDInterface.(string)
			if !troopIDStrOk {
				log.Printf("[GameSession %s] 'troop_id' is not a string in DeployTroop payload: %+v", gs.ID, payloadMap)
				return
			}
			deployPayload.TroopID = troopID
		}

		// Determine which player is deploying
		var deployingPlayer *models.PlayerInGame
		var opponentPlayer *models.PlayerInGame // For context if needed later

		if msg.PlayerToken == gs.Player1.SessionToken {
			deployingPlayer = gs.Player1
			opponentPlayer = gs.Player2
		} else if msg.PlayerToken == gs.Player2.SessionToken {
			deployingPlayer = gs.Player2
			opponentPlayer = gs.Player1
		} else {
			log.Printf("[GameSession %s] DeployTroop command from unknown or mismatched token: %s", gs.ID, msg.PlayerToken)
			return
		}

		// Log a more specific message if player object is nil
		if deployingPlayer == nil {
			log.Printf("[GameSession %s] Deploying player could not be determined for token: %s", gs.ID, msg.PlayerToken)
			return
		}
		if opponentPlayer == nil { // Should not happen if deployingPlayer is set
			log.Printf("[GameSession %s] Opponent player could not be determined for deploying player with token: %s", gs.ID, msg.PlayerToken)
			// Potentially return or handle as a single player context if that's ever supported
		}

		// Get TroopSpec from config
		troopSpec, ok := gs.Config.Troops[deployPayload.TroopID]
		if !ok {
			log.Printf("[GameSession %s] Player %s tried to deploy unknown troop type: %s", gs.ID, deployingPlayer.Account.Username, deployPayload.TroopID)
			gs.sendGameEventToPlayer(deployingPlayer.SessionToken, network.GameEventError, map[string]interface{}{"message": "Unknown troop type: " + deployPayload.TroopID})
			return
		}

		// Check Mana Cost
		if deployingPlayer.CurrentMana < troopSpec.ManaCost {
			log.Printf("[GameSession %s] Player %s not enough mana to deploy %s (Cost: %d, Has: %d)", gs.ID, deployingPlayer.Account.Username, troopSpec.Name, troopSpec.ManaCost, deployingPlayer.CurrentMana)
			gs.sendGameEventToPlayer(deployingPlayer.SessionToken, network.GameEventError, map[string]interface{}{"message": fmt.Sprintf("Not enough mana for %s. Need %d, have %d", troopSpec.Name, troopSpec.ManaCost, deployingPlayer.CurrentMana)})
			return
		}

		// Deduct Mana
		deployingPlayer.CurrentMana -= troopSpec.ManaCost

		// Handle Queen's special ability
		if strings.ToLower(troopSpec.ID) == "queen" {
			healAmount := 300 // As per plan
			healMsg, healedTower, actualHeal, err := game.ApplyQueenHeal(deployingPlayer.Account.Username, gs.toModelGameSession(), healAmount)
			if err != nil {
				log.Printf("[GameSession %s] Error applying Queen heal for %s: %v", gs.ID, deployingPlayer.Account.Username, err)
				gs.sendGameEventToPlayer(deployingPlayer.SessionToken, network.GameEventError, map[string]interface{}{"message": "Queen heal failed."})
			} else {
				log.Printf("[GameSession %s] %s", gs.ID, healMsg)
				eventDetails := map[string]interface{}{
					"player_id": deployingPlayer.Account.Username,
					"message":   healMsg,
				}
				if healedTower != nil {
					eventDetails["tower_id"] = healedTower.GameSpecificID
					eventDetails["tower_spec"] = healedTower.SpecID
					eventDetails["healed_amount"] = actualHeal
					eventDetails["new_hp"] = healedTower.CurrentHP
				}
				gs.sendGameEventToAllPlayers(network.GameEventQueenHeal, eventDetails)
			}
			// Queen does not persist on board, so we don't add to ActiveTroops
		} else {
			// Create and add the new troop
			// Calculate stat multiplier based on player level
			levelMultiplier := 1.0
			if deployingPlayer.Account.Level > 1 {
				for i := 1; i < deployingPlayer.Account.Level; i++ {
					levelMultiplier *= 1.1
				}
			}

			newTroopInstanceID := fmt.Sprintf("%s_troop_%d", deployingPlayer.Account.Username, time.Now().UnixNano())
			activeTroop := &models.ActiveTroop{
				InstanceID: newTroopInstanceID,
				SpecID:     troopSpec.ID,
				OwnerID:    deployingPlayer.Account.Username,
				CurrentHP:  int(float64(troopSpec.BaseHP) * levelMultiplier),
				MaxHP:      int(float64(troopSpec.BaseHP) * levelMultiplier),
				CurrentATK: int(float64(troopSpec.BaseATK) * levelMultiplier),
				CurrentDEF: int(float64(troopSpec.BaseDEF) * levelMultiplier), // Though troops only attack towers
				DeployedAt: time.Now(),
				// TargetID will be set by the attack logic
			}
			deployingPlayer.DeployedTroops[newTroopInstanceID] = activeTroop
			gs.activeTroops[newTroopInstanceID] = activeTroop   // Add to centralized map
			gs.lastTroopAttack[newTroopInstanceID] = time.Now() // Initialize attack timer

			log.Printf("[GameSession %s] Player %s deployed %s (Instance: %s, HP: %d, ATK: %d)",
				gs.ID, deployingPlayer.Account.Username, troopSpec.Name, newTroopInstanceID, activeTroop.CurrentHP, activeTroop.CurrentATK)
			gs.sendGameEventToAllPlayers(network.GameEventTroopDeployed, map[string]interface{}{
				"player_id":   deployingPlayer.Account.Username,
				"troop_id":    newTroopInstanceID,
				"troop_spec":  troopSpec.ID,
				"owner_id":    deployingPlayer.Account.Username,
				"current_hp":  activeTroop.CurrentHP,
				"max_hp":      activeTroop.MaxHP,
				"current_atk": activeTroop.CurrentATK,
			})
		}
		// After handling deployment, immediately send a game state update to reflect mana change and new troop/heal.
		// This can be done by falling through, or explicitly calling a send state function if extracted.
		// The main loop will send an update soon anyway with the ticker.

	case "basic_ping": // Handling basic_ping to avoid unhandled message log
		log.Printf("[GameSession %s] Received basic_ping from PlayerToken %s. Acknowledged.", gs.ID, msg.PlayerToken)
		// Optionally, send a pong back or just ignore after logging.
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

// Helper function to convert GameSession to models.GameSession for game logic functions
func (gs *GameSession) toModelGameSession() *models.GameSession {
	// This is a shallow copy. Be careful if game logic functions modify slices/maps directly
	// without understanding they are modifying gs.Player1/Player2's actual data.
	// The game logic functions in `internal/game` are designed to operate on pointers from these.
	return &models.GameSession{
		SessionID:  gs.ID,
		Player1:    gs.Player1,
		Player2:    gs.Player2,
		GameConfig: &gs.Config,
		// ActiveTroops and Towers are not directly on models.GameSession,
		// but accessed via Player1.DeployedTroops, Player1.Towers, Player2.DeployedTroops, Player2.Towers
		// The logic functions were updated to use these paths.
		// However, for FindLowestHPTower and FindTroopToAttack, they might need a way
		// to see all towers or all troops.
		// Let's reconstruct the full list of towers and troops for the model
		// This means the model used by game logic functions needs to be models.GameSession from the models package.
		// The game logic functions (FindLowestHPTower, FindTroopToAttack) take *models.GameSession
		// and they correctly access player1.Towers, player2.Towers, player1.DeployedTroops, player2.DeployedTroops.
		// No need to populate AllActiveTroops or AllTowers directly on this temporary model object,
		// as the functions in `internal/game` will iterate through player specific fields.
	}
}

// Helper to get player by username
func (gs *GameSession) getPlayerByUsername(username string) *models.PlayerInGame {
	if gs.Player1.Account.Username == username {
		return gs.Player1
	}
	if gs.Player2.Account.Username == username {
		return gs.Player2
	}
	return nil
}
