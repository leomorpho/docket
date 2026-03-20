package agentrun

import (
	"fmt"
	"strings"
)

func parseKeyValueFields(input string) (map[string]string, error) {
	fields := map[string]string{}
	rest := strings.TrimSpace(input)
	for rest != "" {
		eq := strings.IndexByte(rest, '=')
		if eq <= 0 {
			return nil, fmt.Errorf("%w: malformed field %q", ErrInvalidResultLine, rest)
		}
		key := strings.TrimSpace(rest[:eq])
		rest = rest[eq+1:]
		if key == "" {
			return nil, fmt.Errorf("%w: empty key", ErrInvalidResultLine)
		}

		value := ""
		if strings.HasPrefix(rest, `"`) {
			rest = rest[1:]
			var b strings.Builder
			escaped := false
			closed := false
			for i, r := range rest {
				if escaped {
					b.WriteRune(r)
					escaped = false
					continue
				}
				switch r {
				case '\\':
					escaped = true
				case '"':
					value = b.String()
					rest = rest[i+1:]
					closed = true
					goto parsed
				default:
					b.WriteRune(r)
				}
			}
			if !closed {
				return nil, fmt.Errorf("%w: unterminated quoted value for %s", ErrInvalidResultLine, key)
			}
		} else {
			if idx := strings.IndexAny(rest, " \t"); idx >= 0 {
				value = rest[:idx]
				rest = rest[idx:]
			} else {
				value = rest
				rest = ""
			}
		}
	parsed:
		fields[key] = value
		rest = strings.TrimSpace(rest)
	}
	return fields, nil
}
