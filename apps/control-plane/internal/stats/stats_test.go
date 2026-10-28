package stats

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestStudentTwoTailedPKnownValues(t *testing.T) {
	// Reference values from standard t tables.
	cases := []struct{ t, df, p float64 }{
		{2.228, 10, 0.05},
		{2.042, 30, 0.05},
		{1.96, 1e6, 0.05},
		{0, 5, 1},
	}
	for _, c := range cases {
		if got := StudentTwoTailedP(c.t, c.df); math.Abs(got-c.p) > 0.002 {
			t.Errorf("p(t=%.3f, df=%.0f) = %.4f, want %.3f", c.t, c.df, got, c.p)
		}
	}
}

func TestWelchDetectsRealShiftNotNoise(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 11))
	sample := func(mean float64, n int) []float64 {
		out := make([]float64, n)
		for i := range out {
			out[i] = mean + r.NormFloat64()*10
		}
		return out
	}
	if p := WelchTTest(sample(100, 60), sample(100, 60)); p < 0.01 {
		t.Errorf("identical distributions flagged as different (p=%.4f)", p)
	}
	if p := WelchTTest(sample(100, 60), sample(110, 60)); p > 0.001 {
		t.Errorf("a 10%% shift with 60 samples should be significant (p=%.4f)", p)
	}
	if p := WelchTTest([]float64{1}, []float64{2}); p != 1 {
		t.Error("too little data must not be called significant")
	}
}
