package prompt

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/howeyc/gopass"
)

// String prompt.
func String(prompt string, args ...interface{}) string {
	fmt.Printf(prompt, args...)
	s, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimRight(s, "\r\n")
}

// StringRequired prompt (required).
func StringRequired(prompt string, args ...interface{}) string {
	s := String(prompt, args...)

	if strings.Trim(s, " ") == "" {
		return StringRequired(prompt)
	}

	return s
}

// Confirm continues prompting until the input is boolean-ish.
func Confirm(prompt string, args ...interface{}) bool {
	s := String(prompt, args...)
	switch s {
	case "yes", "y", "Y":
		return true
	case "no", "n", "N":
		return false
	default:
		return Confirm(prompt, args...)
	}
}

// Choose prompts for a single selection from `list`, returning in the index.
func Choose(prompt string, list []string) int {
	fmt.Println()
	for i, val := range list {
		fmt.Printf("  %d) %s\n", i+1, val)
	}

	fmt.Println()
	i := -1

	for {
		s := String(prompt)

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

// Password prompt.
func Password(prompt string, args ...interface{}) (string, error) {
	fmt.Printf(prompt, args...)

	b, err := gopass.GetPasswd()
	if err != nil {
		return "", err
	}

	return string(b), nil
}

// PasswordMasked prompt with mask.
func PasswordMasked(prompt string, args ...interface{}) (string, error) {
	fmt.Printf(prompt, args...)

	b, err := gopass.GetPasswdMasked()
	if err != nil {
		return "", err
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
