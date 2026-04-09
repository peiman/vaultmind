package experiment

// reservedKeys are top-level keys in the experiments config section that are
// not experiment definitions.
var reservedKeys = map[string]bool{
	"telemetry":               true,
	"outcome_window_sessions": true,
}

// ExperimentDef holds the parsed definition for a single experiment.
type ExperimentDef struct {
	Enabled  bool
	Primary  string
	Shadows  []string
}

// AllVariants returns the primary variant followed by all shadow variants.
func (e ExperimentDef) AllVariants() []string {
	variants := make([]string, 0, 1+len(e.Shadows))
	variants = append(variants, e.Primary)
	variants = append(variants, e.Shadows...)
	return variants
}

// ParseExperiments parses experiment definitions from Viper's raw map.
// Reserved keys ("telemetry", "outcome_window_sessions") and non-map values
// are skipped. For each map entry it extracts enabled (bool), primary (string),
// and shadows ([]string from []any).
func ParseExperiments(raw map[string]any) map[string]ExperimentDef {
	result := make(map[string]ExperimentDef)
	for key, val := range raw {
		if reservedKeys[key] {
			continue
		}
		m, ok := val.(map[string]any)
		if !ok {
			continue
		}
		var def ExperimentDef
		if b, ok := m["enabled"].(bool); ok {
			def.Enabled = b
		}
		if s, ok := m["primary"].(string); ok {
			def.Primary = s
		}
		if arr, ok := m["shadows"].([]any); ok {
			for _, item := range arr {
				if s, ok := item.(string); ok {
					def.Shadows = append(def.Shadows, s)
				}
			}
		}
		result[key] = def
	}
	return result
}
