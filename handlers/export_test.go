package handlers

import "github.com/pocketbase/pocketbase/core"

// export_test.go bridges unexported package internals to the external
// handlers_test package. It exists ONLY because Go forbids handlers' own
// internal test files (package handlers) from importing routes.Register —
// routes imports handlers, so that would be a cycle. Route-building tests
// live in package handlers_test instead and use setupProductionRoutes
// (see routing_test.go), which needs these bridges for whatever unexported
// helpers/internals it and its sibling test files still reach into.
//
// Add an entry here only when a test in package handlers_test needs an
// unexported symbol from a non-test file in this package. Do not add entries
// for symbols only used by tests that stayed in package handlers.

var (
	AlertError               = alertError
	BuildBracket             = buildBracket
	BuildHomeActions         = buildHomeActions
	BuildResetURL            = buildResetURL
	BuildShareText           = buildShareText
	CompressAvatar           = compressAvatar
	DetectFieldChange        = detectFieldChange
	FirstIncompleteRound     = firstIncompleteRound
	Flash                    = flash
	GenerateFlashMessage     = generateFlashMessage
	GetPaymentStatus         = getPaymentStatus
	ICSEscape                = icsEscape
	IsInviteExpired          = isInviteExpired
	PairPlayerLabel          = pairPlayerLabel
	PlayerNameIfSet          = playerNameIfSet
	PlayerTeamOf             = playerTeamOf
	ProposalActions          = proposalActions
	RedirectHX               = redirectHX
	StatusClass              = statusClass
	ValidateLeveledFields    = validateLeveledFields
	ValidatePlayerUniqueness = validatePlayerUniqueness
)

// Shared test fixture helpers, needed by both package handlers (tests that
// stayed — pure unit tests with no routes) and package handlers_test (tests
// that moved to build routes via setupProductionRoutes). Defined once in
// testutil_test.go; bridged here so both sides use the same fixtures instead
// of duplicating them.
var (
	MakeUserTB        = makeUserTB
	MakeUser          = makeUser
	TestAppFactory    = testAppFactory
	NewTestApp        = newTestApp
	MakePair          = makePair
	MakeCompetition   = makeCompetition
	MakeMatch         = makeMatch
	MakeFinalMatch    = makeFinalMatch
	AuthHeaders       = authHeaders
	AuthToken         = authToken
	MakePairTB        = makePairTB
	MakeCompetitionTB = makeCompetitionTB
	MakeMatchTB       = makeMatchTB
	MakeAdminUserTB   = makeAdminUserTB
	MakeVenueTB       = makeVenueTB
	MakeInvitationTB  = makeInvitationTB
	MakeDocumentTB    = makeDocumentTB
	MakePenaltyTB     = makePenaltyTB
	ReadBody          = readBody
	ExpectRedirect    = expectRedirect

	MakeProposal           = makeProposal
	MakeProposalWithStatus = makeProposalWithStatus
	MakeSchedulingResponse = makeSchedulingResponse
	MakeResultProposal     = makeResultProposal
)

// PairSeq and UserSeq bridge the package-level uniqueness counters so
// handlers_test fixtures that build on makeUserTB/makePairTB (e.g.
// makePairWithGendersTB, makeInvitation) draw from the same sequence and
// never collide with fixtures built from package handlers.
var PairSeq = &pairSeq
var UserSeq = &userSeq

// AvatarMaxUploadSize bridges the shared upload-size limit so tests that
// build an oversized payload (admin_competitions_logo_test.go,
// player_avatar_test.go) stay in sync with the real limit instead of
// hardcoding a copy that could drift.
const AvatarMaxUploadSize = avatarMaxUploadSize

// RegenerateFixturesTx bridges FixtureHandler's unexported transactional
// fixture-regeneration helper, used by handlers_test fixture setup
// (fixtures_test.go) to seed an initial calendar directly instead of driving
// it through an extra HTTP round trip.
func (h *FixtureHandler) RegenerateFixturesTx(txApp core.App, comp *core.Record, pairIDs []string, existingMatches []*core.Record) error {
	return h.regenerateFixturesTx(txApp, comp, pairIDs, existingMatches)
}

// BuildThreadData bridges ThreadHandler's unexported thread view-model
// builder, exercised directly by thread_test.go's route-driven tests that
// need to inspect the timeline after an HTTP action instead of re-parsing
// the rendered HTML.
func (h *ThreadHandler) BuildThreadData(match *core.Record, matchID string, viewerID string, myTeam int, compModifiable bool) ThreadData {
	return h.buildThreadData(match, matchID, threadViewerCtx{viewerID: viewerID, myTeam: myTeam, compModifiable: compModifiable})
}
