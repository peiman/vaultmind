package embedding

// ONNX Runtime's CPU memory arena and memory-pattern planning stay off. With
// them on, ORT keeps a buffer for every input shape it has seen and never
// gives it back, so a long-lived process (vaultmind serve, a long index pass)
// grows with each new input length: measured +300 MB over 20 asks, and 56 GB
// in one resident server whose hooks asked on every prompt (each new prompt
// length a new shape the arena kept). The ORT session applies these (ortMemoryOptions);
// they live here, untagged, so the default test run pins them.
const (
	ortCPUMemArena = false //nolint:unused // read by the ORT-tagged session (session_ort.go)
	ortMemPattern  = false //nolint:unused // read by the ORT-tagged session (session_ort.go)
)
