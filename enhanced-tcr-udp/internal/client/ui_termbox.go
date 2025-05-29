package client

import (
	"enhanced-tcr-udp/internal/models"
	"enhanced-tcr-udp/internal/network" // Added for network.GameOverResults
	"fmt"

	// "log"

	"github.com/nsf/termbox-go"
)

const (
	maxEventLogMessages = 5 // Number of recent event messages to display
)

// UIView defines the different states or screens the UI can be in.
type UIView int

const (
	// ViewLogin might not be explicitly managed by currentView if GetTextInput is blocking
	// and directly handles its drawing until completion.
	ViewLogin UIView = iota
	ViewMatchmaking
	ViewGame
	ViewGameOver
)

// TermboxUI holds state for the termbox interface
type TermboxUI struct {
	gameTimer         int
	myMana            int                           // Renamed from player1Mana for clarity from client's perspective
	opponentMana      int                           // Renamed from player2Mana
	towers            []models.TowerInstance        // All towers in the game state
	activeTroops      map[string]models.ActiveTroop // All active troops
	eventLog          []string                      // To store recent event messages
	inputLine         string
	lastSelectedTroop rune
	client            *Client

	currentView     UIView                  // Current UI state (e.g., game, game over)
	gameOverDetails network.GameOverResults // Stores details for the game over screen
	// TODO: Store TroopSpec (from GameConfig) to display mana costs dynamically
}

// NewTermboxUI creates a new TermboxUI manager.
func NewTermboxUI() *TermboxUI {
	return &TermboxUI{
		activeTroops: make(map[string]models.ActiveTroop),
		towers:       make([]models.TowerInstance, 0),
		eventLog:     make([]string, 0, maxEventLogMessages),
		currentView:  ViewGame, // Default to game view, might be set to login/matchmaking by main flow
	}
}

// SetClient associates the client logic with the UI.
func (ui *TermboxUI) SetClient(c *Client) {
	ui.client = c
}

// SetCurrentView changes the current UI view (e.g., game, game_over).
func (ui *TermboxUI) SetCurrentView(view UIView) {
	// log.Printf("UI View changing from %v to %v", ui.currentView, view)
	ui.currentView = view
	ui.ClearScreen() // Clear screen when view changes
	// ui.Render() // Render immediately after view change - let the main loop control render calls.
}

// SetGameOverDetails stores the results to be displayed on the game over screen.
func (ui *TermboxUI) SetGameOverDetails(results network.GameOverResults) {
	ui.gameOverDetails = results
	// log.Printf("Game over details set in UI: Outcome %s, EXP %d", results.Outcome, results.EXPChange)
}

// Init initializes the termbox screen.
func (ui *TermboxUI) Init() error {
	return termbox.Init()
}

// Close closes the termbox screen.
func (ui *TermboxUI) Close() {
	termbox.Close()
}

// DisplayStaticText draws some static text at given coordinates.
// A more advanced version would take a list of strings or a buffer.
func (ui *TermboxUI) DisplayStaticText(x, y int, text string, fg, bg termbox.Attribute) {
	for i, r := range []rune(text) {
		termbox.SetCell(x+i, y, r, fg, bg)
	}
	termbox.Flush()
}

// UpdateGameInfo updates the game state information to be displayed.
func (ui *TermboxUI) UpdateGameInfo(timer, clientMana, oppMana int, troops map[string]models.ActiveTroop, allTowers []models.TowerInstance) {
	ui.gameTimer = timer
	ui.myMana = clientMana
	ui.opponentMana = oppMana
	ui.activeTroops = troops
	ui.towers = allTowers
}

// AddEventMessage adds a message to the event log.
func (ui *TermboxUI) AddEventMessage(message string) {
	if len(ui.eventLog) >= maxEventLogMessages {
		// Remove the oldest message
		ui.eventLog = ui.eventLog[1:]
	}
	ui.eventLog = append(ui.eventLog, message)
	// It's important to call Render() after adding an event if immediate update is desired.
	// However, typically the main loop calls Render periodically.
	// For critical events, a direct call to ui.Render() might be added here or after the call to AddEventMessage.
}

// displayGameOverScreen renders the game over information.
func (ui *TermboxUI) displayGameOverScreen() {
	// termbox.Clear(termbox.ColorDefault, termbox.ColorDefault) // Clear is handled by Render now
	w, h := termbox.Size()
	y := 1

	title := "--- GAME OVER ---"
	ui.DisplayStaticText((w-len(title))/2, y, title, termbox.ColorYellow, termbox.ColorDefault)
	y += 2

	outcomeMsg := fmt.Sprintf("Outcome: %s", ui.gameOverDetails.Outcome)
	outcomeColor := termbox.ColorWhite
	if ui.gameOverDetails.Outcome == "Win" {
		outcomeColor = termbox.ColorGreen
	} else if ui.gameOverDetails.Outcome == "Loss" {
		outcomeColor = termbox.ColorRed
	} else if ui.gameOverDetails.Outcome == "Draw" {
		outcomeColor = termbox.ColorYellow
	}
	ui.DisplayStaticText(1, y, outcomeMsg, outcomeColor, termbox.ColorDefault)
	y++

	expMsg := fmt.Sprintf("EXP Earned this game: %+d", ui.gameOverDetails.EXPChange)
	ui.DisplayStaticText(1, y, expMsg, termbox.ColorWhite, termbox.ColorDefault)
	y++

	totalExpMsg := fmt.Sprintf("Your Total EXP: %d", ui.gameOverDetails.NewEXP)
	ui.DisplayStaticText(1, y, totalExpMsg, termbox.ColorWhite, termbox.ColorDefault)
	y++

	levelMsg := fmt.Sprintf("Your Level: %d", ui.gameOverDetails.NewLevel)
	if ui.gameOverDetails.LevelUp {
		levelMsg += " (LEVEL UP!)"
		ui.DisplayStaticText(1, y, levelMsg, termbox.ColorMagenta, termbox.ColorDefault)
	} else {
		ui.DisplayStaticText(1, y, levelMsg, termbox.ColorWhite, termbox.ColorDefault)
	}
	y += 2

	// Display who destroyed what, if relevant
	// The current structure of DestroyedTowers in GameOverResults is: map[opponent_username]count_destroyed_by_me
	// We can iterate through this if it's populated.
	if len(ui.gameOverDetails.DestroyedTowers) > 0 {
		for opponent, count := range ui.gameOverDetails.DestroyedTowers {
			destroyedMsg := fmt.Sprintf("You destroyed %d of %s's towers.", count, opponent)
			ui.DisplayStaticText(1, y, destroyedMsg, termbox.ColorCyan, termbox.ColorDefault)
			y++
		}
	}
	y++

	// Instructions to continue
	if y < h-1 {
		instructions := "Press any key to continue..."
		ui.DisplayStaticText(1, y, instructions, termbox.ColorYellow, termbox.ColorDefault)
	} else {
		instructions := "Press any key..."
		ui.DisplayStaticText(1, h-1, instructions, termbox.ColorYellow, termbox.ColorDefault)
	}

	// termbox.Flush() // Flush is handled by Render
}

// Render draws the entire game UI based on current state.
func (ui *TermboxUI) Render() {
	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)

	switch ui.currentView {
	case ViewGame:
		ui.displayGameScreen()
	case ViewGameOver:
		ui.displayGameOverScreen()
	case ViewLogin: // Login screen is handled by GetTextInput calls, may not need explicit render state here.
		ui.DisplayStaticText(1, 1, "Login View (typically handled by input prompts)", termbox.ColorWhite, termbox.ColorDefault)
	case ViewMatchmaking: // Matchmaking screen similarly might be simple text updates.
		ui.DisplayStaticText(1, 1, "Matchmaking View (waiting for match...)", termbox.ColorWhite, termbox.ColorDefault)
	default:
		ui.DisplayStaticText(1, 1, fmt.Sprintf("Error: Unknown UI View (%d)", ui.currentView), termbox.ColorRed, termbox.ColorDefault)
	}
	termbox.Flush()
}

// displayGameScreen renders the main game interface.
func (ui *TermboxUI) displayGameScreen() {
	// termbox.Clear(termbox.ColorDefault, termbox.ColorDefault) // Moved to Render()

	currentY := 1 // Start rendering from Y=1

	// Game Info Area (Top)
	infoLine1 := fmt.Sprintf("Time: %ds | My PlayerID: %s", ui.gameTimer, ui.client.PlayerAccount.Username)
	infoLine2 := fmt.Sprintf("My Mana: %d | Opponent Mana: %d", ui.myMana, ui.opponentMana)
	ui.DisplayStaticText(1, currentY, infoLine1, termbox.ColorWhite, termbox.ColorBlack)
	currentY++
	ui.DisplayStaticText(1, currentY, infoLine2, termbox.ColorWhite, termbox.ColorBlack)
	currentY += 2 // Add some space

	// Display Towers
	towerHeaderY := currentY
	ui.DisplayStaticText(1, towerHeaderY, "--- Towers ---", termbox.ColorYellow, termbox.ColorBlack)
	currentY++
	if len(ui.towers) > 0 {
		myPlayerID := ""
		if ui.client != nil && ui.client.PlayerAccount != nil {
			myPlayerID = ui.client.PlayerAccount.Username
		}
		for _, tower := range ui.towers {
			fgColor := termbox.ColorWhite
			prefix := "Opponent"
			if tower.OwnerID == myPlayerID {
				fgColor = termbox.ColorGreen
				prefix = "My"
			} else {
				fgColor = termbox.ColorRed
			}
			towerInfo := fmt.Sprintf("%s %s (ID: %s): HP %d/%d", prefix, tower.SpecID, tower.GameSpecificID, tower.CurrentHP, tower.MaxHP)
			if tower.IsDestroyed {
				towerInfo += " [DESTROYED]"
				fgColor = termbox.ColorDarkGray // Or some other color to indicate destroyed
			}
			ui.DisplayStaticText(1, currentY, towerInfo, fgColor, termbox.ColorBlack)
			currentY++
		}
	} else {
		ui.DisplayStaticText(1, currentY, "(No tower data yet)", termbox.ColorDefault, termbox.ColorBlack)
		currentY++
	}
	currentY++ // Add some space

	// Display Active Troops
	troopHeaderY := currentY
	ui.DisplayStaticText(1, troopHeaderY, "--- Active Troops ---", termbox.ColorYellow, termbox.ColorBlack)
	currentY++
	if len(ui.activeTroops) > 0 {
		myPlayerID := ""
		if ui.client != nil && ui.client.PlayerAccount != nil {
			myPlayerID = ui.client.PlayerAccount.Username
		}
		for id, troop := range ui.activeTroops {
			fgColor := termbox.ColorWhite
			prefix := "Opponent's"
			if troop.OwnerID == myPlayerID {
				fgColor = termbox.ColorCyan
				prefix = "My"
			} else {
				fgColor = termbox.ColorMagenta
			}
			troopInfo := fmt.Sprintf("%s %s (ID: %s): HP %d/%d, ATK %d", prefix, troop.SpecID, id, troop.CurrentHP, troop.MaxHP, troop.CurrentATK)
			if troop.CurrentHP <= 0 {
				troopInfo += " [DEFEATED]"
				fgColor = termbox.ColorDarkGray // Or some other color
			}
			ui.DisplayStaticText(1, currentY, troopInfo, fgColor, termbox.ColorBlack)
			currentY++
		}
	} else {
		ui.DisplayStaticText(1, currentY, "(No active troops on field)", termbox.ColorDefault, termbox.ColorBlack)
		currentY++
	}
	currentY++ // Add some space

	// Event Log Area
	eventLogHeaderY := currentY
	ui.DisplayStaticText(1, eventLogHeaderY, "--- Event Log ---", termbox.ColorYellow, termbox.ColorBlack)
	currentY++
	logStartY := currentY
	for i, msg := range ui.eventLog {
		if i < maxEventLogMessages { // Ensure we don't try to print too many if log somehow exceeds max
			ui.DisplayStaticText(1, logStartY+i, msg, termbox.ColorWhite, termbox.ColorBlack)
			currentY++
		}
	}
	if len(ui.eventLog) == 0 {
		ui.DisplayStaticText(1, currentY, "(No recent events)", termbox.ColorDefault, termbox.ColorBlack)
		// currentY++ // Don't increment if no messages, let logStartY define the block
	}
	// Ensure currentY is set correctly for prompts below, accounting for the full height of the log area.
	currentY = logStartY + maxEventLogMessages + 1 // +1 for spacing after the designated log area height

	// Input Area (Bottom)
	troopSelectionPromptY := currentY
	troopSelectionPrompt := "Deploy: [1]Pawn(?) [2]Bishop(?) [3]Rook(?) [4]Knight(?) [5]Prince(?) [6]Queen(?). ESC to Deselect."
	ui.DisplayStaticText(1, troopSelectionPromptY, troopSelectionPrompt, termbox.ColorCyan, termbox.ColorBlack)
	selectedMsgY := troopSelectionPromptY + 1
	selectedMsg := "Selected: None"
	if ui.lastSelectedTroop != 0 {
		selectedMsg = fmt.Sprintf("Selected: %c (Press Enter to deploy)", ui.lastSelectedTroop)
	}
	ui.DisplayStaticText(1, selectedMsgY, selectedMsg, termbox.ColorWhite, termbox.ColorBlack)

	// termbox.Flush() // Moved to Render()
}

// ClearScreen clears the termbox screen.
func (ui *TermboxUI) ClearScreen() {
	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
}

// RunSimpleEvacuateLoop runs a basic event loop that waits for Escape key to quit.
// This is a placeholder for a more complex game UI event loop.
// Returns true if the loop was exited via ESC (quit), false otherwise (e.g. error).
func (ui *TermboxUI) RunSimpleEvacuateLoop() bool {
	// ui.DisplayStaticText(1, 1, "Basic Termbox UI Active. Press ESC to quit.", termbox.ColorWhite, termbox.ColorBlack)
	ui.Render() // Initial render of the game screen
	quitRequested := false

mainloop:
	for {
		switch ev := termbox.PollEvent(); ev.Type {
		case termbox.EventKey:
			switch ev.Key {
			case termbox.KeyEsc:
				if ui.lastSelectedTroop != 0 {
					ui.lastSelectedTroop = 0 // Deselect troop
					// log.Println("Troop selection cleared.")
				} else {
					// log.Println("ESC key pressed. Quit requested from UI loop.")
					quitRequested = true // Signal quit
					// No longer sending quit message from here
					break mainloop
				}
			case termbox.KeyEnter:
				if ui.lastSelectedTroop != 0 {
					// Convert rune to TroopID string
					// TODO: This mapping should come from game config or a shared model
					troopID := ""
					switch ui.lastSelectedTroop {
					case '1':
						troopID = "pawn"
					case '2':
						troopID = "bishop"
					case '3':
						troopID = "rook"
					case '4':
						troopID = "knight"
					case '5':
						troopID = "prince"
					case '6':
						troopID = "queen"
					default:
						// log.Printf("Invalid troop selection: %c", ui.lastSelectedTroop)
					}

					if troopID != "" && ui.client != nil {
						err := ui.client.SendDeployTroopCommand(troopID)
						if err != nil {
							// log.Printf("Error sending deploy troop command: %v", err)
							ui.AddEventMessage(fmt.Sprintf("Deploy Error: %v", err))
						} else {
							// log.Printf("Deploy troop command sent for: %s (%c)", troopID, ui.lastSelectedTroop)
							troopName := troopID
							switch ui.lastSelectedTroop {
							case '1':
								troopName = "Pawn"
							case '2':
								troopName = "Bishop"
							case '3':
								troopName = "Rook"
							case '4':
								troopName = "Knight"
							case '5':
								troopName = "Prince"
							case '6':
								troopName = "Queen"
							}
							ui.AddEventMessage(fmt.Sprintf("Deploy command for %s sent.", troopName))
						}
					} else if ui.client == nil {
						// log.Println("Cannot send deploy command: client reference is nil in UI")
					}
					ui.lastSelectedTroop = 0 // Clear selection after attempted deployment
				} else {
					// Handle command input if any, from ui.inputLine
					// log.Printf("Enter pressed. Current input (if any): %s", ui.inputLine)
					ui.inputLine = "" // Clear input line
				}
			default:
				// Check for troop selection keys '1' through '6'
				if ev.Ch >= '1' && ev.Ch <= '6' {
					ui.lastSelectedTroop = ev.Ch
					// log.Printf("Troop %c selected.", ui.lastSelectedTroop)
				} else if ev.Ch != 0 {
					// Append to general input line if not a troop selection
					// ui.inputLine += string(ev.Ch)
					// log.Printf("Other key: %c", ev.Ch) // For debugging other inputs
				}
				// For backspace on ui.inputLine etc., more complex input handling would be needed here
			}
			ui.Render() // Re-render after any key press that changes state

		case termbox.EventResize:
			// log.Println("Screen resized. Redrawing.")
			ui.ClearScreen()
			ui.DisplayStaticText(1, 1, "Basic Termbox UI Active. Press ESC to quit. (Resized)", termbox.ColorWhite, termbox.ColorBlack)

		case termbox.EventError:
			// log.Printf("Termbox event error: %v", ev.Err)
			break mainloop // Exit on error, quitRequested will be false
		}
	}
	return quitRequested
}

// GetTextInput prompts the user for text input at a specific location on the termbox screen.
// This is a very basic implementation.
func (ui *TermboxUI) GetTextInput(prompt string, x, y int, fg, bg termbox.Attribute) string {
	ui.DisplayStaticText(x, y, prompt, fg, bg)
	termbox.Flush()

	var runes []rune
	inputX := x + len(prompt)

	for {
		ev := termbox.PollEvent()
		if ev.Type != termbox.EventKey {
			continue
		}

		switch ev.Key {
		case termbox.KeyEnter:
			return string(runes)
		case termbox.KeyEsc:
			return "" // Cancel input
		case termbox.KeySpace:
			runes = append(runes, ' ')
		case termbox.KeyBackspace, termbox.KeyBackspace2:
			if len(runes) > 0 {
				runes = runes[:len(runes)-1]
				// Clear the last character
				termbox.SetCell(inputX+len(runes), y, ' ', fg, bg)
			}
		default:
			if ev.Ch != 0 {
				runes = append(runes, ev.Ch)
			}
		}

		// Display current input
		// Clear previous input (simple way, could be optimized)
		for i := 0; i < 50; i++ { // Clear a reasonable width
			termbox.SetCell(inputX+i, y, ' ', fg, bg)
		}
		for i, r := range runes {
			termbox.SetCell(inputX+i, y, r, fg, bg)
		}
		termbox.Flush()
	}
}

// Termbox rendering and input handling
