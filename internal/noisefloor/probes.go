package noisefloor

// DefaultProbes is the fixed set of off-topic strings used to measure a vault's
// noise floor N: each is embedded and scored against every note, and N is the
// max cosine any of them reaches. They span diverse, mundane semantic regions
// no personal-knowledge vault should match, so their ceiling estimates "the
// cosine garbage gets" rather than a lucky single outlier. Fixed and versioned
// (changing the set changes N, so a recalibration is required) — never stored,
// never exported; only the resulting float matters.
var DefaultProbes = []string{
	"the weather forecast for next tuesday afternoon",
	"how to bake chocolate chip cookies from scratch",
	"final score of last night's football match",
	"today's stock market closing prices",
	"instructions for assembling flat-pack furniture",
	"the lyrics to the happy birthday song",
	"train timetable between two european cities",
	"the chemical formula for ordinary table salt",
}

// ProbeSetVersion identifies the probe set above. Bump it when the strings
// change so stored calibrations measured against an old set are distinguishable.
const ProbeSetVersion = 1
