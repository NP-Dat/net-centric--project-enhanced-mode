package client

import (
	"log"

	"github.com/nsf/termbox-go"
)

// TermboxUI holds state for the termbox interface
type TermboxUI struct {
	// Add state here if needed, e.g., current view, messages
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

// ClearScreen clears the termbox screen.
func (ui *TermboxUI) ClearScreen() {
	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
}

// RunSimpleEvacuateLoop runs a basic event loop that waits for Escape key to quit.
// This is a placeholder for a more complex game UI event loop.
func (ui *TermboxUI) RunSimpleEvacuateLoop() {
	ui.DisplayStaticText(1, 1, "Basic Termbox UI Active. Press ESC to quit.", termbox.ColorWhite, termbox.ColorBlack)

mainloop:
	for {
		switch ev := termbox.PollEvent(); ev.Type {
		case termbox.EventKey:
			if ev.Key == termbox.KeyEsc {
				log.Println("ESC key pressed. Exiting termbox loop.")
				break mainloop
			}
			// You can add other key handlers here for basic input
			// For example, to redraw screen or show pressed key:
			// debugMsg := fmt.Sprintf("Key: %s, Char: %c", ev.Key, ev.Ch)
			// ui.DisplayStaticText(1, 3, debugMsg, termbox.ColorYellow, termbox.ColorBlack)

		case termbox.EventResize:
			log.Println("Screen resized. Redrawing.")
			ui.ClearScreen()
			ui.DisplayStaticText(1, 1, "Basic Termbox UI Active. Press ESC to quit. (Resized)", termbox.ColorWhite, termbox.ColorBlack)

		case termbox.EventError:
			log.Printf("Termbox event error: %v", ev.Err)
			break mainloop // Exit on error
		}
	}
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
