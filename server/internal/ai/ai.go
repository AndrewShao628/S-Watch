// Package ai wraps LangChainGo + OpenAI for review sentiment ranking and
// personalised re-ranking of recommendation candidates.
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"

	"swatch/internal/models"
)

var ErrDisabled = errors.New("AI features are disabled: OPENAI_API_KEY is not set")

type Service struct {
	llm llms.Model
}

// New returns a Service. With an empty API key the service is created in a
// disabled state so the API can fall back to heuristic behaviour.
func New(apiKey, model string) (*Service, error) {
	if apiKey == "" {
		return &Service{}, nil
	}
	llm, err := openai.New(openai.WithToken(apiKey), openai.WithModel(model))
	if err != nil {
		return nil, fmt.Errorf("init openai: %w", err)
	}
	return &Service{llm: llm}, nil
}

func (s *Service) Enabled() bool { return s != nil && s.llm != nil }

const reviewPrompt = `You are a film critic assistant for the S-Watch streaming platform.
Classify the sentiment of the admin review below into exactly one of these rankings:
%s

Review:
"""
%s
"""

Respond with JSON only, in the form {"ranking_name": "<one of the rankings above>"}.`

// ClassifyReview maps a free-text admin review to one of the provided rankings.
func (s *Service) ClassifyReview(ctx context.Context, review string, rankings []models.Ranking) (models.Ranking, error) {
	if !s.Enabled() {
		return models.Ranking{}, ErrDisabled
	}
	names := make([]string, 0, len(rankings))
	for _, r := range rankings {
		names = append(names, r.RankingName)
	}

	prompt := fmt.Sprintf(reviewPrompt, strings.Join(names, ", "), review)
	out, err := llms.GenerateFromSinglePrompt(ctx, s.llm, prompt,
		llms.WithTemperature(0),
		llms.WithJSONMode(),
		llms.WithMaxTokens(50),
	)
	if err != nil {
		return models.Ranking{}, fmt.Errorf("classify review: %w", err)
	}

	var parsed struct {
		RankingName string `json:"ranking_name"`
	}
	if err := decodeJSON(out, &parsed); err != nil {
		return models.Ranking{}, err
	}
	for _, r := range rankings {
		if strings.EqualFold(strings.TrimSpace(parsed.RankingName), r.RankingName) {
			return r, nil
		}
	}
	return models.Ranking{}, fmt.Errorf("model returned unknown ranking %q", parsed.RankingName)
}

// Profile summarises a viewer's taste for the re-ranking prompt.
type Profile struct {
	FirstName       string
	FavouriteGenres []string
	RecentlyWatched []string
}

type Pick struct {
	ImdbID string `json:"imdb_id"`
	Reason string `json:"reason"`
}

const rerankPrompt = `You are the recommendation engine for S-Watch, a movie streaming platform.

Viewer: %s
Favourite genres: %s
Recently watched: %s

Candidate movies (already pre-scored by our ranking algorithm, best first):
%s

Pick the %d movies this viewer is most likely to enjoy next. Prefer strong critic rankings and
genre fit, avoid titles they recently watched unless nothing else fits, and keep some variety.
For each pick write one short, friendly sentence (max 25 words) explaining why it suits them.

Respond with JSON only: {"picks": [{"imdb_id": "...", "reason": "..."}]}`

// Rerank asks the LLM to choose and explain the best candidates for the viewer.
// Only imdb_ids present in candidates are returned.
func (s *Service) Rerank(ctx context.Context, p Profile, candidates []models.Recommendation, limit int) ([]Pick, error) {
	if !s.Enabled() {
		return nil, ErrDisabled
	}

	var b strings.Builder
	valid := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		m := c.Movie
		valid[m.ImdbID] = true
		genres := make([]string, 0, len(m.Genre))
		for _, g := range m.Genre {
			genres = append(genres, g.GenreName)
		}
		fmt.Fprintf(&b, "- imdb_id=%s | %s (%d) | genres: %s | critic ranking: %s | score: %.2f | %s\n",
			m.ImdbID, m.Title, m.Year, strings.Join(genres, ", "), m.Ranking.RankingName, c.Score, truncate(m.Overview, 160))
	}

	prompt := fmt.Sprintf(rerankPrompt,
		orNone(p.FirstName), orNone(strings.Join(p.FavouriteGenres, ", ")),
		orNone(strings.Join(p.RecentlyWatched, ", ")), b.String(), limit)

	out, err := llms.GenerateFromSinglePrompt(ctx, s.llm, prompt,
		llms.WithTemperature(0.3),
		llms.WithJSONMode(),
		llms.WithMaxTokens(1200),
	)
	if err != nil {
		return nil, fmt.Errorf("rerank: %w", err)
	}

	var parsed struct {
		Picks []Pick `json:"picks"`
	}
	if err := decodeJSON(out, &parsed); err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	picks := make([]Pick, 0, limit)
	for _, pick := range parsed.Picks {
		if !valid[pick.ImdbID] || seen[pick.ImdbID] {
			continue
		}
		seen[pick.ImdbID] = true
		picks = append(picks, pick)
		if len(picks) == limit {
			break
		}
	}
	if len(picks) == 0 {
		return nil, errors.New("model returned no valid picks")
	}
	return picks, nil
}

// decodeJSON tolerates models that wrap JSON in prose or code fences.
func decodeJSON(raw string, v any) error {
	start, end := strings.Index(raw, "{"), strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return fmt.Errorf("model response is not JSON: %q", truncate(raw, 200))
	}
	if err := json.Unmarshal([]byte(raw[start:end+1]), v); err != nil {
		return fmt.Errorf("decode model response: %w", err)
	}
	return nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
