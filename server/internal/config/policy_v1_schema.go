package config

// This is the immutable text-tuning-v1 serialization schema, captured before
// executable v1 retirement. Field order, names, types and JSON omissions define
// existing match hashes. These types never load YAML or execute gameplay.
// A future serialized policy schema requires an explicit version transition.

type policyV1TuningConfig struct {
	TextCatalog policyV1TextCatalogTuning
	Contract    policyV1ContractTuning
	Seed        int
	Game        policyV1GameTuning
	Timers      policyV1TimersTuning
	Hand        policyV1HandTuning
	Dealing     policyV1DealingTuning
	Points      policyV1PointsTuning
	Noin        policyV1NoinTuning
	Economy     policyV1EconomyTuning
	Liquidity   policyV1LiquidityTuning
	LiveOps     policyV1LiveOpsTuning
	Portal      policyV1PortalTuning
	Progression policyV1ProgressionTuning
}

type policyV1TextCatalogTuning struct {
	MaxRecords     int
	MaxFileBytes   int64
	MaxBundleBytes int64
	MaxSearchNodes int
}

type policyV1ContractTuning struct {
	MaxHistoryEvents     int
	MaxHistoryPageEvents int
	MaxTextBytes         int
	MaxRequestsPerSeat   int
}

type policyV1GameTuning struct {
	RoomSizes              []int
	DonowersBySize         map[int]int
	VotesBySize            map[int]int
	MinConnected           int
	ReconnectGraceS        int
	PokesPerTargetPerRound int
	AbandonCooldownsS      []int
}

type policyV1TimersTuning struct {
	TradeResponseS      int
	RoundStartCountdown int
	PlayTurn            int
	DiscussionPerPlayer int
	KnowoffBallot       int
	KnowoffRunoff       int
	VoteResultWindow    int
	VoteResultFalling   int `json:"VoteResultFalling,omitempty"`
	RevealLockout       int
	RevealView          int
	ShuffleBonusSeconds int
	PrefetchCountdown   int
}

type policyV1HandTuning struct {
	Size             int
	DrawPile         int
	SpecialtyWeights map[string]float64
}

type policyV1DealingTuning struct {
	BandHigh          float64
	BandLow           float64
	MinHighPerNown    int
	MinDistantPerNown int
}

type policyV1PointsTuning struct {
	CorrectVote    int
	NowerWinBonus  int
	DonowerTeamWin int
	DrawPenalty    int
}

type policyV1NoinTuning struct {
	MatchCompleted           int
	NowerWin                 int
	DonowerTeamWin           int
	CorrectVote              int
	DonowerVoteSurvived      int
	DailyFirstWin            int
	DailyEarnCap             int
	ChallengeWinner          int
	ContributorAcceptedAsset int
}

type policyV1EconomyTuning struct {
	FreeDailyQuickplayMatches int
	PointsToNoin              int
	PlayPassPrices            map[string]int
	PremiumYearlyDiscountPct  int
	UnlockPrices              map[string]int
	NoinBundles               []int
}

type policyV1LiquidityTuning struct {
	BackfillEnabled      bool
	QueueTimeoutS        int
	MinHumans            int
	LeaderboardMinHumans int
	NoinMinHumans        int
	BotThinkMinS         float64
	BotThinkMaxS         float64
}

type policyV1LiveOpsTuning struct {
	LeaderboardDailyCountedMatches int
	ChallengeMaxEntries            int
	ChallengeVotesPerPlayer        int
}

type policyV1PortalTuning struct {
	MinAccountLevelToApply          int
	SubmissionsPerContributorPerDay int
	GuardFreezeMaxH                 int
	MaxTextSubmissionLength         int
	TermsVersion                    string
}

type policyV1ProgressionTuning struct {
	XPBase           int
	XPPerCorrectVote int
	XPWinBonus       int
	LevelThresholds  []int
}
