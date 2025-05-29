package client

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"

	"enhanced-tcr-udp/internal/network"
	// "enhanced-tcr-udp/internal/models" // For game state model access if needed directly here
)

// Handles incoming TCP/UDP messages

// ListenForUDPMessages continuously listens for incoming UDP messages from the server.
// It should be run in a goroutine.
func (c *Client) ListenForUDPMessages() {
	if c.UDPConn == nil {
		log.Println("UDP connection is not established. Cannot listen for UDP messages.")
		return
	}
	log.Println("Starting to listen for UDP messages from server...")

	buffer := make([]byte, 2048) // Adjust buffer size as needed for expected message sizes

	for {
		n, _, err := c.UDPConn.ReadFromUDP(buffer) // Can use Read() since we used DialUDP
		if err != nil {
			// Check if the error is due to the connection being closed
			// This can happen when the client is shutting down or the connection is intentionally closed
			netErr, ok := err.(net.Error)
			if ok && netErr.Timeout() {
				log.Println("UDP read timeout. Continuing to listen...")
				continue
			}
			if strings.Contains(err.Error(), "use of closed network connection") {
				log.Println("UDP connection closed. Stopping listener.")
				return // Exit goroutine
			}
			log.Printf("Error reading from UDP: %v. Listener might stop.", err)
			return // Or handle error more gracefully, e.g. attempt to re-establish for some errors
		}

		var udpMsg network.UDPMessage
		if err := json.Unmarshal(buffer[:n], &udpMsg); err != nil {
			log.Printf("Error unmarshalling UDP message: %v. Raw: %s", err, string(buffer[:n]))
			continue
		}

		// Log the raw message type for now
		// log.Printf("Received UDP PDU: Type=%s, SessionID=%s, PlayerToken=%s, Seq=%d",
		// 	udpMsg.Type, udpMsg.SessionID, udpMsg.PlayerToken, udpMsg.Seq)

		switch udpMsg.Type {
		case network.UDPMsgTypeGameStateUpdate:
			c.handleGameStateUpdate(udpMsg.Payload)
		case network.UDPMsgTypeGameEvent:
			var gameEventPayload network.GameEventUDP
			payloadMap, ok := udpMsg.Payload.(map[string]interface{})
			if !ok {
				log.Printf("Error: GameEvent payload is not map[string]interface{}. Type: %T", udpMsg.Payload)
				continue
			}
			payloadBytes, err := json.Marshal(payloadMap)
			if err != nil {
				log.Printf("Error re-marshalling GameEvent payload: %v", err)
				continue
			}
			if err := json.Unmarshal(payloadBytes, &gameEventPayload); err != nil {
				log.Printf("Error unmarshalling GameEventUDP payload: %v. Raw: %s", err, string(payloadBytes))
				continue
			}

			log.Printf("Client %s received Game Event: Type=%s, Details=%v", c.PlayerAccount.Username, gameEventPayload.EventType, gameEventPayload.Details)

			// Format and add to UI event log
			if c.ui != nil {
				message := ""
				// Ensure detailsMap is initialized even if details are nil to prevent panic
				var detailsMap map[string]interface{}
				if gameEventPayload.Details != nil {
					detailsMap, _ = gameEventPayload.Details.(map[string]interface{})
				} else {
					detailsMap = make(map[string]interface{}) // Initialize to empty map
				}

				switch gameEventPayload.EventType {
				case network.GameEventTroopDeployed:
					playerID, _ := detailsMap["player_id"].(string)
					troopSpecID, _ := detailsMap["troop_spec"].(string)
					if playerID == c.PlayerAccount.Username {
						message = fmt.Sprintf("You deployed %s.", troopSpecID)
					} else {
						message = fmt.Sprintf("Opponent deployed %s.", troopSpecID)
					}
				case network.GameEventQueenHeal:
					msgFromServer, _ := detailsMap["message"].(string)
					if msgFromServer != "" {
						message = msgFromServer // Use the pre-formatted message from server
					} else {
						playerID, _ := detailsMap["player_id"].(string)
						towerSpecID, _ := detailsMap["tower_spec"].(string)
						healedAmount, _ := detailsMap["healed_amount"].(float64) // JSON numbers are float64
						newHP, _ := detailsMap["new_hp"].(float64)
						if playerID == c.PlayerAccount.Username {
							message = fmt.Sprintf("Your Queen healed tower %s for %.0f HP (now %.0f).", towerSpecID, healedAmount, newHP)
						} else {
							message = fmt.Sprintf("Opponent's Queen healed tower %s for %.0f HP (now %.0f).", towerSpecID, healedAmount, newHP)
						}
					}
				case network.GameEventTowerDamaged:
					attackerSpec, _ := detailsMap["attacker_spec"].(string)
					defenderSpec, _ := detailsMap["defender_spec"].(string)
					damage, _ := detailsMap["damage"].(float64)
					newHP, _ := detailsMap["new_hp"].(float64)
					message = fmt.Sprintf("%s damaged %s for %.0f! (HP: %.0f)", attackerSpec, defenderSpec, damage, newHP)
				case network.GameEventTroopDamaged:
					attackerSpec, _ := detailsMap["attacker_spec"].(string)
					defenderSpec, _ := detailsMap["defender_spec"].(string)
					damage, _ := detailsMap["damage"].(float64)
					newHP, _ := detailsMap["new_hp"].(float64)
					message = fmt.Sprintf("%s damaged %s for %.0f! (HP: %.0f)", attackerSpec, defenderSpec, damage, newHP)
				case network.GameEventTowerDestroyed:
					towerSpec, _ := detailsMap["tower_spec"].(string)
					destroyerTroopSpec, _ := detailsMap["destroyed_by_troop_id"].(string) // This might be troop instance ID or spec based on server
					message = fmt.Sprintf("Tower %s DESTROYED by %s!", towerSpec, destroyerTroopSpec)
				case network.GameEventTroopDefeated:
					troopSpec, _ := detailsMap["troop_spec"].(string)
					defeatedByTowerSpec, _ := detailsMap["defeated_by_tower_id"].(string) // This might be tower instance ID or spec
					message = fmt.Sprintf("Troop %s DEFEATED by %s!", troopSpec, defeatedByTowerSpec)
				case network.GameEventCritHit:
					attackerSpec, _ := detailsMap["attacker_spec"].(string)
					defenderSpec, _ := detailsMap["defender_spec"].(string)
					damage, _ := detailsMap["damage"].(float64)
					message = fmt.Sprintf("CRITICAL HIT! %s smashes %s for %.0f damage!", attackerSpec, defenderSpec, damage)
				case network.GameEventError: // Display errors sent by server
					errorMsg, _ := detailsMap["message"].(string)
					message = fmt.Sprintf("Server Error: %s", errorMsg)
				case "DeployFailed": // Legacy, consider replacing with GameEventError
					reason, _ := detailsMap["reason"].(string)
					message = fmt.Sprintf("Deployment failed: %s", reason)
				default:
					message = fmt.Sprintf("Event: %s - %v", gameEventPayload.EventType, gameEventPayload.Details)
				}
				if message != "" {
					c.ui.AddEventMessage(message)
					c.ui.Render() // Re-render immediately after adding an event message
				}
			}
		default:
			log.Printf("Received unknown UDP message type: %s", udpMsg.Type)
		}
	}
}

func (c *Client) handleGameStateUpdate(payload interface{}) {
	// The payload from UDPMessage is interface{}. We need to assert it to the correct type.
	// One way is to remarshal and unmarshal, or use map[string]interface{}.
	// For more type safety, remarshaling is often cleaner if performance isn't critical.
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshaling GameStateUpdate payload: %v", err)
		return
	}

	var updateData network.GameStateUpdateUDP
	if err := json.Unmarshal(payloadBytes, &updateData); err != nil {
		log.Printf("Error unmarshalling GameStateUpdateUDP: %v", err)
		return
	}

	// log.Printf("Game State Update: Time Left: %ds, P1 Mana: %d, P2 Mana: %d",
	// 	updateData.GameTimeRemainingSeconds, updateData.Player1Mana, updateData.Player2Mana)

	if c.ui != nil {
		// Determine which mana belongs to this client
		myMana := 0
		opponentMana := 0
		if c.IsPlayerOne { // Assuming c.IsPlayerOne is set based on MatchFoundResponse
			myMana = updateData.Player1Mana
			opponentMana = updateData.Player2Mana
		} else {
			myMana = updateData.Player2Mana
			opponentMana = updateData.Player1Mana
		}

		c.ui.UpdateGameInfo(
			updateData.GameTimeRemainingSeconds,
			myMana,
			opponentMana,
			updateData.ActiveTroops,
			updateData.Towers,
		)
		// TODO: Update towers and troops in UI (Sprint 2/3) - This is now done by passing troops/towers to UpdateGameInfo
		c.ui.Render() // Re-render the UI with new information
	} else {
		// Fallback for non-UI or headless mode if ever needed
		log.Printf("Received GameStateUpdate: Timer=%d, P1_Mana=%d", updateData.GameTimeRemainingSeconds, updateData.Player1Mana)
	}
	// TODO: Further process the game state, update local client model, etc.
}
