package commands_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/stretchr/testify/require"
)

// pflag treats the first backtick pair in a flag's usage as the metavar.
// `hooks status` therefore rendered as `--profile hooks status`. A backtick
// pair is allowed only when it is a single placeholder token.
func TestRegistryUsageBackticksAreMetavars(t *testing.T) {
	require.NotEmpty(t, commands.HooksInstallOptions(), "command options must be registered")
	metavar := regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)
	for _, opt := range config.Registry() {
		rest := opt.Description
		for {
			start := strings.IndexByte(rest, '`')
			if start < 0 {
				break
			}
			rest = rest[start+1:]
			end := strings.IndexByte(rest, '`')
			if end < 0 {
				t.Errorf("%s: unclosed backtick in %q", opt.Key, opt.Description)
				break
			}
			inside := rest[:end]
			if !metavar.MatchString(inside) {
				t.Errorf("%s: %q is not a metavar; pflag would use it as the flag's value name", opt.Key, inside)
			}
			rest = rest[end+1:]
		}
	}
}
