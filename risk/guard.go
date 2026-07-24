package risk

import (
	"errors"
	"fmt"

	"github.com/BullionBear/seq/core/model/command"
)

// Guard is a risk actor with pre-trade veto power.
// Implementors must also be actor.Actor — the same instance, not two objects.
type Guard interface {
	Check(cmd command.RiskCheck) error
}

// Stable rejection codes carried on OrderRiskInvalid.ErrorCode.
const (
	ErrCodeUnknown     = -1
	ErrCodeRateLimited = 1001
)

// Error is a risk rejection with a stable code for OrderRiskInvalid.
type Error struct {
	Code int
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

// CodeOf extracts a risk error code, or ErrCodeUnknown.
func CodeOf(err error) int {
	var re *Error
	if errors.As(err, &re) {
		return re.Code
	}
	return ErrCodeUnknown
}

// RateLimited returns a coded rate-limit rejection.
func RateLimited(waitMs uint64) error {
	return &Error{
		Code: ErrCodeRateLimited,
		Msg:  fmt.Sprintf("rate limited: next accepted in %d ms", waitMs),
	}
}
