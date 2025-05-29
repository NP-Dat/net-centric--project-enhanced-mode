**Project Plan: Enhanced Text-Based Clash Royale (UDP & Termbox)**

This document outlines the plan to build the "Enhanced TCR" game as a separate project, utilizing UDP for continuous gameplay and the `termbox-go` library for the client's user interface.

**Phase 1: Understanding & Definition (Enhanced TCR - UDP & Termbox)**

*   **Goal:** Create a real-time, text-based, client-server version of a Clash Royale-like game using Go, featuring UDP for core gameplay communication and `termbox-go` for a richer client UI.
*   **Core Architecture:** Client-Server model.
*   **Network Protocol Strategy:**
    *   **TCP:** Used for initial connection, user authentication, matchmaking, sending initial game configuration/rules, and delivering critical/reliable game end results (win/loss/draw, EXP awards).
    *   **UDP:** Used for all high-frequency, real-time in-game communication: player actions (deploy troop), game state updates (HP, Mana, troop positions/statuses, timer), and continuous attack events.
*   **Data Serialization:** JSON for client-server messages (both TCP and UDP).
*   **Client UI:** `termbox-go` library for a more structured and responsive text-based interface.
*   **Authentication:** Users connect with a username and password (via TCP). Passwords securely hashed (`bcrypt`) on the server.
*   **Persistence:**
    *   Player data (Username, HashedPassword, EXP, Level) stored in server-side JSON files.
    *   Base Tower and Troop specifications (HP, ATK, DEF, CRIT% for Towers, Mana Cost for Troops, EXP Yield for Towers) loaded from server-side configuration JSON files (using the provided Appendix).
*   **Game Elements (Enhanced TCR Rules):**
    *   **Players:** Two players per game session.
    *   **Towers:** Each player has 1 King Tower and 2 Guard Towers. Stats from Appendix.
    *   **Troops:** Players deploy troops from the defined list (Pawn, Bishop, Rook, Knight, Prince, Queen). Stats from Appendix.
    *   **Targeting Rule:** Guard Tower 1 must be destroyed before Guard Tower 2 or the King Tower can be targeted by troops.
    *   **Combat:**
        *   Damage Formula: `DMG = Attacker_ATK - Defender_DEF`. If DMG < 0, DMG = 0. `Defender_HP = Defender_HP - DMG`.
        *   CRIT Chance: Attacks from Towers have a CRIT chance (from Appendix). Troops have 0% base CRIT. Critical Hit Damage: `DMG = (Attacker_ATK * 1.2) - Defender_DEF`.
        *   Troops only attack Towers.
        *   Towers attack Troops.
    *   **Queen Troop:** Costs 5 Mana. Deployment is a one-time action healing the friendly tower with the lowest absolute HP by 300 (up to max HP). Does not persist on the board.
    *   **Real-Time:** No turns. Game runs continuously for 3 minutes.
    *   **MANA System:**
        *   Players start with 5 MANA.
        *   MANA regenerates at 1 MANA per **2 seconds**.
        *   Maximum MANA is 10.
        *   Deploying troops costs MANA.
    *   **Continuous Attack:**
        *   Deployed troops attack automatically **once per 2 seconds**, targeting the opponent's lowest absolute HP valid tower.
        *   Towers attack automatically **once per 2 seconds** if a valid enemy troop target exists (last attacker or oldest attacker).
    *   **Win Conditions:**
        *   Instant Win: First player to destroy the opponent's King Tower.
        *   Timeout (3 minutes): Player who destroyed more towers wins. Draw if equal.
    *   **EXP & Leveling System:**
        *   Players gain EXP by destroying enemy towers (amount per tower type) and by winning (30 EXP) or drawing (10 EXP) a match.
        *   EXP required for the next level increases cumulatively by 10% per level (base: 100 EXP for Level 2).
        *   Each player level increases their Troops' and Towers' base HP, ATK, and DEF by 10% cumulatively per level.

**Phase 2: Proposed Architecture & Design (Enhanced TCR - UDP & Termbox)**

1.  **Server:**
    *   **Responsibilities:**
        *   Listens for incoming TCP connections (for auth, matchmaking, initial setup, final results).
        *   Listens on a dedicated UDP port for in-game messages.
        *   Handles client authentication (TCP).
        *   Manages player matchmaking (TCP).
        *   Creates and manages game sessions.
        *   Enforces Enhanced TCR game rules.
        *   Runs the real-time game loop (timer, mana regen, continuous attacks).
        *   Calculates combat (including CRITs), manages HP, MANA, EXP, levels.
        *   Sends game state updates to clients via UDP.
        *   Sends critical game results via TCP.
        *   Persists player data and loads game configuration.
    *   **Concurrency:** Goroutines for TCP connections, UDP packet processing, and individual game sessions. Channels for internal communication.
    *   **UDP Handling:**
        *   Maintain a mapping of client (IP:Port or session token) to game sessions.
        *   Implement sequence numbers for UDP messages that require ordering or loss detection (e.g., deploy troop commands).
        *   State updates might be sent as full snapshots or prioritized deltas to manage bandwidth and complexity.

2.  **Client:**
    *   **Responsibilities:**
        *   Connects to the server via TCP for initial handshake and setup.
        *   Switches to UDP communication for in-game actions and state updates.
        *   Manages `termbox-go` UI for rendering game state and handling input.
            * The game interface is at the top of terminal, and render constantly.
            * Client can input command below the game interface.
        *   Prompts user for username/password (sent via TCP).
        *   Receives and displays game state (towers, troops, HP, mana, timer) via UDP, rendered with `termbox-go`.
        *   Captures user actions (e.g., key press to select and deploy troop) via `termbox-go` events and sends them via UDP.
        *   Handles server messages (TCP for critical, UDP for real-time).
    *   **Concurrency:**
        *   Main goroutine for `termbox-go` event loop and rendering.
        *   Goroutine for listening to UDP messages from the server.
        *   Goroutine for handling TCP communication (initial/final stages).

3.  **Communication Protocol (Hybrid):**
    *   **TCP Channel:**
        *   C2S: `login_request`, `matchmaking_request`.
        *   S2C: `login_response`, `match_found_response` (includes UDP port info, opponent details), `game_config_data`, `game_over_results` (EXP, level changes).
    *   **UDP Channel:** (All messages should ideally include a sequence number and session/player identifier)
        *   C2S: `deploy_troop_command_udp` (`troop_id`), `player_input_udp` (generic for other potential inputs).
        *   S2C: `game_state_update_udp` (full or delta: towers, troops, mana, timer), `game_event_udp` (e.g., "Tower Destroyed!").
    *   **Serialization:** JSON for both TCP and UDP message payloads.

4.  **Data Structures (Core Models):**
    *   `Player`: `ID`, `Username`, `HashedPassword`, `EXP`, `Level`, `Connection` (transient `net.Conn`), `GameID` (transient), `CurrentMana` (Enhanced), `Towers` (map/slice), `AvailableTroops` (map/slice based on clarification), `ActiveTroops` (map/slice).
    *   `Tower`: `ID`, `Name`, `BaseHP`, `BaseATK`, `BaseDEF`, `CurrentHP`, `OwnerPlayerID`.
    *   `TroopSpec`: `ID`, `Name`, `BaseHP`, `BaseATK`, `BaseDEF`, `ManaCost` (Enhanced), `BaseCritChance` (Enhanced). (Loaded from JSON).
    *   `ActiveTroop`: `InstanceID`, `SpecID`, `CurrentHP`, `OwnerPlayerID`, `TargetID` (transient).
    *   `Game`: `ID`, `Players` [2]*Player, `GameState` (e.g., `Waiting`, `RunningSimple`, `RunningEnhanced`, `Finished`), `CurrentTurnPlayerID` (Simple), `StartTime` (Enhanced), `EndTime` (Enhanced), `BoardState` (containing all active troops and tower statuses).
    *   `PlayerData`: `Username`, `HashedPassword`, `EXP`, `Level`. (For JSON persistence).
    *   `GameConfig`: Contains maps/slices of `TowerSpec` and `TroopSpec`. (Loaded from JSON).

5.  **Persistence:**
    *   Use Go's `encoding/json` package.
    *   Player data: Store one JSON file per player (e.g., `data/players_enhanced/username.json`) or a single JSON file containing a map of all players. A single file is simpler initially, but separate files scale better if many players were expected (though not likely critical here).
    *   Game config: Store troop and tower base stats in separate JSON files (e.g., `config_enhanced/troops.json`, `config_enhanced/towers.json`).

**Phase 3: Project Structure (Enhanced TCR - UDP & Termbox)**

```
enhanced-tcr-udp/
├── cmd/
│   ├── tcr-server-enhanced/
│   │   └── main.go                 # Server executable entry point
│   └── tcr-client-enhanced/
│       └── main.go                 # Client executable entry point
├── internal/
│   ├── server/
│   │   ├── server.go               # Main server logic (TCP listener, UDP listener)
│   │   ├── auth_tcp.go             # Authentication (TCP)
│   │   ├── matchmaking_tcp.go      # Matchmaking, Pairing players (TCP)
│   │   ├── session_manager.go      # Manages game sessions
│   │   └── game_session.go         # Logic for a single game session (real-time loop, UDP handling)
│   ├── client/
│   │   ├── client.go               # Main client logic (TCP/UDP connection, termbox setup)
│   │   ├── network_handler.go      # Handles incoming TCP/UDP messages
│   │   └── ui_termbox.go           # Termbox rendering and input handling
│   ├── game/
│   │   ├── logic_enhanced.go       # Core Enhanced TCR game rules, state
│   │   ├── combat.go               # Damage, CRIT calculation
│   │   └── progression.go          # EXP, Leveling
│   ├── models/                 
│   │   ├── player.go               # Player data structure (for persistence)
│   │   ├── config.go               # Structures for loading troop/tower specs
│   │   └── game_entities.go
│   ├── network/
│   │   ├── protocol_tcp.go         # TCP message definitions
│   │   ├── protocol_udp.go         # UDP message definitions
│   │   └── codec.go                # JSON encoding/decoding
│   └── persistence/
│       └── storage.go         # Functions for loading/saving JSON data (player profiles, config)
│ 
│  
│ 
├── data/                     # Default directory for persistent player data
│   └── players_enhanced/     
├── config_enhanced/          # Default directory for game configuration files
│   ├── troops.json
│   └── towers.json
├── go.mod
├── go.sum
└── README.md
```

**Phase 4: Development Plan (Iterative for Enhanced TCR - UDP & Termbox)**

*   **Sprint 0: Foundation - TCP/UDP Basics & Termbox Shell**
    *   [x] Implement basic TCP server/client for handshake (e.g., simple ping/pong).
    *   [x] Implement basic UDP server/client for sending/receiving simple datagrams.
    *   [x] Create a basic `termbox-go` client application: initialize screen, display static text, handle basic key input (e.g., quit).
    *   [x] Define core data models (`models/`).
    *   [x] Define initial TCP and UDP PDU structures (`network/protocol_tcp.go`, `network/protocol_udp.go`).
    *   [x] Implement `persistence/storage` to load dummy `config/towers.json` and `config/troops.json`.
    *   [x] Implement basic `network/codec` for sending/receiving simple JSON messages.

*   **Sprint 1: Authentication, Matchmaking (TCP) & Game Session Init**
    *   [x] Implement user authentication over TCP (`internal/server/auth_tcp.go`) - store hashed passwords in memory or basic files initially (`internal/persistence`).
    *   [x] Implement client-side login UI (simple text input before `termbox` fully integrated for this part, or basic `termbox` input).
    *   [X] Implement matchmaking over TCP, pair the first two authenticated users (`internal/server/matchmaking_tcp.go`).
    *   [x] Server: Upon match, create a game session, assign a UDP port/game ID, and send this info to clients via TCP.
    *   [x] Client: Receive game session info, prepare to switch to UDP for gameplay.
    *   [x] Load game configuration (towers, troops) from JSON (`internal/persistence/storage.go`).

*   **Sprint 2: Basic Real-Time Loop & Termbox UI Core (UDP)**
    *   [x] Server: Implement the main game loop for a session (3-minute timer). Auto cancel the game when 2 clients quit.
    *   [x] Client: Establish UDP communication with the server for the game session.
    *   [x] Server: Start sending basic periodic `game_state_update_udp` (e.g., just timer, initial mana).
    *   [x] Client (`ui_termbox.go`):
        *   Render basic game board layout (areas for player/opponent towers, mana, timer).
        *   Display initial tower HPs, player mana, game timer based on UDP updates.
        *   Implement input handling for selecting a troop to deploy (e.g., number keys).

*   **Sprint 3: Troop Deployment & Mana System (UDP)**
    *   [x] Client: Send `deploy_troop_command_udp` when player attempts to deploy.
    *   [x] Server: Handle `deploy_troop_command_udp`: check mana, validate troop, update game state.
    *   [x] Server: Implement MANA regeneration (1 per 2s, max 10) and update in `game_state_update_udp`.
    *   [x] Server: Include active troops (basic info: type, owner, HP) in `game_state_update_udp`.
    *   [x] Client (`ui_termbox.go`): Display player's current mana, list of available troops (with mana costs), and visually represent deployed troops on the board.

*   **Sprint 4: Continuous Combat & Targeting (UDP)**
    *   [x] Server (`game/logic_enhanced.go`, `game/combat.go`):
        *   Implement continuous troop attacks (1/sec) against towers (lowest HP valid target, GT1 rule).
        *   Implement continuous tower attacks (1/sec) against troops (last/oldest attacker).
        *   Implement damage calculation and CRIT chance (for towers).
        *   Implement Queen's heal ability.
    *   [x] Server: Update HP values in `game_state_update_udp` frequently. Send `game_event_udp` for significant combat events (tower destroyed, critical hit).
    *   [x] Client (`ui_termbox.go`): Update HP displays for towers and troops dynamically. Display game events from server.

*   **Sprint 5: Win Conditions, EXP/Leveling & Results**
    *   [x] Server: Implement win condition checks (King Tower destruction, timeout tower count).
    *   [x] Server: Upon game end, calculate EXP earned (tower destruction + win/draw bonus).
    *   [x] Server: Update player EXP/Level in persisted data (`internal/persistence/storage.go`).
    *   [x] Server: Send `game_over_results` message to clients.
    *   [x] Client: Handle `game_over_results` and display outcome, EXP earned, and any level-up message in `termbox-go`.
    *   [x] Ensure player levels correctly influence troop/tower stats at the start of the next game.

*   **Sprint 6: UDP Reliability, Refinements & Testing**
    *   [x] Implement sequence numbers for UDP messages if not already done, especially for commands like `deploy_troop_command_udp`. Consider simple ACK mechanism for critical UDP commands if packet loss is problematic.
    *   [x] Refine `termbox-go` UI for clarity, responsiveness, and better visual feedback.
    *   [x] Add comprehensive error handling for network issues (TCP/UDP), invalid inputs.
    *   [x] Conduct thorough testing, including simulating UDP packet loss/reordering if possible.

**Phase 5: Testing Strategy (Enhanced TCR - UDP & Termbox)**

*   **Unit Tests:** Test individual functions in `game/combat.go`, `game/progression.go`, `game/logic_enhanced.go`, persistence, and network PDU encoding/decoding.
*   **Integration Tests:**
    *   Test TCP handshake, auth, matchmaking, and transition to UDP.
    *   Test client-server interaction over UDP for core game mechanics.
    *   Test game end and results delivery over TCP.
*   **End-to-End Tests:** Run the server and multiple `termbox-go` client instances to simulate full game sessions.
*   **UI/UX Testing:** Manually test the `termbox-go` client for usability, clarity, and responsiveness.
*   **Network Robustness Testing (UDP):** If tools or manual methods allow, test behavior under simulated packet loss, high latency, or packet reordering to see how the game handles these conditions.

**Phase 6: Deliverables (Enhanced TCR - UDP & Termbox)**

*   [ ] Source Code for the `enhanced-tcr-udp` project (well-structured Go project).
*   [ ] `README.md` with build/run instructions specific to this enhanced version.
*   [ ] JSON configuration files (`config_enhanced/`) for troops/towers.
*   [ ] Demonstration of the application, showcasing real-time gameplay, `termbox-go` UI, and UDP communication.
