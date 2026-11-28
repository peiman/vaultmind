package arcgate

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode"
)

var decisionCue = regexp.MustCompile("(?i)(recommend|i['’]d pick|i would pick|choose|go with|decision:|i suggest)")
var imperative = regexp.MustCompile("(?i)(?:^|[.!?][[:space:]]+)(?:do|don't|do not)[[:space:]]+[[:alnum:]]+|(?:don't|do not)[[:space:]]+[[:alnum:]]+")
var snakeCase = regexp.MustCompile("[[:alnum:]]_[[:alnum:]]")

var plainWordsAllowlist = map[string]bool{
	"OK": true, "PR": true, "CI": true, "API": true, "URL": true, "CLI": true,
	"ID": true, "UI": true, "JSON": true, "HTTP": true, "HTTPS": true,
	"YES": true, "NO": true,
}

// CheckPlainWords reports general plain-words failures in msg.
func CheckPlainWords(msg string) []string {
	if strings.TrimSpace(msg) == "" {
		return nil
	}
	tokens := strings.Fields(msg)
	var flags []string
	if len(tokens) > 250 {
		flags = append(flags, fmt.Sprintf("%d words, over 250", len(tokens)))
	}
	if len(tokens) > 80 && !hasDecisionCue(firstTwoUnits(msg)) {
		flags = append(flags, "no decision in the first two sentences")
	}
	codeLike := 0
	for _, token := range tokens {
		if isCodeLike(trimToken(token)) {
			codeLike++
		}
	}
	if len(tokens) > 0 {
		percent := int(math.Round(float64(codeLike) * 100 / float64(len(tokens))))
		if float64(codeLike)/float64(len(tokens)) > 0.08 {
			flags = append(flags, fmt.Sprintf("%d%% of words are code-like, over 8%%", percent))
		}
	}
	return flags
}

func trimToken(token string) string {
	return strings.TrimFunc(token, func(r rune) bool {
		return strings.ContainsRune(".,;:!?()[]{}\"'", r)
	})
}

func firstTwoUnits(msg string) string {
	var units []string
	var current strings.Builder
	flush := func() {
		if text := strings.TrimSpace(current.String()); text != "" {
			units = append(units, text)
		}
		current.Reset()
	}
	lines := strings.Split(strings.ReplaceAll(msg, "\r\n", "\n"), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			flush()
			continue
		}
		if isListBullet(trimmed) {
			flush()
		}
		for i, r := range trimmed {
			current.WriteRune(r)
			if (r == '.' || r == '!' || r == '?') && sentenceBoundary(trimmed, i+len(string(r))) {
				flush()
				if len(units) >= 2 {
					return strings.Join(units[:2], " ")
				}
			}
		}
		if current.Len() > 0 {
			current.WriteByte(' ')
		}
	}
	flush()
	if len(units) > 2 {
		units = units[:2]
	}
	return strings.Join(units, " ")
}

func sentenceBoundary(line string, next int) bool {
	if next >= len(line) {
		return true
	}
	return unicode.IsSpace(rune(line[next]))
}

func isListBullet(line string) bool {
	if len(line) >= 2 && (line[0] == '-' || line[0] == '*' || line[0] == '+') && unicode.IsSpace(rune(line[1])) {
		return true
	}
	for i := 0; i < len(line) && i < 3; i++ {
		if line[i] == '.' && i > 0 && unicode.IsDigit(rune(line[i-1])) && i+1 < len(line) && unicode.IsSpace(rune(line[i+1])) {
			return true
		}
	}
	return false
}

func hasDecisionCue(text string) bool {
	return decisionCue.MatchString(text) || imperative.MatchString(text)
}

func isCodeLike(token string) bool {
	if token == "" {
		return false
	}
	if strings.Contains(token, "`") || snakeCase.MatchString(token) {
		return true
	}
	letters := 0
	allUpper := true
	upperCount := 0
	firstLetterUpper := false
	for _, r := range token {
		if unicode.IsLetter(r) {
			if letters == 0 {
				firstLetterUpper = unicode.IsUpper(r)
			}
			letters++
			if !unicode.IsUpper(r) {
				allUpper = false
			}
			if unicode.IsUpper(r) {
				upperCount++
			}
		}
	}
	if allUpper && letters >= 3 && !plainWordsAllowlist[token] {
		return true
	}
	if allUpper {
		return false
	}
	camel := (!firstLetterUpper && upperCount >= 1) || (firstLetterUpper && upperCount >= 2)
	return letters > 1 && camel
}
