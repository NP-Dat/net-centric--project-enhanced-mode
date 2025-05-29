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
	gameWinner      *models.PlayerInGame           // Stores the winner of the game
	gameResult      string                         // e.g., "win", "loss", "draw"
	isGameOver      bool                           // Flag to indicate if the game has concluded
	resultsChan     chan<- network.GameResultInfo  // Channel to send game results back

	processedDeployCommands map[string]map[uint32]time.Time // PlayerToken -> Seq -> ProcessTime
}

// NewGameSession creates a new game session.
func NewGameSession(id string, p1Acc, p2Acc *models.PlayerAccount, p1Token, p2Token string, udpPort int, resultsChan chan<- network.GameResultInfo) *GameSession {
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
		ID:                      id,
		Player1:                 &models.PlayerInGame{Account: *p1Acc, SessionToken: p1Token, CurrentMana: 5, DeployedTroops: make(map[string]*models.ActiveTroop), Towers: make([]*models.TowerInstance, 0)},
		Player2:                 &models.PlayerInGame{Account: *p2Acc, SessionToken: p2Token, CurrentMana: 5, DeployedTroops: make(map[string]*models.ActiveTroop), Towers: make([]*models.TowerInstance, 0)},
		Config:                  gameCfg,
		udpPort:                 udpPort,
		startTime:               startTime,
		gameEndTime:             startTime.Add(3 * time.Minute),
		playerActions:           make(chan network.UDPMessage, 10),
		playerClientAddresses:   make(map[string]*net.UDPAddr),
		lastManaRegen:           startTime,
		lastTroopAttack:         make(map[string]time.Time),
		lastTowerAttack:         make(map[string]time.Time),
		activeTroops:            make(map[string]*models.ActiveTroop), // Initialize centralized map
		towers:                  make([]*models.TowerInstance, 0),     // Initialize centralized list
		gameWinner:              nil,
		gameResult:              "",
		isGameOver:              false,
		resultsChan:             resultsChan,
		processedDeployCommands: make(map[string]map[uint32]time.Time),
	}

	// Initialize processedDeployCommands for each player
	gs.processedDeployCommands[p1Token] = make(map[uint32]time.Time)
	gs.processedDeployCommands[p2Token] = make(map[uint32]time.Time)

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

	ticker := time.NewTicker(500 * time.Millisecond) // Tick more frequently for responsiveness
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			gs.mu.Lock()
			if gs.isGameOver {
				gs.mu.Unlock()
				// gs.Stop() // Stop is handled by determineWinnerAndStop
				return
			}

			if time.Now().After(gs.gameEndTime) {
				log.Printf("[GameSession %s] Timer ended.", gs.ID)
				gs.determineWinnerAndStop("timeout")
				gs.mu.Unlock()
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
								// Check for King Tower destruction for instant win
								if gs.isKingTower(targetTower) {
									log.Printf("[GameSession %s] King Tower %s DESTROYED! Determining winner.", gs.ID, targetTower.GameSpecificID)
									gs.determineWinnerAndStop("king_tower_destroyed")
									gs.mu.Unlock() // ensure unlock before return
									return
								}
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

			gs.sendGameStateToAllPlayers()
			gs.mu.Unlock()

		case action := <-gs.playerActions:
			gs.mu.Lock()
			if !gs.isGameOver { // Process actions only if game is not over
				gs.handlePlayerAction(action)
			}
			// After handling action, check if game ended due to it (e.g., Queen heal on a King Tower might be a win if it was the last action)
			// This might be redundant if handlePlayerAction itself can trigger a game end check.
			// However, for now, we rely on the main loop's tower destruction checks.
			gs.mu.Unlock()

		case <-time.After(5 * time.Second): // Timeout for player actions if channel is empty
			// This case helps prevent the select from blocking indefinitely if no actions or ticks occur.
			// Potentially log this if it happens too often, might indicate an issue.
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
		// Check if this command sequence from this player has already been processed.
		if _, processed := gs.processedDeployCommands[msg.PlayerToken][msg.Seq]; processed {
			log.Printf("[GameSession %s] Player %s: Duplicate DeployTroop command (Seq: %d) received. Ignoring and resending ACK.", gs.ID, msg.PlayerToken, msg.Seq)
			// Resend ACK just in case the first one was lost
			ackPayload := network.CommandAckUDP{AckSeq: msg.Seq}
			clientAddr, addrOk := gs.playerClientAddresses[msg.PlayerToken]
			if addrOk && clientAddr != nil {
				gs.sendUDPMessageToAddress(network.UDPMessage{
					Type:        network.UDPMsgTypeCommandAck,
					SessionID:   gs.ID,           // Important for client to validate
					PlayerToken: msg.PlayerToken, // Echo back player token
					Seq:         0,               // ACKs themselves don't need sequence numbers for this simple ACK system
					Timestamp:   time.Now(),
					Payload:     ackPayload,
				}, clientAddr)
			} else {
				log.Printf("[GameSession %s] Player %s: Could not resend ACK for Seq %d, client address unknown.", gs.ID, msg.PlayerToken, msg.Seq)
			}
			return
		}

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

				// Record processed command and send ACK for Queen deployment
				gs.processedDeployCommands[msg.PlayerToken][msg.Seq] = time.Now()
				ackPayload := network.CommandAckUDP{AckSeq: msg.Seq}
				clientAddr, addrOk := gs.playerClientAddresses[msg.PlayerToken]
				if addrOk && clientAddr != nil {
					gs.sendUDPMessageToAddress(network.UDPMessage{
						Type:        network.UDPMsgTypeCommandAck,
						SessionID:   gs.ID,
						PlayerToken: msg.PlayerToken,
						Seq:         0, // ACK specific seq
						Timestamp:   time.Now(),
						Payload:     ackPayload,
					}, clientAddr)
					log.Printf("[GameSession %s] Player %s: Sent ACK for Queen Deploy (Seq: %d)", gs.ID, msg.PlayerToken, msg.Seq)
				} else {
					log.Printf("[GameSession %s] Player %s: Could not send ACK for Queen Deploy (Seq: %d), client address unknown.", gs.ID, msg.PlayerToken, msg.Seq)
				}
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

			// Record processed command and send ACK for normal troop deployment
			gs.processedDeployCommands[msg.PlayerToken][msg.Seq] = time.Now()
			ackPayload := network.CommandAckUDP{AckSeq: msg.Seq}
			clientAddr, addrOk := gs.playerClientAddresses[msg.PlayerToken]
			if addrOk && clientAddr != nil {
				gs.sendUDPMessageToAddress(network.UDPMessage{
					Type:        network.UDPMsgTypeCommandAck,
					SessionID:   gs.ID,
					PlayerToken: msg.PlayerToken,
					Seq:         0, // ACK specific seq
					Timestamp:   time.Now(),
					Payload:     ackPayload,
				}, clientAddr)
				log.Printf("[GameSession %s] Player %s: Sent ACK for Troop Deploy %s (Seq: %d)", gs.ID, msg.PlayerToken, troopSpec.Name, msg.Seq)
			} else {
				log.Printf("[GameSession %s] Player %s: Could not send ACK for Troop Deploy %s (Seq: %d), client address unknown.", gs.ID, msg.PlayerToken, troopSpec.Name, msg.Seq)
			}
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

// Stop ends the game session, closes connections, and notifies the manager.
func (gs *GameSession) Stop() {
	log.Printf("Game session %s stopped.", gs.ID)
	if gs.udpConn != nil {
		gs.udpConn.Close()
	}
	// TODO: Persist player EXP/level changes, notify SessionManager to remove session.
}

// setupUDPConnectionAndListener sets up the UDP listener for this game session.
func (gs *GameSession) setupUDPConnectionAndListener() error {
	if gs.udpConn != nil {
		gs.udpConn.Close() // Close existing connection if any before setting up new
	}
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

// isKingTower checks if a given tower is a King Tower.
func (gs *GameSession) isKingTower(tower *models.TowerInstance) bool {
	// Assuming King Tower can be identified by its SpecID or Name.
	// Let's primarily use SpecID for robustness.
	spec, ok := gs.Config.Towers[tower.SpecID]
	if !ok {
		log.Printf("[GameSession %s] Warning: Could not find tower spec for ID %s to check if King Tower.", gs.ID, tower.SpecID)
		return false // Or handle as an error
	}
	return spec.Name == "King Tower" // Or check spec.ID == "king_tower"
}

// determineWinnerAndStop evaluates win conditions and stops the game.
// reason: "timeout", "king_tower_destroyed", "player_quit"
func (gs *GameSession) determineWinnerAndStop(reason string) {
	if gs.isGameOver { // Prevent multiple calls
		return
	}
	gs.isGameOver = true // Mark game as over immediately
	log.Printf("[GameSession %s] Determining winner due to: %s", gs.ID, reason)

	var winner *models.PlayerInGame
	var resultPlayer1, resultPlayer2 string // "win", "loss", "draw"
	var p1ExpEarned, p2ExpEarned int

	switch reason {
	case "king_tower_destroyed":
		// The player whose King Tower is NOT destroyed is the winner.
		// We need to find out which King Tower was destroyed.
		// The call to this function happens right after a tower is destroyed.
		// The 'defender' in that context lost their King Tower.
		// Let's iterate through towers to be certain.
		p1KingDestroyed := false
		p2KingDestroyed := false
		for _, tower := range gs.towers {
			if gs.isKingTower(tower) && tower.IsDestroyed {
				if tower.OwnerID == gs.Player1.Account.Username {
					p1KingDestroyed = true
				} else if tower.OwnerID == gs.Player2.Account.Username {
					p2KingDestroyed = true
				}
			}
		}

		if p1KingDestroyed && !p2KingDestroyed {
			winner = gs.Player2
			gs.gameWinner = gs.Player2
			gs.gameResult = fmt.Sprintf("%s won (King Tower)", gs.Player2.Account.Username)
			resultPlayer1 = "loss"
			resultPlayer2 = "win"
		} else if p2KingDestroyed && !p1KingDestroyed {
			winner = gs.Player1
			gs.gameWinner = gs.Player1
			gs.gameResult = fmt.Sprintf("%s won (King Tower)", gs.Player1.Account.Username)
			resultPlayer1 = "win"
			resultPlayer2 = "loss"
		} else {
			// This case (both or neither king tower destroyed by this specific event) should ideally not happen
			// if called correctly. Or could be a simultaneous destruction? For now, treat as a draw.
			log.Printf("[GameSession %s] Ambiguous King Tower destruction state (p1King: %v, p2King: %v). Declaring draw.", gs.ID, p1KingDestroyed, p2KingDestroyed)
			gs.gameResult = "Draw (Simultaneous King Tower Destruction or Error)"
			resultPlayer1 = "draw"
			resultPlayer2 = "draw"
		}

	case "timeout":
		p1TowersDestroyed := 0
		p2TowersDestroyed := 0
		for _, tower := range gs.towers {
			if tower.IsDestroyed {
				if tower.OwnerID == gs.Player1.Account.Username { // This tower belonged to P1, so P2 destroyed it.
					p2TowersDestroyed++
				} else if tower.OwnerID == gs.Player2.Account.Username { // This tower belonged to P2, so P1 destroyed it.
					p1TowersDestroyed++
				}
			}
		}
		log.Printf("[GameSession %s] Timeout: Player 1 destroyed %d towers, Player 2 destroyed %d towers.", gs.ID, p1TowersDestroyed, p2TowersDestroyed)
		if p1TowersDestroyed > p2TowersDestroyed {
			winner = gs.Player1
			gs.gameWinner = gs.Player1
			gs.gameResult = fmt.Sprintf("%s won (Most Towers)", gs.Player1.Account.Username)
			resultPlayer1 = "win"
			resultPlayer2 = "loss"
		} else if p2TowersDestroyed > p1TowersDestroyed {
			winner = gs.Player2
			gs.gameWinner = gs.Player2
			gs.gameResult = fmt.Sprintf("%s won (Most Towers)", gs.Player2.Account.Username)
			resultPlayer1 = "loss"
			resultPlayer2 = "win"
		} else {
			gs.gameResult = "Draw (Equal Towers Destroyed)"
			resultPlayer1 = "draw"
			resultPlayer2 = "draw"
		}
	case "player_quit":
		// Determine which player did not quit
		if gs.player1Quit && !gs.player2Quit {
			winner = gs.Player2
			gs.gameWinner = gs.Player2
			gs.gameResult = fmt.Sprintf("%s won (Opponent Quit)", gs.Player2.Account.Username)
			resultPlayer1 = "loss" // The quitter loses
			resultPlayer2 = "win"
		} else if gs.player2Quit && !gs.player1Quit {
			winner = gs.Player1
			gs.gameWinner = gs.Player1
			gs.gameResult = fmt.Sprintf("%s won (Opponent Quit)", gs.Player1.Account.Username)
			resultPlayer1 = "win"
			resultPlayer2 = "loss" // The quitter loses
		} else {
			// Both quit or some other state, declare as draw or handle as needed
			gs.gameResult = "Draw (Both Players Quit or Undetermined)"
			resultPlayer1 = "draw"
			resultPlayer2 = "draw"
			log.Printf("[GameSession %s] Both players quit or quit state unclear. Declaring draw.", gs.ID)
		}

	default:
		log.Printf("[GameSession %s] Unknown game end reason: %s. Declaring draw.", gs.ID, reason)
		gs.gameResult = "Draw (Unknown Reason)"
		resultPlayer1 = "draw"
		resultPlayer2 = "draw"
	}

	// Calculate EXP from destroyed towers
	for _, tower := range gs.towers {
		if tower.IsDestroyed {
			towerSpec, ok := gs.Config.Towers[tower.SpecID]
			if !ok {
				log.Printf("[GameSession %s] Warning: Could not find spec for destroyed tower %s (ID: %s) for EXP calculation.", gs.ID, tower.GameSpecificID, tower.SpecID)
				continue
			}
			// If Player1's tower was destroyed, Player2 gets EXP
			if tower.OwnerID == gs.Player1.Account.Username {
				p2ExpEarned += towerSpec.EXPYield
			} else if tower.OwnerID == gs.Player2.Account.Username {
				p1ExpEarned += towerSpec.EXPYield
			}
		}
	}

	// Add win/draw bonus EXP
	if resultPlayer1 == "win" {
		p1ExpEarned += 30 // Win bonus from plan
	} else if resultPlayer1 == "draw" {
		p1ExpEarned += 10 // Draw bonus from plan
	}

	if resultPlayer2 == "win" {
		p2ExpEarned += 30 // Win bonus
	} else if resultPlayer2 == "draw" {
		p2ExpEarned += 10 // Draw bonus
	}

	log.Printf("[GameSession %s] EXP Earned This Game: %s -> %d, %s -> %d", gs.ID, gs.Player1.Account.Username, p1ExpEarned, gs.Player2.Account.Username, p2ExpEarned)
	// gs.Player1.Account.EXP += p1ExpEarned // This is now handled by UpdatePlayerAfterGame
	// gs.Player2.Account.EXP += p2ExpEarned // This is now handled by UpdatePlayerAfterGame

	p1LeveledUp, errP1 := persistence.UpdatePlayerAfterGame(&gs.Player1.Account, p1ExpEarned)
	if errP1 != nil {
		log.Printf("[GameSession %s] Error updating player %s data: %v", gs.ID, gs.Player1.Account.Username, errP1)
	}
	p2LeveledUp, errP2 := persistence.UpdatePlayerAfterGame(&gs.Player2.Account, p2ExpEarned)
	if errP2 != nil {
		log.Printf("[GameSession %s] Error updating player %s data: %v", gs.ID, gs.Player2.Account.Username, errP2)
	}

	if p1LeveledUp {
		log.Printf("[GameSession %s] Player %s leveled up to Level %d!", gs.ID, gs.Player1.Account.Username, gs.Player1.Account.Level)
	}
	if p2LeveledUp {
		log.Printf("[GameSession %s] Player %s leveled up to Level %d!", gs.ID, gs.Player2.Account.Username, gs.Player2.Account.Level)
	}

	if winner != nil {
		log.Printf("[GameSession %s] Game ended. Winner: %s. Result: %s", gs.ID, winner.Account.Username, gs.gameResult)
	} else {
		log.Printf("[GameSession %s] Game ended. Result: %s", gs.ID, gs.gameResult)
	}

	// TODO: Sprint 5: Calculate EXP for Player1 -> DONE
	// TODO: Sprint 5: Calculate EXP for Player2 -> DONE
	// TODO: Sprint 5: Persist Player1 and Player2 data (including new EXP/Level) -> DONE
	// TODO: Sprint 5: Send game_over_results message via TCP to both clients -> To be done by receiver of resultsChan

	// Construct GameResultInfo
	resultInfo := network.GameResultInfo{
		SessionID:       gs.ID,
		Player1Username: gs.Player1.Account.Username,
		Player2Username: gs.Player2.Account.Username,
		GameEndReason:   reason,
	}
	if gs.gameWinner != nil {
		resultInfo.OverallWinnerID = gs.gameWinner.Account.Username
	}

	// Player 1 results
	resultInfo.Player1Result = network.GameOverResults{
		WinnerID:  resultInfo.OverallWinnerID,
		Outcome:   resultPlayer1, // "win", "loss", "draw"
		EXPChange: p1ExpEarned,
		NewEXP:    gs.Player1.Account.EXP,
		NewLevel:  gs.Player1.Account.Level,
		LevelUp:   p1LeveledUp,
		// DestroyedTowers: populated below
	}

	// Player 2 results
	resultInfo.Player2Result = network.GameOverResults{
		WinnerID:  resultInfo.OverallWinnerID,
		Outcome:   resultPlayer2, // "win", "loss", "draw"
		EXPChange: p2ExpEarned,
		NewEXP:    gs.Player2.Account.EXP,
		NewLevel:  gs.Player2.Account.Level,
		LevelUp:   p2LeveledUp,
		// DestroyedTowers: populated below
	}

	// Populate DestroyedTowers for each player
	// p1DestroyedCount is towers P1 destroyed (owned by P2)
	// p2DestroyedCount is towers P2 destroyed (owned by P1)
	p1DestroyedCount := 0
	p2DestroyedCount := 0
	for _, tower := range gs.towers {
		if tower.IsDestroyed {
			if tower.OwnerID == gs.Player2.Account.Username { // P2's tower destroyed by P1
				p1DestroyedCount++
			} else if tower.OwnerID == gs.Player1.Account.Username { // P1's tower destroyed by P2
				p2DestroyedCount++
			}
		}
	}
	resultInfo.Player1Result.DestroyedTowers = map[string]int{gs.Player2.Account.Username: p1DestroyedCount} // Towers P1 destroyed (belonging to P2)
	resultInfo.Player2Result.DestroyedTowers = map[string]int{gs.Player1.Account.Username: p2DestroyedCount} // Towers P2 destroyed (belonging to P1)

	if gs.resultsChan != nil {
		select {
		case gs.resultsChan <- resultInfo:
			log.Printf("[GameSession %s] Sent game results to results channel.", gs.ID)
		case <-time.After(2 * time.Second): // Timeout to prevent blocking indefinitely
			log.Printf("[GameSession %s] Timeout sending game results to results channel.", gs.ID)
		}
		// close(gs.resultsChan) // The receiver should decide when to close if it's long-lived, or if it's one-shot, this is fine.
		// For now, assume the receiver handles its lifecycle.
	} else {
		log.Printf("[GameSession %s] resultsChan is nil. Cannot send game results.", gs.ID)
	}

	// Send final game state update, possibly indicating game over
	gs.sendGameStateToAllPlayers() // Ensure clients get one last update

	// Actual stopping of the session (closing UDP conn, removing from manager)
	gs.Stop() // Call the original Stop method to clean up resources
}

// sendGameStateToAllPlayers sends a game state update to all players in the session.
func (gs *GameSession) sendGameStateToAllPlayers() {
	// This is a placeholder. The actual implementation should gather game state data
	// and send it to all connected players.
	// For now, we'll just log the call.

	// log.Printf("[GameSession %s] Sending game state to all players.", gs.ID)
}
