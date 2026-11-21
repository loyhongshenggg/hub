package engine

import (
	"math"
	"math/rand/v2"
)

// Model constants. Skill θ and difficulty d share the 1–5 scale.
const (
	defaultTheta = 1.5 // a topic never assessed or practised
	metTheta     = 2.0 // a prerequisite counts as met from here
	zpdStretch   = 0.5 // aim this far above current skill
	snoozeFor    = 2   // a snoozed chapter returns after this many completions
	maxObs       = 12
	maxFresh     = 2 // optional fresh steps shown per chapter
	freshDays    = 60
)

// Self-assessment levels.
var levels = map[string]float64{"new": 1, "some": 2.5, "solid": 4}

// Score weights for picking the next chapter.
const (
	wInterest = 0.4
	wFit      = 0.4
	wProgress = 0.2
	momentum  = 0.15 // staying in the current track, at neutral interest
	unmetCost = 0.2  // per unmet prerequisite topic
	unlockGap = 0.1  // teaches a prerequisite another frontier chapter lacks
)

// outcome maps a rating onto [0,1] (1 = it was easy for you).
func outcome(r Rating) (float64, bool) {
	switch r {
	case Easy:
		return 1, true
	case Right:
		return 0.6, true
	case Hard:
		return 0, true
	}
	return 0, false
}

// expected is the predicted outcome for skill θ on difficulty d: a logistic
// curve (Elo) shifted so content zpdStretch above your skill expects
// "just right" (0.6).
func expected(theta, d float64) float64 {
	return 1 / (1 + math.Exp(-(1.5*(theta-d) + 1.15)))
}

// observe applies one outcome s on content of difficulty d to a topic: an
// Elo update with a step size that shrinks as evidence grows.
func observe(t TopicState, d, s float64) TopicState {
	k := 0.6 / (1 + 0.15*float64(t.N))
	t.Theta = clamp(t.Theta+k*(s-expected(t.Theta, d)), 1, 5)
	t.N++
	t.Obs = append(t.Obs, s)
	if len(t.Obs) > maxObs {
		t.Obs = t.Obs[len(t.Obs)-maxObs:]
	}
	return t
}

// learningProgress is how much a topic's recent outcomes improved on the
// ones before (ZPDES). Topics with little evidence get an optimistic prior
// so they get explored.
func learningProgress(t TopicState) float64 {
	n := len(t.Obs)
	if n < 4 {
		return 0.3
	}
	k := min(3, n/2)
	recent, older := mean(t.Obs[n-k:]), mean(t.Obs[n-2*k:n-k])
	return clamp(recent-older, 0, 1)
}

// fit peaks when difficulty is zpdStretch above skill.
func fit(theta, d float64) float64 {
	x := d - (theta + zpdStretch)
	return math.Exp(-x * x / 2)
}

// sampleBeta draws from Beta(a, b) via two Gamma draws.
func sampleBeta(r *rand.Rand, a, b float64) float64 {
	x, y := sampleGamma(r, a), sampleGamma(r, b)
	return x / (x + y)
}

// sampleGamma is Marsaglia and Tsang's method (shape < 1 boosted).
func sampleGamma(r *rand.Rand, shape float64) float64 {
	if shape < 1 {
		return sampleGamma(r, shape+1) * math.Pow(r.Float64(), 1/shape)
	}
	d := shape - 1.0/3
	c := 1 / math.Sqrt(9*d)
	for {
		x := r.NormFloat64()
		v := 1 + c*x
		if v <= 0 {
			continue
		}
		v = v * v * v
		u := r.Float64()
		if math.Log(u) < 0.5*x*x+d-d*v+d*math.Log(v) {
			return d * v
		}
	}
}

func clamp(x, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, x)) }

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}
