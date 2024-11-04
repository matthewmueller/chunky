package prompt

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/howeyc/gopass"
)

// Prompter interface
type Prompter interface {
	String(prompt string, args ...interface{}) string
	StringRequired(prompt string, args ...interface{}) string
	Confirm(prompt string, args ...interface{}) bool
	Choose(prompt string, list []string) int
	Password(prompt string, args ...interface{}) (string, error)
}

// Default prompter
func Default() Prompter {
	return prompter{}
}

type prompter struct{}

// String prompt.
func (prompter) String(prompt string, args ...interface{}) string {
	fmt.Printf(prompt, args...)
	s, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimRight(s, "\r\n")
}

// StringRequired prompt (required).
func (p prompter) StringRequired(prompt string, args ...interface{}) string {
retry:
	s := p.String(prompt, args...)

	if strings.Trim(s, " ") == "" {
		goto retry
	}

	return s
}

// Confirm continues prompting until the input is boolean-ish.
func (p prompter) Confirm(prompt string, args ...interface{}) bool {
	s := p.String(prompt, args...)
	switch s {
	case "yes", "y", "Y":
		return true
	case "no", "n", "N":
		return false
	default:
		return p.Confirm(prompt, args...)
	}
}

// Choose prompts for a single selection from `list`, returning in the index.
func (p prompter) Choose(prompt string, list []string) int {
	fmt.Println()
	for i, val := range list {
		fmt.Printf("  %d) %s\n", i+1, val)
	}

	fmt.Println()
	i := -1

	for {
		s := p.String(prompt)

		// index
		n, err := strconv.Atoi(s)
		if err == nil {
			if n > 0 && n <= len(list) {
				i = n - 1
				break
			} else {
				continue
			}
		}

		// value
		i = indexOf(s, list)
		if i != -1 {
			break
		}
	}

	return i
}

// Password prompt with mask.
func (prompter) Password(prompt string, args ...interface{}) (string, error) {
retry:
	fmt.Printf(prompt, args...)

	b, err := gopass.GetPasswdMasked()
	if err != nil {
		return "", err
	} else if len(b) == 0 {
		goto retry
	}

	return string(b), nil
}

// index of `s` in `list`.
func indexOf(s string, list []string) int {
	for i, val := range list {
		if val == s {
			return i
		}
	}
	return -1
}
