Output demo/fix.gif
Output demo/fix.mp4

Require bun
Require node

Set Shell "zsh"
Set Width 1280
Set Height 720
Set FontSize 18
Set LineHeight 1.25
Set TypingSpeed 22ms
Set Padding 18
Set Margin 22
Set MarginFill "#ff6b5f"
Set BorderRadius 8
Set WindowBar Colorful
Set Theme { "name": "Hem Coral Dark", "black": "#17181c", "red": "#ff6b5f", "green": "#a7c080", "yellow": "#ffd166", "blue": "#7aa2f7", "magenta": "#f08bb4", "cyan": "#73daca", "white": "#f7f1ed", "brightBlack": "#4b4f58", "brightRed": "#ff8a7f", "brightGreen": "#c3d99b", "brightYellow": "#ffe08a", "brightBlue": "#9bbcff", "brightMagenta": "#ffabc8", "brightCyan": "#9be9dc", "brightWhite": "#ffffff", "background": "#111217", "foreground": "#f7f1ed", "selection": "#3b2b2d", "cursor": "#ff6b5f" }

Type "cd __HEM_VISIBLE_PROJECT__"
Enter
Sleep 1s

Type "hem snapshot create --name before-harness --agent codex --scope user --project ."
Enter
Sleep 3s

Type "npx experimental-codex-harness install"
Enter
Sleep 4s

Type "hem diff before-harness current --agent codex --scope user --project ."
Enter
Sleep 5s

Type "hem restore --snapshot before-harness --dry-run --agent codex --scope user --project ."
Enter
Sleep 7s

Type "hem restore --snapshot before-harness --apply --experimental --agent codex --scope user --project ."
Enter
Sleep 4s

Type "verify-rollback"
Enter
Sleep 5s
