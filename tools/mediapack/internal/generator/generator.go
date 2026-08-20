// Package generator builds deterministic, synthetic text-only media packs for
// development, fixtures, and seed content. It intentionally avoids GPU or
// external API dependencies so packs can be regenerated in CI.
package generator

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/knowoff/knowoff/server/pkg/media"
)

// SyntheticPack creates a certified-feasible text-only pack.
// nowns: number of Nowns. cardsPerNown: total cards generated per Nown,
// split into high/distant/chaos bands deterministically.
func SyntheticPack(tag string, nowns, cardsPerNown int, seed int64) (*media.Pack, error) {
	if nowns <= 0 || cardsPerNown < 24 {
		return nil, fmt.Errorf("nowns must be > 0 and cardsPerNown >= 24")
	}
	rng := rand.New(rand.NewSource(seed))

	nowMedia := make([]*media.MediaItem, nowns)
	nownAngles := make([]float64, nowns)
	for i := 0; i < nowns; i++ {
		angle := 2 * math.Pi * float64(i) / float64(nowns)
		nownAngles[i] = angle
		nowMedia[i] = &media.MediaItem{
			ID:         fmt.Sprintf("nown-%04d", i),
			Type:       media.MediaTypeText,
			Content:    fmt.Sprintf("Synthetic topic %d", i),
			Embedding:  angleVector(float32(angle)),
			Tags:       []string{"synthetic", "text"},
			ToneBucket: randomToneBucket(rng),
			Rating:     media.RatingEveryone,
			License:    "CC0-1.0",
		}
	}

	highCount := cardsPerNown / 3
	distantCount := cardsPerNown / 3
	chaosCount := cardsPerNown - highCount - distantCount

	var cards []*media.CardItem
	idCounter := 0
	for i := 0; i < nowns; i++ {
		for j := 0; j < highCount; j++ {
			cards = append(cards, &media.CardItem{
				ID:         fmt.Sprintf("card-%05d", idCounter),
				Type:       media.MediaTypeText,
				Content:    fmt.Sprintf("High match for topic %d", i),
				Embedding:  angleVector(float32(nownAngles[i] + (rng.Float64()-0.5)*0.1)),
				Tags:       []string{"synthetic", "text"},
				ToneBucket: randomToneBucket(rng),
			})
			idCounter++
		}
		for j := 0; j < distantCount; j++ {
			// Offset angle whose cosine lands in the distant band.
			offset := 1.15 + (rng.Float64()-0.5)*0.2
			cards = append(cards, &media.CardItem{
				ID:         fmt.Sprintf("card-%05d", idCounter),
				Type:       media.MediaTypeText,
				Content:    fmt.Sprintf("Distant match for topic %d", i),
				Embedding:  angleVector(float32(nownAngles[i] + offset)),
				Tags:       []string{"synthetic", "text"},
				ToneBucket: randomToneBucket(rng),
			})
			idCounter++
		}
		for j := 0; j < chaosCount; j++ {
			cards = append(cards, &media.CardItem{
				ID:         fmt.Sprintf("card-%05d", idCounter),
				Type:       media.MediaTypeText,
				Content:    fmt.Sprintf("Chaos card %d", idCounter),
				Embedding:  randomUnitVector2D(rng),
				Tags:       []string{"synthetic", "text"},
				ToneBucket: "chaos",
			})
			idCounter++
		}
	}

	if tag == "" {
		tag = fmt.Sprintf("synthetic-%s", time.Now().UTC().Format("20060102"))
	}
	pack := &media.Pack{
		Manifest: media.Manifest{
			PackTag:          tag,
			FormatVersion:    1,
			Language:         "en",
			EmbeddingModel:   "synthetic-deterministic",
			EmbeddingVersion: "1.0",
			AgeRating:        "everyone",
			CreatedAt:        time.Now().UTC().Format(time.RFC3339),
			Checksums:        map[string]string{},
		},
		Media: nowMedia,
		Cards: cards,
	}
	pack.Candidates = media.BuildCandidates(nowMedia, cards, media.DealingTuning{
		BandHigh:          0.55,
		BandLow:           0.30,
		MinHighPerNown:    2,
		MinDistantPerNown: 2,
	})
	return pack, nil
}

// BandStarvedPack returns a deliberately uncertifiable pack for negative tests.
func BandStarvedPack(seed int64) (*media.Pack, error) {
	axis := angleVector(0)
	nowMedia := []*media.MediaItem{{
		ID:         "starved-nown-1",
		Type:       media.MediaTypeText,
		Content:    "Starved topic",
		Embedding:  clone(axis),
		Tags:       []string{"test"},
		ToneBucket: "chaos",
		Rating:     media.RatingEveryone,
		License:    "CC0-1.0",
	}}
	rng := rand.New(rand.NewSource(seed))
	var cards []*media.CardItem
	for i := 0; i < 4; i++ {
		cards = append(cards, &media.CardItem{
			ID:         fmt.Sprintf("high-%d", i),
			Type:       media.MediaTypeText,
			Content:    "High",
			Embedding:  clone(axis),
			Tags:       []string{"test"},
			ToneBucket: "millennial-cope",
		})
	}
	for i := 0; i < 10; i++ {
		cards = append(cards, &media.CardItem{
			ID:         fmt.Sprintf("chaos-%d", i),
			Type:       media.MediaTypeText,
			Content:    "Chaos",
			Embedding:  randomUnitVector2D(rng),
			Tags:       []string{"test"},
			ToneBucket: "chaos",
		})
	}
	pack := &media.Pack{
		Manifest: media.Manifest{
			PackTag:          "test-band-starved",
			FormatVersion:    1,
			Language:         "en",
			EmbeddingModel:   "synthetic",
			EmbeddingVersion: "1.0",
			AgeRating:        "everyone",
			CreatedAt:        time.Now().UTC().Format(time.RFC3339),
			Checksums:        map[string]string{},
		},
		Media: nowMedia,
		Cards: cards,
	}
	pack.Candidates = media.BuildCandidates(nowMedia, cards, media.DealingTuning{
		BandHigh: 0.55, BandLow: 0.30, MinHighPerNown: 2, MinDistantPerNown: 2,
	})
	return pack, nil
}

func randomToneBucket(rng *rand.Rand) string {
	buckets := []string{"millennial-cope", "gen-z-absurdism", "social-awkwardness", "chaos"}
	return buckets[rng.Intn(len(buckets))]
}

func clone(v []float32) []float32 {
	out := make([]float32, len(v))
	copy(out, v)
	return out
}

func normalize(v []float32) []float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		return v
	}
	n := float32(1.0 / sqrtFloat64(sum))
	for i := range v {
		v[i] *= n
	}
	return v
}

func sqrtFloat64(x float64) float64 {
	if x == 0 {
		return 0
	}
	z := x
	for i := 0; i < 20; i++ {
		z = (z + x/z) / 2
	}
	return z
}

func angleVector(angle float32) []float32 {
	return []float32{float32(math.Cos(float64(angle))), float32(math.Sin(float64(angle)))}
}

func randomUnitVector2D(rng *rand.Rand) []float32 {
	angle := rng.Float64() * 2 * math.Pi
	return angleVector(float32(angle))
}
