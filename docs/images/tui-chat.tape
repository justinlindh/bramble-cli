Output "docs/images/tui-chat.gif"

Set Shell "bash"
Set Width 1200
Set Height 700
Set FontFamily "DejaVu Sans Mono"
Set FontSize 14
Set Theme { "name": "Bramble", "black": "#0d0d1a", "red": "#ff6b6b", "green": "#51cf66", "yellow": "#ffd43b", "blue": "#748ffc", "magenta": "#da77f2", "cyan": "#66d9e8", "white": "#e0e0e0", "brightBlack": "#666680", "brightRed": "#ff8787", "brightGreen": "#69db7c", "brightYellow": "#ffe066", "brightBlue": "#91a7ff", "brightMagenta": "#e599f7", "brightCyan": "#99e9f2", "brightWhite": "#ffffff", "background": "#0d0d1a", "foreground": "#e0e0e0", "selectionBackground": "#3d3d5c", "cursorColor": "#e0e0e0" }

Type "cd /home/user/src/bramble-cli"
Enter
Type "./bramble tui"
Enter
Sleep 3s
Type "/help"
Enter
Sleep 1500ms
Type "/nodes"
Enter
Sleep 1500ms
Type "/stats"
Enter
Sleep 1500ms
Escape
Type "1"
Sleep 1s
Escape
Type "2"
Sleep 1s
Ctrl+C
Sleep 500ms
