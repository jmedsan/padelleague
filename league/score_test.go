package league

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseScore(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		score   string
		sets1   int
		sets2   int
		games1  int
		games2  int
		wantErr bool
	}{
		{"two-one", "6-3 4-6 7-5", 2, 1, 17, 14, false},
		{"straight sets", "6-3 6-4", 2, 0, 12, 7, false},
		{"opponent wins", "3-6 6-4 5-7", 1, 2, 14, 17, false},
		{"tiebreaks", "7-6(5) 6-7(3) 6-4", 2, 1, 19, 17, false},
		{"walkover", "WO", 0, 0, 0, 0, false},
		{"walkover lowercase", "wo", 0, 0, 0, 0, false},
		{"empty", "", 0, 0, 0, 0, true},
		{"invalid", "abc", 0, 0, 0, 0, true},
		{"tied set", "6-6 6-4", 0, 0, 0, 0, true},

		{"invalid set: 7-0", "7-0 6-3", 0, 0, 0, 0, true},
		{"invalid set: 7-1", "7-1 6-3", 0, 0, 0, 0, true},
		{"invalid set: 7-2", "7-2 6-3", 0, 0, 0, 0, true},
		{"invalid set: 7-3", "7-3 6-3", 0, 0, 0, 0, true},
		{"invalid set: 7-4", "7-4 6-3", 0, 0, 0, 0, true},
		{"invalid set: 0-7", "6-3 0-7", 0, 0, 0, 0, true},
		{"valid set: 7-5", "7-5 6-3", 2, 0, 13, 8, false},
		{"valid set: 7-6", "7-6 6-4", 2, 0, 13, 10, false},
		{"valid set: 5-7", "6-3 5-7 6-4", 2, 1, 17, 14, false},
		{"valid set: 6-7", "6-3 6-7 6-4", 2, 1, 18, 14, false},

		{"invalid set: 6-8", "6-8 6-3", 0, 0, 0, 0, true},
		{"invalid set: 8-6", "8-6 6-3", 0, 0, 0, 0, true},

		{"invalid: 1 set", "6-3", 0, 0, 0, 0, true},
		{"invalid: 4 sets", "6-3 3-6 6-4 6-2", 0, 0, 0, 0, true},

		{"already decided: winner has 3 sets", "6-3 6-4 6-2", 0, 0, 0, 0, true},
		{"already decided: extra loss set", "6-3 6-4 3-6", 0, 0, 0, 0, true},
		{"already decided: side-2 variant", "3-6 4-6 2-1", 0, 0, 0, 0, true},

		{"invalid set: 6-5", "6-5 6-3", 0, 0, 0, 0, true},
		{"tied 7-7", "7-7 6-3", 0, 0, 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc, err := ParseScore(tt.score)
			if tt.wantErr {
				assert.Error(t, err, "ParseScore(%q) should error", tt.score)
				return
			}
			assert.NoError(t, err, "ParseScore(%q) should not error", tt.score)
			assert.Equal(t, tt.sets1, sc.Sets1, "sets1")
			assert.Equal(t, tt.sets2, sc.Sets2, "sets2")
			assert.Equal(t, tt.games1, sc.Games1, "games1")
			assert.Equal(t, tt.games2, sc.Games2, "games2")
		})
	}
}

func TestParseScoreMode_AllowOpenSet(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		score   string
		sets1   int
		sets2   int
		games1  int
		games2  int
		openG1  int
		openG2  int
		hasOpen bool
		wantErr bool
	}{
		{"rule win", "6-3 4-1", 1, 0, 10, 4, 4, 1, true, false},
		{"tied open set", "6-3 3-3", 1, 0, 9, 6, 3, 3, true, false},
		{"one completed set only", "6-3", 1, 0, 6, 3, 0, 0, false, false},
		{"1-1 no open", "6-3 3-6", 1, 1, 9, 9, 0, 0, false, false},
		{"trailing 0-0 dropped", "6-3 0-0", 1, 0, 6, 3, 0, 0, false, false},
		{"open not last", "4-1 6-3", 0, 0, 0, 0, 0, 0, false, true},
		{"already won with open", "6-3 6-4 2-1", 0, 0, 0, 0, 0, 0, false, true},
		{"7-4 invalid open", "6-3 7-4", 0, 0, 0, 0, 0, 0, false, true},
		{"6-6 open tiebreak pending", "6-3 6-6", 1, 0, 12, 9, 6, 6, true, false},
		{"third set open", "6-3 3-6 5-2", 1, 1, 14, 11, 5, 2, true, false},
		{"third set open close", "6-3 3-6 4-2", 1, 1, 13, 11, 4, 2, true, false},
		{"side 2 open lead", "4-6 3-0", 0, 1, 7, 6, 3, 0, true, false},
		{"tiebreak stripped carried", "7-6(5) 3-3", 1, 0, 10, 9, 3, 3, true, false},
		{"0-0 alone error", "0-0", 0, 0, 0, 0, 0, 0, false, true},
		{"4 tokens error", "6-3 3-6 6-4 6-1", 0, 0, 0, 0, 0, 0, false, true},
		{"7-7 error", "6-3 7-7", 0, 0, 0, 0, 0, 0, false, true},
		{"already decided open side-2", "3-6 4-6 2-1", 0, 0, 0, 0, 0, 0, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc, err := ParseScoreMode(tt.score, AllowOpenSet)
			if tt.wantErr {
				assert.Error(t, err, "AllowOpenSet(%q) should error", tt.score)
				return
			}
			require.NoError(t, err, "AllowOpenSet(%q) should not error", tt.score)
			assert.Equal(t, tt.sets1, sc.Sets1, "sets1")
			assert.Equal(t, tt.sets2, sc.Sets2, "sets2")
			assert.Equal(t, tt.games1, sc.Games1, "games1")
			assert.Equal(t, tt.games2, sc.Games2, "games2")
			if tt.hasOpen {
				require.NotNil(t, sc.Open, "should have open set")
				assert.Equal(t, tt.openG1, sc.Open.G1, "open G1")
				assert.Equal(t, tt.openG2, sc.Open.G2, "open G2")
			} else {
				assert.Nil(t, sc.Open, "should not have open set")
			}
		})
	}
}

func TestParseScoreMode_StrictRejectsOpenSets(t *testing.T) {
	t.Parallel()
	partials := []string{"6-3 4-1", "6-3 3-3", "6-3", "6-3 3-6 5-2", "6-3 6-6"}
	for _, score := range partials {
		t.Run(score, func(t *testing.T) {
			_, err := ParseScore(score)
			assert.Error(t, err, "strict mode should reject %q", score)
		})
	}
}

func TestParseScoreMode_AlreadyDecidedBothModes(t *testing.T) {
	t.Parallel()
	cases := []string{"6-3 6-4 6-2", "6-3 6-4 3-6", "3-6 4-6 2-1"}
	for _, score := range cases {
		t.Run("strict/"+score, func(t *testing.T) {
			_, err := ParseScore(score)
			assert.Error(t, err)
		})
		t.Run("lenient/"+score, func(t *testing.T) {
			_, err := ParseScoreMode(score, AllowOpenSet)
			assert.Error(t, err)
		})
	}
}

func TestEvaluateScore(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		score   string
		won     bool
		winner  int
		carried string
	}{
		{"complete 2-0", "6-3 6-4", true, 1, ""},
		{"rule win 3-game lead", "6-3 4-1", true, 1, ""},
		{"third set rule win", "6-3 3-6 5-2", true, 1, ""},
		{"third set close no win", "6-3 3-6 4-2", false, 0, "6-3 3-6"},
		{"side 2 no sets no win", "4-6 3-0", false, 0, "4-6"},
		{"small lead no win", "6-3 2-1", false, 0, "6-3"},
		{"tied open no win", "6-3 3-3", false, 0, "6-3"},
		{"1-1 no open no win", "6-3 3-6", false, 0, "6-3 3-6"},
		{"one set only no win", "6-3", false, 0, "6-3"},
		{"side 2 wins complete", "3-6 0-6", true, 2, ""},
		{"side 2 rule win", "3-6 1-4", true, 2, ""},
		{"side 2 lead but no set", "6-3 1-4", false, 0, "6-3"},
		{"diff 2 not enough", "6-3 4-2", false, 0, "6-3"},
		{"tiebreak stripped carried", "7-6(5) 3-3", false, 0, "7-6"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc, err := ParseScoreMode(tt.score, AllowOpenSet)
			require.NoError(t, err)
			out := EvaluateScore(sc)
			assert.Equal(t, tt.won, out.Won, "Won")
			assert.Equal(t, tt.winner, out.WinnerSide, "WinnerSide")
			assert.Equal(t, tt.carried, out.Carried, "Carried")
		})
	}
}

func TestDetermineWinner_NoWinner(t *testing.T) {
	t.Parallel()
	_, err := DetermineWinner(nil, "6-3 2-1")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoWinner))
}

func TestTallyScore(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		score  string
		sets1  int
		sets2  int
		games1 int
		games2 int
		ok     bool
	}{
		{"rule win awards set", "6-3 4-1", 2, 0, 10, 4, true},
		{"complete score", "6-3 6-4", 2, 0, 12, 7, true},
		{"tied open no award", "6-3 3-3", 1, 0, 9, 6, true},
		{"walkover", "WO", 0, 0, 0, 0, false},
		{"side 2 rule win", "3-6 1-4", 0, 2, 4, 10, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc, ok := TallyScore(tt.score)
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.sets1, sc.Sets1, "sets1")
				assert.Equal(t, tt.sets2, sc.Sets2, "sets2")
				assert.Equal(t, tt.games1, sc.Games1, "games1")
				assert.Equal(t, tt.games2, sc.Games2, "games2")
				assert.Nil(t, sc.Open, "tally should clear open set")
			}
		})
	}
}

func TestScoreNote(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		score string
		want  string
	}{
		{"complete", "6-3 6-4", ""},
		{"rule win", "6-3 4-1", ""},
		{"no winner", "6-3 2-1", "No terminado · se reanudará otro día"},
		{"no winner 1-1", "6-3 3-6", "No terminado · se reanudará otro día"},
		{"walkover", "WO", ""},
		{"garbage", "garbage", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ScoreNote(tt.score))
		})
	}
}

func TestDetermineWinner_WalkoverError(t *testing.T) {
	t.Parallel()
	_, err := DetermineWinner(nil, "WO")
	assert.ErrorContains(t, err, "walkover")
}

func TestDetermineWinner_InvalidScore(t *testing.T) {
	t.Parallel()
	_, err := DetermineWinner(nil, "invalid")
	assert.Error(t, err)
}
