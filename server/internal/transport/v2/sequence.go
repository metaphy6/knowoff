package v2

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/knowoff/knowoff/server/pkg/gamecontract"
)

// CheckNext applies only within the epoch established by an authoritative
// snapshot. A new connection must establish its new epoch with a snapshot;
// messages from any other epoch are ignored, never used to reset the cursor.
func CheckNext(current, next Cursor) error {
	if err := current.validate(); err != nil {
		return err
	}
	if err := next.validate(); err != nil {
		return err
	}
	if current.StreamEpoch != next.StreamEpoch {
		return invalid(ErrStaleStream, "stream_epoch")
	}
	if next.RecipientSeq <= current.RecipientSeq {
		return invalid(ErrDuplicateEvent, "recipient_seq")
	}
	if next.RecipientSeq != current.RecipientSeq+1 {
		return invalid(ErrSequenceGap, "recipient_seq")
	}
	if next.EvidenceSeq < current.EvidenceSeq {
		return invalid(ErrStaleEvidence, "evidence_seq")
	}
	if next.EvidenceSeq > current.EvidenceSeq+1 {
		return invalid(ErrSequenceGap, "evidence_seq")
	}
	return nil
}

// RequestRecord stores result identity, never a cached private response. The
// handler must reauthorize rendering when a duplicate request is answered.
type RequestRecord struct {
	MatchID       string
	Seat          int
	RequestID     string
	BodyHash      string
	ResultEventID string
}

func RequestHash(request ActionRequest) (string, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func CheckRequestReuse(previous RequestRecord, request ActionRequest, seat int) (bool, error) {
	if previous.MatchID != request.MatchID || previous.Seat != seat || previous.RequestID != request.RequestID {
		return false, nil
	}
	hash, err := RequestHash(request)
	if err != nil {
		return false, err
	}
	if hash != previous.BodyHash {
		return false, invalid(ErrRequestConflict, "request_id")
	}
	return true, nil
}

// CheckRequestBudget is evaluated after identity/conflict lookup. Replaying an
// existing outcome never consumes another slot; new request IDs are bounded.
func CheckRequestBudget(recorded int, duplicate bool, l Limits) error {
	if err := l.Validate(); err != nil {
		return err
	}
	if recorded < 0 {
		return invalid(ErrMalformed, "recorded_requests")
	}
	if !duplicate && recorded >= l.MaxRequestsPerSeat {
		return invalid(ErrRequestLimit, "requests")
	}
	return nil
}

// ActionContext is server state at the serialized validation boundary. The
// engine additionally checks actor/role/ownership/target and admission state.
type ActionContext struct {
	MatchID       string
	ModeID        gamecontract.ModeID
	Round         int
	Turn          int
	Phase         Phase
	PhaseID       string
	BoardRevision uint64
	DeadlineMS    int64
	NowMS         int64
}

func CheckActionContext(r ActionRequest, current ActionContext) error {
	if r.MatchID != current.MatchID || r.ModeID != current.ModeID {
		return invalid(ErrStaleMatch, "match_id")
	}
	if r.Round != current.Round || r.Turn != current.Turn || r.Phase != current.Phase || r.PhaseID != current.PhaseID {
		return invalid(ErrStalePhase, "phase")
	}
	if r.ExpectedBoardRevision != current.BoardRevision {
		return invalid(ErrStaleRevision, "expected_board_revision")
	}
	if !clock(current.DeadlineMS) || !clock(current.NowMS) || current.NowMS >= current.DeadlineMS {
		return invalid(ErrDeadlineExpired, "deadline_ms")
	}
	return nil
}
