Output "docs/images/tui-chat.gif"

Set Shell "bash"
Set Width 1200
Set Height 700
Set FontFamily "DejaVu Sans Mono"
Set FontSize 14
Set Theme { "name": "Bramble", "black": "#0d0d1a", "red": "#ff6b6b", "green": "#51cf66", "yellow": "#ffd43b", "blue": "#748ffc", "magenta": "#da77f2", "cyan": "#66d9e8", "white": "#e0e0e0", "brightBlack": "#666680", "brightRed": "#ff8787", "brightGreen": "#69db7c", "brightYellow": "#ffe066", "brightBlue": "#91a7ff", "brightMagenta": "#e599f7", "brightCyan": "#99e9f2", "brightWhite": "#ffffff", "background": "#0d0d1a", "foreground": "#e0e0e0", "selectionBackground": "#3d3d5c", "cursorColor": "#e0e0e0" }

# Setup is hidden from the recording: the GIF opens on the drawn TUI.
Hide
# Start background message sender (sends from Heltec-V4a on ACM1)
Type "~/src/bramble-cli/docs/images/send-during-recording.sh &"
Enter
Sleep 1s

# Launch TUI on T-Deck (ACM0)
Type "cd ~/src/bramble-cli && ./bramble tui --port /dev/ttyACM0"
Enter
Sleep 5s

# Start from a clean scrollback
Type "/clear"
Enter
Sleep 1s
Show

# Send a broadcast from the TUI
Type "Evening check-in — anyone on the mesh?"
Enter
Sleep 6s

# Wait for incoming replies from V4a
Sleep 6s

# Slap someone
Type "/slap Bob"
Enter
Sleep 3s

# Sign off
Type "Copy that. Same time tomorrow"
Enter
Sleep 4s

# Teardown is hidden: the recording ends on the TUI, not the shell.
Hide
# Quit
Type "/quit"
Enter
Sleep 1s
