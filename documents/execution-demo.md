# Execution Demo: Enhanced Text-Based Clash Royale (UDP & Termbox)

This document provides instructions on how to run and test the `enhanced-tcr-udp` server and client components.

## Prerequisites

*   Go programming language installed and configured.
*   A terminal or command prompt that supports `go run`.
*   The project dependencies should be managed by Go modules (`go.mod` in the `enhanced-tcr-udp` directory).

## 1. Running the Server

The server handles client connections, authentication, matchmaking, and will eventually manage game sessions.

1.  **Open a terminal.**
2.  **Navigate to the server's directory:**
    ```bash
    cd "D:/Phuc Dat/IU/MY PROJECT/Golang/net-centric--project-enhanced-mode/enhanced-tcr-udp/cmd/tcr-server-enhanced"
    ```
    *(Adjust the path if your workspace root differs slightly, though this is based on your provided setup.)*
3.  **Run the server executable:**
    ```bash
    go run main.go
    ```
4.  **Expected Server Output:**
    You should see log messages indicating the server has started:
    ```text
    INFO Starting Enhanced TCR Server...
    INFO Server listening for TCP connections on localhost:8080
    INFO Global UDP echo server listening on localhost:8081
    INFO Server is running. Press Ctrl+C to exit.
    ```
    The server is now active and waiting for client connections.

## 2. Running the Client (Two Instances for Matchmaking)

The client provides a Termbox-based UI for interacting with the server.

**For Client 1:**

1.  **Open a new terminal.** (Keep the server terminal running.)
2.  **Navigate to the client's directory:**
    ```bash
    cd "D:/Phuc Dat/IU/MY PROJECT/Golang/net-centric--project-enhanced-mode/enhanced-tcr-udp/cmd/tcr-client-enhanced"
    ```
3.  **Run the first client executable:**
    ```bash
    go run main.go
    ```
4.  **Client 1 Interaction:**
    *   The terminal window will clear and initialize a Termbox interface.
    *   A welcome message appears, followed by "Login Required".
    *   **Username:** Type a unique username (e.g., `player1`) and press `Enter`.
    *   **Password:** Type a password (e.g., `pass1`) and press `Enter`. If the account is new, it will be created.
    *   Upon successful login, the UI will display:
        ```text
        Welcome, player1 (Level 1, EXP 0)!
        Login successful. Requesting matchmaking...
        Waiting for match...
        ```
    *   Client 1 will now wait in the matchmaking queue.

**For Client 2:**

1.  **Open another new terminal.** (Keep server and Client 1 terminals running.)
2.  **Navigate to the client's directory (same as Client 1):**
    ```bash
    cd "D:/Phuc Dat/IU/MY PROJECT/Golang/net-centric--project-enhanced-mode/enhanced-tcr-udp/cmd/tcr-client-enhanced"
    ```
3.  **Run the second client executable:**
    ```bash
    go run main.go
    ```
4.  **Client 2 Interaction:**
    *   Follow the same Termbox login steps as Client 1, but use a **different unique username** (e.g., `player2`, password `pass2`).

## 3. Observing Matchmaking and UDP Test

Once Client 2 logs in and enters matchmaking:

*   **Both Client 1 and Client 2 UIs should update to show:**
    *   "Match Found!"
    *   Game ID (a unique UUID).
    *   Opponent's username and level.
    *   The UDP port assigned for the game session.
    *   Whether they are Player One or not.
*   **UDP Ping Test:**
    *   The UI will then display: "Attempting to send a UDP ping to global echo server (localhost:8081)..."
    *   If the server's global UDP echo is running and reachable, it should show: "UDP Ping successful! Response: UDP Echo: Hello UDP Echo Server!" (or a similar echo from the server).
    *   If there's an issue, a UDP Ping failure message will be shown.
*   **Exiting the Client:**
    *   The UI will then show a message like: "Client is ready for game-specific UDP gameplay. Press ESC to exit this screen."
    *   Press the `ESC` key on each client's Termbox window. This will close the UI and terminate the client application.

## 4. Stopping the Server

1.  Return to the terminal where the server is running.
2.  Press `Ctrl+C`.
3.  The server will initiate a graceful shutdown, displaying messages like:
    ```text
    INFO Shutdown signal received, stopping server...
    INFO Server stopped gracefully.
    ```

## Notes

*   If `go run` has issues finding packages (e.g., `termbox-go`), ensure your Go environment is correctly set up and that you are in the correct directory (`enhanced-tcr-udp` subdirectories) which contains the `go.mod` file for the respective `cmd` application or its parent module.
*   The client interaction is entirely through the Termbox UI. There are no command-line commands like `login <user> <pass>` or `join` in this version; these actions are part of the UI flow.
*   This demo tests authentication, matchmaking for two players, and a basic UDP message exchange with the server's global echo UDP port. Actual gameplay logic and game-specific UDP communication are part of subsequent development sprints.

