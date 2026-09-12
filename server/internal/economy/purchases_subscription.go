package economy

import (
	"context"
	"encoding/json"
	"time"
)

// VerifiedSubscription is current provider authority for one source. The
// separately retained purchase reference may describe an earlier billing period.
type VerifiedSubscription struct {
	SourceKey       string
	Current         VerifiedPurchase
	PredecessorKey  string
	RequestProducts []string
	RenewalSignedAt *time.Time
	Evidence        json.RawMessage
}

type SubscriptionReceiptVerifier interface {
	VerifySubscription(context.Context, ReceiptRequest) (VerifiedSubscription, error)
}

// SubscriptionSourceVerifier discovers configured current products for an
// already persisted source. This returns proof only; the worker must revalidate
// that exact existing source/account/application/environment before committing.
type SubscriptionSourceVerifier interface {
	ObserveSubscription(context.Context, ReceiptRequest) (VerifiedSubscription, error)
}
