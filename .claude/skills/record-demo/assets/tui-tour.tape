# TUI tour. demo.sh fills {{BIN}}, {{CMD}}, {{OUT}} and {{QUERY}}.
Output "{{OUT}}/tui-tour.gif"
Output "{{OUT}}/tui-tour.mp4"

Set Shell bash
Set FontSize 16
Set Width 1400
Set Height 800
Set Padding 20
Set Theme "Catppuccin Mocha"
Set WindowBar Colorful
Set TypingSpeed 40ms

Hide
Type "export PATH={{BIN}}:$PATH BASH_SILENCE_DEPRECATION_WARNING=1 && clear"
Enter
Sleep 500ms
Show

Type "{{CMD}}"
Sleep 500ms
Enter

# Cut the clone wait out of the recording.
Hide
Wait+Screen@120s /\d+ skills?, \d+ invalid/
Sleep 500ms
Show
Sleep 1.5s
Screenshot "{{OUT}}/tui-list.png"

# Walk the list.
Down@400ms 4
Sleep 1s

# Filter, keep it, show the match.
Type "/"
Sleep 300ms
Type@120ms "{{QUERY}}"
Sleep 800ms
Enter
Sleep 1.5s
Screenshot "{{OUT}}/tui-filter.png"

# Scroll the detail pane.
Tab
Sleep 500ms
Down@200ms 10
Sleep 1.5s
Screenshot "{{OUT}}/tui-detail.png"

Tab
Sleep 500ms
Escape
Sleep 1s
Type "q"
Sleep 500ms
