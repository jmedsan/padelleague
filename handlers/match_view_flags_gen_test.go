package handlers

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"
)

// HTML markers rendered when each flag is true.
const (
	markerCanSubmit   = "Registrar resultado"
	markerCanWalkover = "Reportar incomparecencia"
	markerCanConfirm  = "El rival ha enviado este resultado"
	markerCanCorrect  = "Corregir marcador"
	markerWaiting     = "Esperando confirmación del rival"
)

type matchViewCase struct {
	name         string
	status       string
	viewer       string // "submitter", "opponent", "outsider", "admin"
	submitted    bool
	recentSubmit bool
	httpStatus   int // 0 means 200
	want         []string
	deny         []string
}

func TestBuildMatchViewFlags(t *testing.T) {
	cases := []matchViewCase{
		// --- Pending ---
		{
			name: "pending/submitter-team", status: "pending", viewer: "submitter",
			want: []string{markerCanSubmit, markerCanWalkover},
			deny: []string{markerCanConfirm, markerCanCorrect},
		},
		{
			name: "pending/opponent", status: "pending", viewer: "opponent",
			want: []string{markerCanSubmit, markerCanWalkover},
			deny: []string{markerCanConfirm, markerCanCorrect},
		},
		{
			name: "pending/outsider", status: "pending", viewer: "outsider",
			httpStatus: 403,
			want:       []string{"No tienes acceso"},
		},
		{
			name: "pending/admin", status: "pending", viewer: "admin",
			deny: []string{markerCanSubmit, markerCanWalkover, markerCanConfirm, markerCanCorrect},
		},

		// --- Confirmed with submitter set ---
		{
			name: "confirmed/submitter/recent", status: "confirmed", viewer: "submitter",
			submitted: true, recentSubmit: true,
			want: []string{markerCanCorrect, markerWaiting},
			deny: []string{markerCanSubmit, markerCanConfirm, markerCanWalkover},
		},
		{
			name: "confirmed/submitter/expired", status: "confirmed", viewer: "submitter",
			submitted: true, recentSubmit: false,
			want: []string{markerWaiting},
			deny: []string{markerCanSubmit, markerCanConfirm, markerCanCorrect, markerCanWalkover},
		},
		{
			name: "confirmed/opponent", status: "confirmed", viewer: "opponent",
			submitted: true,
			want:      []string{markerCanConfirm},
			deny:      []string{markerCanSubmit, markerCanCorrect, markerCanWalkover, markerWaiting},
		},
		{
			name: "confirmed/outsider", status: "confirmed", viewer: "outsider",
			submitted:  true,
			httpStatus: 403,
			want:       []string{"No tienes acceso"},
		},
		{
			name: "confirmed/admin-nonparticipant", status: "confirmed", viewer: "admin",
			submitted: true,
			deny:      []string{markerCanSubmit, markerCanConfirm, markerCanCorrect, markerCanWalkover},
		},
		// submittedBy="" → isSubmitter=false → opponent sees CanConfirm (line 93 branch)
		{
			name: "confirmed/no-submitter/opponent", status: "confirmed", viewer: "opponent",
			submitted: false,
			want:      []string{markerCanConfirm},
			deny:      []string{markerCanSubmit, markerCanCorrect, markerCanWalkover},
		},

		// --- Disputed ---
		{
			name: "disputed/submitter", status: "disputed", viewer: "submitter",
			submitted: true,
			deny:      []string{markerCanSubmit, markerCanConfirm, markerCanCorrect, markerCanWalkover},
		},
		{
			name: "disputed/opponent", status: "disputed", viewer: "opponent",
			submitted: true,
			deny:      []string{markerCanSubmit, markerCanConfirm, markerCanCorrect, markerCanWalkover},
		},

		// --- Final ---
		{
			name: "final/submitter", status: "final", viewer: "submitter",
			submitted: true,
			deny:      []string{markerCanSubmit, markerCanConfirm, markerCanCorrect, markerCanWalkover},
		},
		{
			name: "final/opponent", status: "final", viewer: "opponent",
			submitted: true,
			deny:      []string{markerCanSubmit, markerCanConfirm, markerCanCorrect, markerCanWalkover},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expectedStatus := tc.httpStatus
			if expectedStatus == 0 {
				expectedStatus = 200
			}
			s := &tests.ApiScenario{
				TestAppFactory:     testAppFactory,
				Name:               tc.name,
				Method:             http.MethodGet,
				ExpectedStatus:     expectedStatus,
				ExpectedContent:    tc.want,
				NotExpectedContent: tc.deny,
			}

			s.BeforeTestFunc = func(tb testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				setupAllRoutes(tb, app, e)

				p1 := makePairTB(tb, app, fmt.Sprintf("MV%s A", tc.name[:3]))
				p2 := makePairTB(tb, app, fmt.Sprintf("MV%s B", tc.name[:3]))
				comp := makeCompetitionTB(tb, app, "league", []*core.Record{p1, p2})
				match := makeMatchTB(tb, app, comp.Id, p1.Id, p2.Id, tc.status)

				submitterUserID := p1.GetString("player1")

				if tc.submitted {
					match.Set("scores", "6-3 6-4")
					match.Set("submitted_by", submitterUserID)
					if tc.recentSubmit {
						match.SetRaw("submitted_at", time.Now().Add(-1*time.Hour).UTC().Format(time.RFC3339))
					} else {
						match.SetRaw("submitted_at", time.Now().Add(-25*time.Hour).UTC().Format(time.RFC3339))
					}
				}

				// Final status needs a winner to render properly
				if tc.status == "final" {
					match.Set("winner", p1.Id)
					match.Set("scores", "6-3 6-4")
				}

				require.NoError(tb, app.Save(match))

				s.URL = "/match/" + match.Id

				var viewerUser *core.Record
				switch tc.viewer {
				case "submitter":
					viewerUser, _ = app.FindRecordById("users", submitterUserID)
				case "opponent":
					opponentID := p2.GetString("player1")
					viewerUser, _ = app.FindRecordById("users", opponentID)
				case "outsider":
					viewerUser = makeUserTB(tb, app, "MV Outsider", "")
				case "admin":
					viewerUser = makeAdminUserTB(tb, app)
				}
				s.Headers = authHeaders(tb, viewerUser)
			}

			s.Test(t)
		})
	}
}
