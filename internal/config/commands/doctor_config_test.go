package commands_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/stretchr/testify/assert"
)

func TestDoctorMetadata_Fields(t *testing.T) {
	meta := commands.DoctorMetadata
	assert.Equal(t, "doctor", meta.Use)
	assert.NotEmpty(t, meta.Short)
	assert.Equal(t, "app.doctor", meta.ConfigPrefix)
}

// The --all and --root flags must be wired into FlagOverrides so the registry
// auto-registers them as short --all/--root flags (not --app-doctor-all).
func TestDoctorMetadata_AllAndRootFlagOverrides(t *testing.T) {
	overrides := commands.DoctorMetadata.FlagOverrides
	assert.Equal(t, "all", overrides["app.doctor.all"], "--all flag override")
	assert.Equal(t, "root", overrides["app.doctor.root"], "--root flag override")
}

// The --all and --root config options must exist with the locked defaults:
// --all is an EXPLICIT opt-in (default false) and --root defaults to the
// current directory.
func TestDoctorOptions_AllAndRootDefaults(t *testing.T) {
	opts := commands.DoctorOptions()
	byKey := make(map[string]any, len(opts))
	for _, o := range opts {
		byKey[o.Key] = o.DefaultValue
	}
	assert.Equal(t, false, byKey["app.doctor.all"], "--all defaults to false (explicit opt-in)")
	assert.Equal(t, ".", byKey["app.doctor.root"], "--root defaults to the current directory")
}
