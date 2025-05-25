package client

import (
	"fmt"
	"log"

	"github.com/nsf/termbox-go"
)

// TermboxUI holds state for the termbox interface
type TermboxUI struct {
	// Add state here if needed, e.g., current view, messages
	gameTimer   int
	player1Mana int
	player2Mana int
	// TODO: Add fields for tower HPs, troop positions etc.
	// player1Towers []models.TowerDisplayInfo
	// player2Towers []models.TowerDisplayInfo
	// activeTroops  []models.TroopDisplayInfo

	inputLine         string  // For user commands
	lastSelectedTroop rune    // Example: '1' for Pawn, '2' for Bishop etc.
	client            *Client // Reference to the main client logic
}

// NewTermboxUI creates a new TermboxUI manager.
func NewTermboxUI() *TermboxUI {
	return &TermboxUI{}
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
func (ui *TermboxUI) UpdateGameInfo(timer, p1Mana, p2Mana int) {
	ui.gameTimer = timer
	ui.player1Mana = p1Mana
	ui.player2Mana = p2Mana
	// In a real scenario, you might lock here if accessed by multiple goroutines,
	// but termbox operations should generally be from one goroutine.
}

// Render draws the entire game UI based on current state.
func (ui *TermboxUI) Render() {
	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)

	// Game Info Area (Top)
	infoLine1 := fmt.Sprintf("Time: %d s", ui.gameTimer)
	infoLine2 := fmt.Sprintf("P1 Mana: %d | P2 Mana: %d", ui.player1Mana, ui.player2Mana)
	ui.DisplayStaticText(1, 1, infoLine1, termbox.ColorWhite, termbox.ColorBlack)
	ui.DisplayStaticText(1, 2, infoLine2, termbox.ColorWhite, termbox.ColorBlack)

	// Placeholder for Player 1 Towers
	ui.DisplayStaticText(5, 5, "[P1 GT1: HP ?/?]", termbox.ColorGreen, termbox.ColorBlack)
	ui.DisplayStaticText(5, 7, "[P1 KT: HP ?/?]", termbox.ColorGreen, termbox.ColorBlack)
	ui.DisplayStaticText(5, 9, "[P1 GT2: HP ?/?]", termbox.ColorGreen, termbox.ColorBlack)

	// Placeholder for Player 2 Towers
	ui.DisplayStaticText(40, 5, "[P2 GT1: HP ?/?]", termbox.ColorRed, termbox.ColorBlack)
	ui.DisplayStaticText(40, 7, "[P2 KT: HP ?/?]", termbox.ColorRed, termbox.ColorBlack)
	ui.DisplayStaticText(40, 9, "[P2 GT2: HP ?/?]", termbox.ColorRed, termbox.ColorBlack)

	// Placeholder for troop deployment area / active troops
	ui.DisplayStaticText(1, 12, "--- Active Troops --- (TODO)", termbox.ColorYellow, termbox.ColorBlack)

	// Input Area (Bottom)
	troopSelectionPrompt := "Deploy: [1]Pawn [2]Bishop [3]Rook [4]Knight [5]Prince [6]Queen. ESC to Deselect."
	ui.DisplayStaticText(1, 15, troopSelectionPrompt, termbox.ColorCyan, termbox.ColorBlack)
	selectedMsg := "Selected: None"
	if ui.lastSelectedTroop != 0 {
		selectedMsg = fmt.Sprintf("Selected: %c (Press Enter to confirm deployment - simulated)", ui.lastSelectedTroop)
	}
	ui.DisplayStaticText(1, 16, selectedMsg, termbox.ColorWhite, termbox.ColorBlack)

	// Command line (if we had one separate from troop selection)
	// ui.DisplayStaticText(1, 18, "> "+ui.inputLine, termbox.ColorWhite, termbox.ColorBlack)

	termbox.Flush()
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
					log.Println("Troop selection cleared.")
				} else {
					log.Println("ESC key pressed. Quit requested from UI loop.")
					quitRequested = true // Signal quit
					// No longer sending quit message from here
					break mainloop
				}
			case termbox.KeyEnter:
				if ui.lastSelectedTroop != 0 {
					// TODO: Send deploy_troop_command_udp (Sprint 3)
					log.Printf("Simulating deployment of troop: %c", ui.lastSelectedTroop)
					// Placeholder for actual deployment logic
					// c.sendDeployTroopCommand(ui.lastSelectedTroop) // Needs client reference or callback
					ui.lastSelectedTroop = 0 // Clear selection after attempted deployment
				} else {
					// Handle command input if any, from ui.inputLine
					log.Printf("Enter pressed. Current input (if any): %s", ui.inputLine)
					ui.inputLine = "" // Clear input line
				}
			default:
				// Check for troop selection keys '1' through '6'
				if ev.Ch >= '1' && ev.Ch <= '6' {
					ui.lastSelectedTroop = ev.Ch
					log.Printf("Troop %c selected.", ui.lastSelectedTroop)
				} else if ev.Ch != 0 {
					// Append to general input line if not a troop selection
					// ui.inputLine += string(ev.Ch)
					log.Printf("Other key: %c", ev.Ch) // For debugging other inputs
				}
				// For backspace on ui.inputLine etc., more complex input handling would be needed here
			}
			ui.Render() // Re-render after any key press that changes state

		case termbox.EventResize:
			log.Println("Screen resized. Redrawing.")
			ui.ClearScreen()
			ui.DisplayStaticText(1, 1, "Basic Termbox UI Active. Press ESC to quit. (Resized)", termbox.ColorWhite, termbox.ColorBlack)

		case termbox.EventError:
			log.Printf("Termbox event error: %v", ev.Err)
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

// SetClient allows the main client logic to pass a reference to itself to the UI.
func (ui *TermboxUI) SetClient(c *Client) {
	ui.client = c
}

// Termbox rendering and input handling
