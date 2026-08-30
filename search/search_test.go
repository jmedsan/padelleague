package search

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFold(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{"Otoño 2025", "otono 2025"},
		{"Vídeo de normas", "video de normas"},
		{"Reglamento", "reglamento"},
		{"PADEL 360", "padel 360"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, fold(tc.in), "fold(%q)", tc.in)
	}
}

func TestRankTypoAccentMatrix(t *testing.T) {
	t.Parallel()
	entries := []Entry{
		NewEntry(Entry{Label: "Reglamento", Type: "documento", URL: "/doc/1", Scope: Scope{Public: true}}),
		NewEntry(Entry{Label: "Padel 360", Type: "pista", URL: "/venues/1", Scope: Scope{Admin: true}}),
		NewEntry(Entry{Label: "Vídeo de normas", Type: "documento", URL: "/doc/2", Scope: Scope{Public: true}}),
		NewEntry(Entry{Label: "Otoño 2025", Type: "competición", URL: "/comp/1", Scope: Scope{Public: true}}),
		NewEntry(Entry{Label: "Javier Medina", Type: "jugador", URL: "/player/1", Scope: Scope{Public: true}}),
	}

	ix := &Index{}
	ix.Replace(entries)
	admin := Viewer{IsAdmin: true, CompIDs: map[string]struct{}{}}

	tests := []struct {
		query string
		want  string
	}{
		{"reglemento", "Reglamento"},
		{"padle", "Padel 360"},
		{"video norma", "Vídeo de normas"},
		{"otono", "Otoño 2025"},
		{"javi", "Javier Medina"},
	}
	for _, tc := range tests {
		results := ix.Search(tc.query, admin, 10)
		require.NotEmpty(t, results, "query %q must return results", tc.query)
		assert.Equal(t, tc.want, results[0].Label, "query %q top result", tc.query)
	}
}

func TestScopeFilterPlayer(t *testing.T) {
	t.Parallel()
	entries := []Entry{
		NewEntry(Entry{Label: "Public Page", Type: "página", URL: "/", Scope: Scope{Public: true}}),
		NewEntry(Entry{Label: "Admin Panel", Type: "página", URL: "/admin", Scope: Scope{Admin: true}}),
		NewEntry(Entry{Label: "Comp A Thread", Type: "mensaje", URL: "/match/1", Scope: Scope{CompID: "comp-a"}}),
		NewEntry(Entry{Label: "Comp B Thread", Type: "mensaje", URL: "/match/2", Scope: Scope{CompID: "comp-b"}}),
	}
	ix := &Index{}
	ix.Replace(entries)

	player := Viewer{
		IsAdmin: false,
		CompIDs: map[string]struct{}{"comp-a": {}},
	}

	results := ix.Search("p", player, 10)
	var labels []string
	for _, r := range results {
		labels = append(labels, r.Label)
	}

	assert.Contains(t, labels, "Public Page")
	assert.Contains(t, labels, "Comp A Thread")
	assert.NotContains(t, labels, "Admin Panel", "player must not see admin entries")
	assert.NotContains(t, labels, "Comp B Thread", "player must not see foreign comp entries")
}

func TestScopeFilterAdmin(t *testing.T) {
	t.Parallel()
	entries := []Entry{
		NewEntry(Entry{Label: "Public Page", Type: "página", URL: "/", Scope: Scope{Public: true}}),
		NewEntry(Entry{Label: "Admin Panel", Type: "página", URL: "/admin", Scope: Scope{Admin: true}}),
		NewEntry(Entry{Label: "Comp A Thread", Type: "mensaje", URL: "/match/1", Scope: Scope{CompID: "comp-a"}}),
		NewEntry(Entry{Label: "Comp B Thread", Type: "mensaje", URL: "/match/2", Scope: Scope{CompID: "comp-b"}}),
	}
	ix := &Index{}
	ix.Replace(entries)

	admin := Viewer{IsAdmin: true, CompIDs: map[string]struct{}{}}

	results := ix.Search("p", admin, 10)
	var labels []string
	for _, r := range results {
		labels = append(labels, r.Label)
	}

	assert.Contains(t, labels, "Public Page")
	assert.Contains(t, labels, "Admin Panel")
	assert.Contains(t, labels, "Comp A Thread")
	assert.Contains(t, labels, "Comp B Thread")
}

func TestSearchLimit(t *testing.T) {
	t.Parallel()
	var entries []Entry
	for i := 0; i < 20; i++ {
		entries = append(entries, NewEntry(Entry{Label: "Test Entry", Type: "página", URL: "/", Scope: Scope{Public: true}}))
	}
	ix := &Index{}
	ix.Replace(entries)

	results := ix.Search("test", Viewer{}, 5)
	assert.Len(t, results, 5)
}

func TestSearchEmptyQuery(t *testing.T) {
	t.Parallel()
	ix := &Index{}
	ix.Replace([]Entry{NewEntry(Entry{Label: "Foo", Type: "página", URL: "/", Scope: Scope{Public: true}})})
	results := ix.Search("", Viewer{}, 10)
	assert.Empty(t, results)
}

func TestPenaltySearchAdminOnlyScope(t *testing.T) {
	t.Parallel()
	entries := []Entry{
		NewEntry(Entry{Label: "Falta grave", Type: "penalización", URL: "/admin/competitions/c1", Scope: Scope{Admin: true}}),
		NewEntry(Entry{Label: "Public comp", Type: "competición", URL: "/comp/c1", Scope: Scope{Public: true}}),
	}
	ix := &Index{}
	ix.Replace(entries)

	admin := Viewer{IsAdmin: true, CompIDs: map[string]struct{}{}}
	player := Viewer{IsAdmin: false, CompIDs: map[string]struct{}{"c1": {}}}

	adminResults := ix.Search("falta", admin, 10)
	require.Len(t, adminResults, 1, "admin must find penalty")
	assert.Equal(t, "Falta grave", adminResults[0].Label)

	playerResults := ix.Search("falta", player, 10)
	assert.Empty(t, playerResults, "player must not see penalty search entries")
}

func TestVisibleTo(t *testing.T) {
	t.Parallel()

	player := Viewer{IsAdmin: false, CompIDs: map[string]struct{}{"c1": {}}}
	admin := Viewer{IsAdmin: true, CompIDs: map[string]struct{}{}}

	pub := Entry{Scope: Scope{Public: true}}
	adm := Entry{Scope: Scope{Admin: true}}
	ownComp := Entry{Scope: Scope{CompID: "c1"}}
	foreignComp := Entry{Scope: Scope{CompID: "c2"}}

	assert.True(t, pub.visibleTo(player))
	assert.False(t, adm.visibleTo(player))
	assert.True(t, ownComp.visibleTo(player))
	assert.False(t, foreignComp.visibleTo(player))

	assert.True(t, pub.visibleTo(admin))
	assert.True(t, adm.visibleTo(admin))
	assert.True(t, ownComp.visibleTo(admin))
	assert.True(t, foreignComp.visibleTo(admin))
}

func TestKeywordSearch(t *testing.T) {
	t.Parallel()
	entries := []Entry{
		NewEntry(Entry{Label: "A vs B (J1)", Type: "partido", URL: "/match/1", Keywords: []string{"partido", "jornada 1"}, Scope: Scope{Public: true}}),
		NewEntry(Entry{Label: "Pendientes", Secondary: "Partidos pendientes", Type: "página", URL: "/admin/outstanding", Keywords: []string{"partido", "partidos", "jornada"}, Scope: Scope{Admin: true}}),
		NewEntry(Entry{Label: "Penalizaciones", Secondary: "Gestión de penalizaciones", Type: "página", URL: "/admin", Keywords: []string{"penalización", "walkover"}, Scope: Scope{Admin: true}}),
		NewEntry(Entry{Label: "Clasificación", Secondary: "Tabla de clasificación", Type: "página", URL: "/", Keywords: []string{"ranking", "tabla"}, Scope: Scope{Public: true}}),
	}
	ix := &Index{}
	ix.Replace(entries)
	admin := Viewer{IsAdmin: true, CompIDs: map[string]struct{}{}}

	tests := []struct {
		query string
		want  string
	}{
		{"partidos", "Pendientes"},
		{"jornada", "Pendientes"},
		{"penalización", "Penalizaciones"},
		{"walkover", "Penalizaciones"},
		{"ranking", "Clasificación"},
	}
	for _, tc := range tests {
		results := ix.Search(tc.query, admin, 10)
		require.NotEmpty(t, results, "query %q must return results", tc.query)
		found := false
		for _, r := range results {
			if r.Label == tc.want {
				found = true
				break
			}
		}
		assert.True(t, found, "query %q must find %q in results", tc.query, tc.want)
	}
}

func TestSecondarySearch(t *testing.T) {
	t.Parallel()
	entries := []Entry{
		NewEntry(Entry{Label: "Pendientes", Secondary: "Partidos pendientes", Type: "página", URL: "/admin/outstanding", Scope: Scope{Admin: true}}),
	}
	ix := &Index{}
	ix.Replace(entries)
	admin := Viewer{IsAdmin: true, CompIDs: map[string]struct{}{}}

	results := ix.Search("partidos", admin, 10)
	require.NotEmpty(t, results, "secondary text 'partidos' must match")
	assert.Equal(t, "Pendientes", results[0].Label)
}

func TestPlayerPistasNoSpuriousMatch(t *testing.T) {
	t.Parallel()
	entries := []Entry{
		NewEntry(Entry{Label: "Pistas", Secondary: "Gestión de pistas", Type: "página", URL: "/admin/venues", Keywords: []string{"pista"}, Scope: Scope{Admin: true}}),
		NewEntry(Entry{Label: "Partidos", Secondary: "Todos los partidos", Type: "página", URL: "/", Keywords: []string{"partido", "partidos"}, Scope: Scope{Public: true}}),
	}
	ix := &Index{}
	ix.Replace(entries)
	player := Viewer{IsAdmin: false, CompIDs: map[string]struct{}{}}

	results := ix.Search("Pistas", player, 10)
	assert.Empty(t, results, "player searching 'Pistas' must get no results (admin entry filtered, no spurious public match)")
}

func TestLabelMatchOutranksKeyword(t *testing.T) {
	t.Parallel()
	entries := []Entry{
		NewEntry(Entry{Label: "Partidos", Type: "página", URL: "/partidos", Keywords: []string{"partido"}, Scope: Scope{Public: true}}),
		NewEntry(Entry{Label: "Pendientes", Type: "página", URL: "/admin/outstanding", Keywords: []string{"partido", "partidos"}, Scope: Scope{Admin: true}}),
	}
	ix := &Index{}
	ix.Replace(entries)
	admin := Viewer{IsAdmin: true, CompIDs: map[string]struct{}{}}

	results := ix.Search("partidos", admin, 10)
	require.Len(t, results, 2)
	assert.Equal(t, "Partidos", results[0].Label, "label match must rank above keyword match")
}
