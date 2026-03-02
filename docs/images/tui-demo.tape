Output "docs/images/tui-chat.gif"

Set Shell "bash"
Set Width 1200
Set Height 700
Set FontFamily "DejaVu Sans Mono"
Set FontSize 14
Set Theme { "name": "Bramble", "black": "#0d0d1a", "red": "#ff6b6b", "green": "#51cf66", "yellow": "#ffd43b", "blue": "#748ffc", "magenta": "#da77f2", "cyan": "#66d9e8", "white": "#e0e0e0", "brightBlack": "#666680", "brightRed": "#ff8787", "brightGreen": "#69db7c", "brightYellow": "#ffe066", "brightBlue": "#91a7ff", "brightMagenta": "#e599f7", "brightCyan": "#99e9f2", "brightWhite": "#ffffff", "background": "#0d0d1a", "foreground": "#e0e0e0", "selectionBackground": "#3d3d5c", "cursorColor": "#e0e0e0" }

# Start the background message sender before launching TUI
Type "~/src/bramble-cli/docs/images/send-during-recording.sh &"
Enter
Sleep 1s

# Launch TUI on Alpha
Type "cd ~/src/bramble-cli && ./bramble tui --port /dev/ttyACM0"
Enter
Sleep 5s

# Send a broadcast from the TUI
Type "Alpha checking in — anyone on the mesh?"
Enter
Sleep 6s

# Bravo's reply should appear by now (sent at ~8s mark)
# Wait for second reply too
Sleep 6s

# Show nodes
Type "/nodes"
Enter
Sleep 4s

# Quit
Type "/quit"
Enter
Sleep 1s
