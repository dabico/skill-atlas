package skill

import "strings"

const delimiter = "---"

// splitFrontmatter separates the YAML frontmatter from the Markdown body.
// errMsg is empty when the frontmatter is well-formed.
func splitFrontmatter(content []byte) (front, body, errMsg string) {
	text := strings.TrimPrefix(string(content), "\uFEFF")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")

	if !isDelimiter(lines[0]) {
		return "", cleanBody(text), "missing YAML frontmatter (file must start with \"---\")"
	}
	for i := 1; i < len(lines); i++ {
		if isDelimiter(lines[i]) {
			return strings.Join(lines[1:i], "\n"), cleanBody(strings.Join(lines[i+1:], "\n")), ""
		}
	}
	return "", "", "frontmatter isn't closed with \"---\""
}

func isDelimiter(line string) bool {
	return strings.TrimRight(line, " \t\r") == delimiter
}

// cleanBody trims leading blank lines and trailing whitespace.
func cleanBody(s string) string {
	for {
		i := strings.IndexByte(s, '\n')
		if i < 0 || strings.TrimSpace(s[:i]) != "" {
			break
		}
		s = s[i+1:]
	}
	return strings.TrimRight(s, " \t\r\n")
}
