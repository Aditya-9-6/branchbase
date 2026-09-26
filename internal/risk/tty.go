package risk

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// ErrNonInteractiveTerminal is returned when an interactive confirmation is attempted in a non-TTY context.
var ErrNonInteractiveTerminal = errors.New("cannot request interactive confirmation in non-interactive environment (stdin is not a TTY); run with --force or in an interactive terminal")

// IsTerminal checks if the provided file descriptor is attached to a terminal/character device.
func IsTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// PromptConfirmation asks the user for a [y/N] confirmation via stdin and stdout.
// In non-interactive environments, it returns ErrNonInteractiveTerminal to avoid hanging.
func PromptConfirmation(r io.Reader, w io.Writer, prompt string) (bool, error) {
	if f, ok := r.(*os.File); ok {
		if !IsTerminal(f) {
			return false, ErrNonInteractiveTerminal
		}
	}

	fmt.Fprintf(w, "%s [y/N]: ", prompt)
	reader := bufio.NewReader(r)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read response: %w", err)
	}

	ans := strings.ToLower(strings.TrimSpace(input))
	return ans == "y" || ans == "yes", nil
}
