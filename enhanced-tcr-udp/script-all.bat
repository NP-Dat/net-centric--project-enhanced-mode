@echo off
echo Starting Text Clash Royale Server...
start cmd /k "cd D:\Phuc Dat\IU\MY PROJECT\Golang\net-centric--project-enhanced-mode\enhanced-tcr-udp && go run .\cmd\tcr-server-enhanced\main.go"

timeout /t 5 /nobreak >nul

echo Starting Text Clash Royale Client 1...
start cmd /k "cd D:\Phuc Dat\IU\MY PROJECT\Golang\net-centric--project-enhanced-mode\enhanced-tcr-udp && go run .\cmd\tcr-client-enhanced\main.go"

timeout /t 2 /nobreak >nul

echo Starting Text Clash Royale Client 2...
start cmd /k "cd D:\Phuc Dat\IU\MY PROJECT\Golang\net-centric--project-enhanced-mode\enhanced-tcr-udp && go run .\cmd\tcr-client-enhanced\main.go"

echo Server and Clients started in separate windows.
echo Close the server window first (Ctrl+C) to stop both.

REM run ".\script-all.bat"