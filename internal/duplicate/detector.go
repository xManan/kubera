package duplicate

import (
	"time"

	"kubera/internal/domain"
)

// Tolerance is the documented half-open window around the candidate
// occurrence time used for non-reference matching. 5 minutes covers
// notification retries and re-forwards.
const Tolerance = 5 * time.Minute

// Candidate is a normalized transaction under duplicate evaluation.
type Candidate struct {
	Direction             domain.Direction
	AmountMinor           int64
	Currency              domain.CurrencyCode
	OccurredAt            time.Time
	NormalizedDescription string
	ReferenceID           string
}

// DuplicateMatchReason values returned with duplicate results.
const (
	MatchReasonReference = "reference_id"
	MatchReasonFields    = "amount_currency_direction_description_time"
)
