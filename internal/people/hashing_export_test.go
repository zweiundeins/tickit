package people

// HashingPeak is the most hashes that ran at once since ResetHashingPeak.
func HashingPeak() int { return int(hashingPeak.Load()) }

func ResetHashingPeak() { hashingPeak.Store(0) }
