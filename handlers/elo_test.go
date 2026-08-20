package handlers

import "testing"

func TestComputeELO(t *testing.T) {
	tests := []struct {
		name      string
		winnerELO int
		loserELO  int
		wantW     int
		wantL     int
	}{
		{
			name:      "equal ratings",
			winnerELO: 1500,
			loserELO:  1500,
			wantW:     1516,
			wantL:     1484,
		},
		{
			name:      "strong beats weak",
			winnerELO: 1800,
			loserELO:  1200,
			wantW:     1801,
			wantL:     1199,
		},
		{
			name:      "weak beats strong",
			winnerELO: 1200,
			loserELO:  1800,
			wantW:     1231,
			wantL:     1769,
		},
		{
			name:      "small difference",
			winnerELO: 1550,
			loserELO:  1500,
			wantW:     1564,
			wantL:     1486,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, l := ComputeELO(tt.winnerELO, tt.loserELO)
			if w != tt.wantW || l != tt.wantL {
				t.Errorf("ComputeELO(%d, %d) = (%d, %d), want (%d, %d)",
					tt.winnerELO, tt.loserELO, w, l, tt.wantW, tt.wantL)
			}
		})
	}
}

func TestComputeELOZeroSum(t *testing.T) {
	w, l := ComputeELO(1500, 1500)
	if (w + l) != 3000 {
		t.Errorf("ELO not zero-sum: winner=%d, loser=%d, total=%d", w, l, w+l)
	}
}
