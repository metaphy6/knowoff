package media

import "github.com/knowoff/knowoff/server/pkg/gamecontract"

const TextSchemaVersion = 2
const TextSuitabilityVersion = "reviewed-bands-v1"

// TextLimits are supplied from gameplay tuning, including offline tool runs.
type TextLimits struct {
	MaxTextBytes   int
	MaxRecords     int
	MaxFileBytes   int64
	MaxBundleBytes int64
}

// TextProvenance binds the accepted wording to its original immutable input.
// References identify privileged review records; they are never player DTOs.
type TextProvenance struct {
	SourceKind         string `json:"source_kind"`
	SourceID           string `json:"source_id"`
	SourceRevision     uint64 `json:"source_revision"`
	AcceptedTextSHA256 string `json:"accepted_text_sha256"`
	TermsVersion       string `json:"terms_version"`
	ConsentReference   string `json:"consent_reference"`
	ConsentAtMS        int64  `json:"consent_at_ms"`
	ApprovalReference  string `json:"approval_reference"`
	EditorReference    string `json:"editor_reference"`
	ReviewedAtMS       int64  `json:"reviewed_at_ms"`
	License            string `json:"license"`
	Attribution        string `json:"attribution"`
}

type TextNown struct {
	ID         string                `json:"id"`
	Revision   uint64                `json:"revision"`
	Type       string                `json:"type"`
	Text       string                `json:"text"`
	Kind       string                `json:"kind"`
	Modes      []gamecontract.ModeID `json:"modes"`
	ToneBucket string                `json:"tone_bucket"`
	Provenance TextProvenance        `json:"provenance"`
}

type TextCard struct {
	ID         string                `json:"id"`
	Revision   uint64                `json:"revision"`
	Type       string                `json:"type"`
	Text       string                `json:"text"`
	Pool       string                `json:"pool"`
	Modes      []gamecontract.ModeID `json:"modes"`
	ToneBucket string                `json:"tone_bucket"`
	Provenance TextProvenance        `json:"provenance"`
}

// Bands are reviewed relation evidence, never a semantic action validator.
type TextSuitability struct {
	Mode            gamecontract.ModeID `json:"mode"`
	NownID          string              `json:"nown_id"`
	NownRevision    uint64              `json:"nown_revision"`
	CardID          string              `json:"card_id"`
	CardRevision    uint64              `json:"card_revision"`
	Band            string              `json:"band"`
	ReviewReference string              `json:"review_reference"`
}

type TextEvaluator struct {
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	Version        string `json:"version"`
	EvidenceSHA256 string `json:"evidence_sha256"`
}

type TextDuplicateReview struct {
	FirstID         string `json:"first_id"`
	SecondID        string `json:"second_id"`
	ReviewReference string `json:"review_reference"`
}

type TextManifest struct {
	SchemaVersion          int                   `json:"schema_version"`
	ReleaseID              string                `json:"release_id"`
	Language               string                `json:"language"`
	RulesVersion           string                `json:"rules_version"`
	Modes                  []gamecontract.ModeID `json:"modes"`
	SuitabilityVersion     string                `json:"suitability_version"`
	AgeRating              string                `json:"age_rating"`
	Synthetic              bool                  `json:"synthetic"`
	Checksums              map[string]string     `json:"checksums"`
	CertificationArtifacts map[string]string     `json:"certification_artifacts"`
	DuplicateReviews       []TextDuplicateReview `json:"duplicate_reviews"`
	Evaluator              *TextEvaluator        `json:"evaluator,omitempty"`
}

type TextBundle struct {
	Manifest    TextManifest      `json:"manifest"`
	Nowns       []TextNown        `json:"nowns"`
	Cards       []TextCard        `json:"cards"`
	Suitability []TextSuitability `json:"suitability"`
	Artifacts   map[string][]byte `json:"artifacts"`
}

// A snapshot's storage is private. Accessors and deals return independent copies.
type TextSnapshot struct {
	bundle       TextBundle
	hash         string
	manifestHash string
	nowns        map[string]int
	cards        map[string]int
	bands        map[string]string
}

type TextDealTuning struct{ HandSize, ReserveSize, MinHigh, MinDistant, MaxSearchNodes int }

// Randomness is privileged replay input. Independent domains ensure public seeds
// are unaffected by schedule selection, hands or later role assignment.
type TextRandomness struct{ Schedule, Hands, System int64 }
type TextHand struct{ Cards, Reserve []TextCard }

// TextDeal is server-private. The engine allocates match-unique physical copies
// and emits authorized projections, never this full schedule/reserve/metadata.
type TextDeal struct {
	ReleaseID, Language, RulesVersion, SnapshotSHA256 string
	Nowns                                             []TextNown
	Hands                                             []TextHand
	SystemSeeds                                       [][]TextCard
}
