package doc

// RuleError is a refusal by a server rule; its message is a sentence a
// person can read.
type RuleError struct{ Msg string }

func (e *RuleError) Error() string { return e.Msg }
