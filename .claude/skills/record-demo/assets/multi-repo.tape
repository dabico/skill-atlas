# Multi-repo TUI tour. demo.sh fills {{BIN}}, {{CMD}}, {{OUT}} and {{QUERY}}.
Output "{{OUT}}/tui-multi.gif"
Output "{{OUT}}/tui-multi.mp4"

Set Shell bash
Set FontSize 16
Set Width 1400
Set Height 800
Set Padding 20
Set Theme "Catppuccin Mocha"
Set WindowBar Colorful
Set TypingSpeed 30ms

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
Sleep 2s
Screenshot "{{OUT}}/tui-multi-list.png"

# Walk from the first repo into the next; the cursor skips headings and error rows.
Down@500ms 6
Sleep 2s
Screenshot "{{OUT}}/tui-multi-boundary.png"

# Filter across the repos.
Type "/"
Sleep 300ms
Type@150ms "{{QUERY}}"
Sleep 800ms
Enter
Sleep 2s
Screenshot "{{OUT}}/tui-multi-filter.png"

Escape
Sleep 1s
Type "q"
Sleep 800ms

# Failure lines stay on stderr; the exit code is 1 if any repo failed.
Type 'echo "exit $?"'
Enter
Sleep 2s
Screenshot "{{OUT}}/tui-multi-exit.png"
Sleep 1s
