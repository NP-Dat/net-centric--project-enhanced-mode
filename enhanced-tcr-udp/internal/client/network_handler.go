package client

import (
	"encoding/json"
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
			// TODO: Implement handleGameEvent (Sprint 4+)
			log.Printf("Received GameEvent (TODO): %+v", udpMsg.Payload)
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
		c.ui.UpdateGameInfo(updateData.GameTimeRemainingSeconds, updateData.Player1Mana, updateData.Player2Mana)
		// TODO: Update towers and troops in UI (Sprint 2/3)
		c.ui.Render() // Re-render the UI with new information
	} else {
		// Fallback for non-UI or headless mode if ever needed
		log.Printf("Received GameStateUpdate: Timer=%d, P1_Mana=%d", updateData.GameTimeRemainingSeconds, updateData.Player1Mana)
	}
	// TODO: Further process the game state, update local client model, etc.
}
