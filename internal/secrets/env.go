package secrets

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// Load parses a KEY=VALUE file (shell-env-style, one pair per line, blank lines
// and `#` comments ignored). Missing file returns an empty map, not an error —
// presets with no required secrets still work.
func Load(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := map[string]string{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.IndexByte(line, '=')
		if i <= 0 {
			return nil, fmt.Errorf("malformed line in %s: %q (want KEY=VALUE)", path, line)
		}
		k := strings.TrimSpace(line[:i])
		v := strings.TrimSpace(line[i+1:])
		v = strings.Trim(v, `"'`)
		out[k] = v
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Require returns an error listing every key from `keys` that is missing from
// the supplied map or present but empty. Nil when everything is present.
func Require(got map[string]string, keys ...string) error {
	var missing []string
	for _, k := range keys {
		if v, ok := got[k]; !ok || v == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("missing required secrets: %s", strings.Join(missing, ", "))
}

// EnsureSecureMode warns (via the supplied log func) if the file exists with
// group/world-readable bits set. Secrets files should be mode 0600.
func EnsureSecureMode(path string, warn func(string)) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	if info.Mode().Perm()&0o077 != 0 {
		warn(fmt.Sprintf("secrets file %s has permissive mode %o; recommend `chmod 600 %s`", path, info.Mode().Perm(), path))
	}
}
