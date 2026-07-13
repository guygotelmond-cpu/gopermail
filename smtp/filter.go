package smtp

import (
	"strings"

	"gomail.com/db"
)

// ProcessRules evaluates an email against a list of rules and returns its target folder and spam status.
func ProcessRules(rules []db.Rule, sender string, body string) (string, bool) {
	folder := "INBOX"
	isSpam := false

	for _, rule := range rules {
		match := false
		var targetField string

		// 1. Resolve which field we are testing
		switch strings.ToLower(rule.Field) {
		case "sender":
			targetField = sender
		case "body", "subject":
			// For simplicity, we search the raw body blob which includes headers
			targetField = body
		default:
			continue
		}

		// 2. Evaluate the matching operator
		switch strings.ToLower(rule.Operator) {
		case "contains":
			match = strings.Contains(strings.ToLower(targetField), strings.ToLower(rule.Value))
		case "equals":
			match = strings.EqualFold(targetField, rule.Value)

		}

		// 3. Execute the action if a match is found
		if match {
			switch strings.ToLower(rule.Action) {
			case "mark_spam":
				isSpam = true
				folder = "SPAM"
			case "move_to":
				folder = rule.ActionValue
			case "delete":
				folder = "TRASH"
			}
			// Break early or continue depending on whether you want rules to chain.
			// Here we take the first matching rule.
			break
		}
	}

	return folder, isSpam
}
