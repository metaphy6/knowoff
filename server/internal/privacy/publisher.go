// Package privacy defines the independent suppression receipt required before
// deletion confirmation. Application database backups cannot certify this receipt.
package privacy

import (
	"context"
	"time"
)

// SuppressionRequest is trusted, committed confirmation metadata. Publishers
// derive a purpose-specific selector and must never persist AccountID directly.
type SuppressionRequest struct {
	RequestID, AccountID string
	VerifiedAt           time.Time
	PolicyVersion        string
}

type SuppressionReceipt struct {
	Sequence int64
	Digest   [32]byte
}

// Publisher returns only after independent durable append. An uncertain error
// is retried with exactly the same request; it never authorizes a new request.
type Publisher interface {
	Publish(context.Context, SuppressionRequest) (SuppressionReceipt, error)
}
