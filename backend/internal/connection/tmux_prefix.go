package connection

import (
	"bufio"
	"context"
	"strings"
	"time"
)

const (
	defaultTmuxPrefixKey   = "b"
	tmuxPrefixProbeTries   = 6
	tmuxConfigProbeCommand = `if [ -r "$HOME/.tmux.conf" ]; then sed -n '1,512p' "$HOME/.tmux.conf"; fi`
)

type auxiliaryRunner func(context.Context, *Transport, ...string) ([]byte, error)

func probeTmuxPrefix(parent context.Context, manager *Manager, transport *Transport) (string, string) {
	return probeTmuxPrefixWithRunner(parent, transport, manager.runAuxiliary)
}

func probeTmuxPrefixWithRunner(parent context.Context, transport *Transport, run auxiliaryRunner) (string, string) {
	ctx, cancel := withAuxiliaryTimeout(parent)
	defer cancel()
	var output []byte
	var err error
	effectiveOutput := false
	for attempt := 0; attempt < tmuxPrefixProbeTries; attempt++ {
		output, err = run(ctx, transport, "tmux", "show-options", "-gv", "prefix")
		if err == nil || ctx.Err() != nil {
			effectiveOutput = err == nil
			break
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
		}
	}
	if effectiveOutput {
		if key, ok := normalizeTmuxPrefix(string(output)); ok {
			return key, "runtime"
		}
	}

	// The marker can arrive before tmux has started its server. In that case
	// inspect the user's remote config while retaining the standard C-b default.
	if ctx.Err() == nil {
		config, configErr := run(ctx, transport, "sh", "-c", shellQuote(tmuxConfigProbeCommand))
		if configErr == nil {
			if key, found, supported := parseTmuxConfigPrefix(string(config)); found {
				if supported {
					return key, "runtime"
				}
				return "", "unsupported"
			}
		}
	}
	if effectiveOutput {
		return "", "unsupported"
	}
	return defaultTmuxPrefixKey, "fallback"
}

func normalizeTmuxPrefix(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\r\n\t ") {
		return "", false
	}
	value = strings.ToLower(value)
	if !strings.HasPrefix(value, "c-") || len(value) != 3 {
		return "", false
	}
	key := value[2]
	if key < 'a' || key > 'z' {
		return "", false
	}
	return string(key), true
}

// parseTmuxConfigPrefix accepts only the simple global prefix forms that the
// UI can represent. Other tmux expressions remain unsupported rather than
// being guessed from arbitrary shell syntax.
func parseTmuxConfigPrefix(value string) (string, bool, bool) {
	scanner := bufio.NewScanner(strings.NewReader(value))
	var key string
	var found, supported bool
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 || (fields[0] != "set" && fields[0] != "set-option") {
			continue
		}
		optionIndex := 1
		for optionIndex < len(fields) && strings.HasPrefix(fields[optionIndex], "-") {
			optionIndex++
		}
		if optionIndex+1 >= len(fields) || strings.Trim(fields[optionIndex], "\"'") != "prefix" {
			continue
		}
		prefix := strings.Trim(fields[optionIndex+1], "\"'")
		prefix = strings.TrimPrefix(prefix, `\`)
		key, supported = normalizeTmuxPrefix(prefix)
		found = true
	}
	return key, found, supported
}
