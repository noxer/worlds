package gen

// Fallbacks exposes the autotiler's fallback counter to tests.
func Fallbacks() int64 { return fallbacks.Load() }
