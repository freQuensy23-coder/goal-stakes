// Package domain holds the core entity types and enums for goal-stakes.
// It has no dependencies on storage, transport, or chain layers — it is the
// single source of truth (GPC4) for the shapes every other backend package
// builds on (IF0).
package domain

import "fmt"

// GoalType distinguishes the two enforcement styles (AS4):
//   - Do: deadline-based — a missing check-in by period end is a violation.
//   - Avoid: self-reported — the user reports a slip, which is a violation.
type GoalType string

const (
	GoalDo    GoalType = "do"
	GoalAvoid GoalType = "avoid"
)

// Valid reports whether t is a recognized goal type.
func (t GoalType) Valid() bool {
	switch t {
	case GoalDo, GoalAvoid:
		return true
	default:
		return false
	}
}

// Cadence is how often a goal's period rolls over.
type Cadence string

const (
	CadenceDaily  Cadence = "daily"
	CadenceWeekly Cadence = "weekly"
	// CadenceCustom is reserved for user-defined schedules. v1 has no
	// automatic period math for it (CurrentPeriod returns "", PeriodBounds
	// errors) — callers must handle custom cadence explicitly.
	CadenceCustom Cadence = "custom"
)

// Valid reports whether c is a recognized cadence.
func (c Cadence) Valid() bool {
	switch c {
	case CadenceDaily, CadenceWeekly, CadenceCustom:
		return true
	default:
		return false
	}
}

// ViolationStatus tracks the on-chain charge lifecycle of a violation (IV6).
// A violation row is created Pending before any chain interaction; it then
// transitions to Charged (settled on-chain) or Failed (charge attempt failed).
type ViolationStatus string

const (
	ViolationPending ViolationStatus = "pending"
	ViolationCharged ViolationStatus = "charged"
	ViolationFailed  ViolationStatus = "failed"
)

// Valid reports whether s is a recognized violation status.
func (s ViolationStatus) Valid() bool {
	switch s {
	case ViolationPending, ViolationCharged, ViolationFailed:
		return true
	default:
		return false
	}
}

// MessageRole is the author of a conversation message.
type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleSystem    MessageRole = "system"
)

// Valid reports whether r is a recognized message role.
func (r MessageRole) Valid() bool {
	switch r {
	case RoleUser, RoleAssistant, RoleSystem:
		return true
	default:
		return false
	}
}

// String implements fmt.Stringer for friendlier logs/errors.
func (t GoalType) String() string        { return string(t) }
func (c Cadence) String() string         { return string(c) }
func (s ViolationStatus) String() string { return string(s) }
func (r MessageRole) String() string     { return string(r) }

// errInvalidPeriod is the shared sentinel-style wrapper for period parse/format
// failures. Callers get a descriptive, wrapped error (GPC6).
func errInvalidPeriod(format string, args ...any) error {
	return fmt.Errorf("domain: "+format, args...)
}
