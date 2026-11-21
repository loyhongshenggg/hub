// Package claude holds Finance's optional Claude features: turning a goal
// paragraph into structured goals, and identifying merchants the local
// resolver couldn't. Each call is metered against a monthly spend cap.
//
// Sent to Anthropic: goal paragraphs (plus category names and, if allowed,
// average monthly spend per category) and unknown merchant descriptors.
// Never sent: statements, transactions, dates, names or card numbers.
package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"

	"hub.localhost/apps/finance/api/internal/catalog"
	"hub.localhost/apps/finance/api/internal/store"
)

// Model used for every call.
const Model = "claude-opus-5-5"

// Prices for Model in US$ millionths: $4 / $20 per million tokens, $10 per
// 1,000 web searches.
const (
	inMicros     = 4
	outMicros    = 20
	searchMicros = 10_000
)

// Settings are the user's Claude switches (stored as JSON setting "claude").
type Settings struct {
	APIKey        string `json:"apiKey,omitempty"`
	Goals         bool   `json:"goals"`         // draft goals from a paragraph
	Merchants     bool   `json:"merchants"`     // look up unknown merchants
	ShareAverages bool   `json:"shareAverages"` // include category averages with goal drafts
	CapCents      int64  `json:"capCents"`      // monthly cap in US cents
}

// DefaultCapCents is US$5.
const DefaultCapCents = 500

// ErrNoKey / ErrCap / ErrOff are returned before any request is made.
var (
	ErrNoKey = errors.New("no Anthropic API key: add one in Settings")
	ErrCap   = errors.New("this month's Claude spend cap is reached: raise it in Settings")
	ErrOff   = errors.New("this Claude feature is switched off in Settings")
)

// Service makes metered Claude calls.
type Service struct {
	Store *store.Store
	now   func() time.Time
}

func New(st *store.Store) *Service { return &Service{Store: st, now: time.Now} }

// Settings returns the stored settings with defaults.
func (s *Service) Settings(ctx context.Context) (Settings, error) {
	cfg := Settings{CapCents: DefaultCapCents, ShareAverages: true}
	if _, err := s.Store.Setting(ctx, "claude", &cfg); err != nil {
		return cfg, err
	}
	if cfg.CapCents <= 0 {
		cfg.CapCents = DefaultCapCents
	}
	return cfg, nil
}

// Key returns the API key in use and where it comes from.
func (s *Service) Key(cfg Settings) (key, source string) {
	if cfg.APIKey != "" {
		return cfg.APIKey, "settings"
	}
	if k := os.Getenv("ANTHROPIC_API_KEY"); k != "" {
		return k, "env"
	}
	return "", ""
}

func (s *Service) month() string { return s.now().Format("2006-01") }

// ready checks key, switch and cap, and returns a client.
func (s *Service) ready(ctx context.Context, feature func(Settings) bool) (anthropic.Client, Settings, error) {
	cfg, err := s.Settings(ctx)
	if err != nil {
		return anthropic.Client{}, cfg, err
	}
	key, _ := s.Key(cfg)
	if key == "" {
		return anthropic.Client{}, cfg, ErrNoKey
	}
	if !feature(cfg) {
		return anthropic.Client{}, cfg, ErrOff
	}
	u, err := s.Store.UsageFor(ctx, s.month())
	if err != nil {
		return anthropic.Client{}, cfg, err
	}
	if u.CostMicros >= cfg.CapCents*10_000 {
		return anthropic.Client{}, cfg, ErrCap
	}
	return anthropic.NewClient(option.WithAPIKey(key), option.WithMaxRetries(2)), cfg, nil
}

func (s *Service) meter(ctx context.Context, u anthropic.BetaUsage) {
	in := u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
	cost := in*inMicros + u.OutputTokens*outMicros + u.ServerToolUse.WebSearchRequests*searchMicros
	_ = s.Store.AddUsage(ctx, store.Usage{Month: s.month(), Calls: 1, InputTokens: in, OutputTokens: u.OutputTokens,
		Searches: u.ServerToolUse.WebSearchRequests, CostMicros: cost})
}

// base fills what every request shares: model, server-side refusal
// fallbacks (routed by refusal category).
func base(maxTokens int64) anthropic.BetaMessageNewParams {
	return anthropic.BetaMessageNewParams{
		Model:     Model,
		MaxTokens: maxTokens,
		Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01},
		Fallbacks: anthropic.BetaFallbacksParamOfDefault(),
	}
}

func refusal(m *anthropic.BetaMessage) error {
	if m.StopReason == anthropic.BetaStopReasonRefusal {
		return errors.New("Claude declined this request")
	}
	return nil
}

// --- goal drafting

// Draft is one goal Claude proposes. Amounts are SGD dollars.
type Draft struct {
	Kind         string           `json:"kind"`
	Title        string           `json:"title"`
	Why          string           `json:"why"`
	Area         string           `json:"area"`
	TargetAmount float64          `json:"target_amount"`
	StartAmount  float64          `json:"start_amount"`
	TargetDate   string           `json:"target_date"`
	Period       string           `json:"period"`
	Metric       string           `json:"metric"`
	Categories   []string         `json:"categories"`
	Merchants    []string         `json:"merchants"`
	Keywords     []string         `json:"keywords"`
	MonthlyPlan  float64          `json:"monthly_plan"`
	Milestones   []DraftMilestone `json:"milestones"`
	Reasoning    string           `json:"reasoning"`
}

// DraftMilestone is a proposed step.
type DraftMilestone struct {
	Label  string  `json:"label"`
	Amount float64 `json:"amount"`
	Date   string  `json:"date"`
}

// Drafts is Claude's reading of a paragraph.
type Drafts struct {
	Goals       []Draft  `json:"goals"`
	Assumptions []string `json:"assumptions"`
	Questions   []string `json:"questions"`
}

// GoalContext is what Claude may know besides the paragraph.
type GoalContext struct {
	Today        string           // YYYY-MM-DD
	Averages     map[string]int64 // category → average monthly spend (cents), if shared
	MonthlySpend int64            // average total monthly spend (cents), if shared
	Existing     []string         // titles of active goals, to avoid duplicates
}

var areas = []string{"safety", "home", "retirement", "investing", "debt", "lifestyle", "education", "other"}

func draftSchema() map[string]any {
	cats := make([]any, 0, len(catalog.Categories))
	for _, c := range catalog.Categories {
		cats = append(cats, c.ID)
	}
	areaEnum := make([]any, len(areas))
	for i, a := range areas {
		areaEnum[i] = a
	}
	str := map[string]any{"type": "string"}
	num := map[string]any{"type": "number"}
	strs := map[string]any{"type": "array", "items": str}
	obj := func(props map[string]any) map[string]any {
		req := make([]string, 0, len(props))
		for k := range props {
			req = append(req, k)
		}
		return map[string]any{"type": "object", "properties": props, "required": req, "additionalProperties": false}
	}
	goal := obj(map[string]any{
		"kind":          map[string]any{"type": "string", "enum": []any{"target", "payoff", "limit", "habit"}},
		"title":         str,
		"why":           str,
		"area":          map[string]any{"type": "string", "enum": areaEnum},
		"target_amount": num,
		"start_amount":  num,
		"target_date":   str,
		"period":        map[string]any{"type": "string", "enum": []any{"week", "month", "year"}},
		"metric":        map[string]any{"type": "string", "enum": []any{"amount", "count"}},
		"categories":    map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": cats}},
		"merchants":     strs,
		"keywords":      strs,
		"monthly_plan":  num,
		"milestones":    map[string]any{"type": "array", "items": obj(map[string]any{"label": str, "amount": num, "date": str})},
		"reasoning":     str,
	})
	return obj(map[string]any{
		"goals":       map[string]any{"type": "array", "items": goal},
		"assumptions": strs,
		"questions":   strs,
	})
}

const goalSystem = `You turn a person's own description of a money goal into structured goals for a personal finance app used in Singapore (amounts in SGD).

Goal kinds:
- target: reach an amount by a date (emergency fund, down payment, investment pot). target_amount = amount to reach; start_amount = what they have now if stated, else 0; monthly_plan = planned monthly saving if stated or implied (else 0).
- payoff: bring a debt/loan balance to zero. start_amount = current balance; target_amount = 0.
- limit: keep spending at or under a cap per period. target_amount = cap per period (or a count when metric = count, e.g. "at most 8 Grab rides a month"). Use categories (from the allowed list), merchant names, or keywords to say what spending counts.
- habit: put in at least an amount every period (e.g. invest S$800 a month). target_amount = amount per period.

Rules:
- One paragraph can hold several goals; return each separately. Keep titles short (≤ 6 words) and concrete.
- "why" quotes or closely paraphrases the person's own reason, in their voice.
- Dates are YYYY-MM-DD; resolve relative dates ("in 2 years", "by end of next year") from today's date. Leave target_date "" if none is stated or implied.
- Unknown numbers are 0; unknown text is ""; unused lists are [].
- Milestones: 2-4 meaningful steps for target/payoff goals (amount and/or date), none for limit/habit unless the person describes them.
- If average spending is provided, use it to sanity-check limits and mention in reasoning whether the target is a stretch.
- reasoning: one or two sentences on how you read the paragraph.
- assumptions: anything you assumed. questions: at most 3 short questions whose answers would change the goals.
- Do not invent goals the person did not ask for.`

// DraftGoals asks Claude to structure a paragraph into goal drafts.
func (s *Service) DraftGoals(ctx context.Context, paragraph string, gc GoalContext) (*Drafts, error) {
	client, _, err := s.ready(ctx, func(c Settings) bool { return c.Goals })
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Today is %s.\n\nAllowed spending categories (id: name):\n", gc.Today)
	for _, c := range catalog.Categories {
		fmt.Fprintf(&b, "- %s: %s\n", c.ID, c.Name)
	}
	if len(gc.Averages) > 0 {
		fmt.Fprintf(&b, "\nTheir average monthly spending (SGD), last few months: total %.0f\n", float64(gc.MonthlySpend)/100)
		for _, c := range catalog.Categories {
			if v, ok := gc.Averages[c.ID]; ok && v > 0 {
				fmt.Fprintf(&b, "- %s: %.0f\n", c.ID, float64(v)/100)
			}
		}
	}
	if len(gc.Existing) > 0 {
		fmt.Fprintf(&b, "\nGoals they already track (don't duplicate): %s\n", strings.Join(gc.Existing, "; "))
	}
	fmt.Fprintf(&b, "\nTheir words:\n<paragraph>\n%s\n</paragraph>", paragraph)

	p := base(16000)
	p.System = []anthropic.BetaTextBlockParam{{Text: goalSystem}}
	p.OutputConfig = anthropic.BetaOutputConfigParam{
		Effort: anthropic.BetaOutputConfigEffortHigh,
		Format: anthropic.BetaJSONOutputFormatParam{Schema: draftSchema()},
	}
	p.Messages = []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(b.String()))}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	msg, err := client.Beta.Messages.New(ctx, p)
	if err != nil {
		return nil, apiError(err)
	}
	s.meter(ctx, msg.Usage)
	if err := refusal(msg); err != nil {
		return nil, err
	}
	var text strings.Builder
	for _, blk := range msg.Content {
		if t, ok := blk.AsAny().(anthropic.BetaTextBlock); ok {
			text.WriteString(t.Text)
		}
	}
	var d Drafts
	if err := json.Unmarshal([]byte(text.String()), &d); err != nil {
		return nil, fmt.Errorf("unexpected answer from Claude: %w", err)
	}
	return &d, nil
}

// --- merchant lookup

// MerchantAnswer is Claude's identification of a descriptor.
type MerchantAnswer struct {
	Name       string  `json:"name"`
	Category   string  `json:"category"`
	Confidence float64 `json:"confidence"`
	Evidence   string  `json:"evidence"` // URL or short reason
}

const merchantSystem = `You identify merchants from Singapore credit-card statement descriptors. Descriptors are truncated to about 22 characters, may carry a payment processor prefix (e.g. "SMP*", "TS/", "WL *", "SQ *"), a branch or mall abbreviation (AMK = Ang Mo Kio, AMKH = AMK Hub, NOV = Novena, TPY = Toa Payoh), and OCR errors (C↔G, O↔0).

Search the web when useful (at most a few searches), then call record_merchant exactly once with:
- name: the merchant's usual consumer-facing name (brand), not the legal entity, without branch.
- category: one of the allowed ids.
- confidence: 0-1, how sure you are this is the merchant.
- evidence: a URL or a short reason.
If you can't identify it, call record_merchant with your best category guess, name "" and a low confidence.`

func merchantTool() anthropic.BetaToolUnionParam {
	cats := make([]any, 0, len(catalog.Categories))
	for _, c := range catalog.Categories {
		cats = append(cats, c.ID)
	}
	t := anthropic.BetaToolParam{
		Name:        "record_merchant",
		Description: anthropic.String("Record the merchant behind the descriptor."),
		Strict:      anthropic.Bool(true),
		InputSchema: anthropic.BetaToolInputSchemaParam{
			Properties: map[string]any{
				"name":       map[string]any{"type": "string"},
				"category":   map[string]any{"type": "string", "enum": cats},
				"confidence": map[string]any{"type": "number"},
				"evidence":   map[string]any{"type": "string"},
			},
			Required:    []string{"name", "category", "confidence", "evidence"},
			ExtraFields: map[string]any{"additionalProperties": false},
		},
	}
	return anthropic.BetaToolUnionParam{OfTool: &t}
}

// LookupMerchant identifies one descriptor with web search.
func (s *Service) LookupMerchant(ctx context.Context, descriptor string) (*MerchantAnswer, error) {
	client, _, err := s.ready(ctx, func(c Settings) bool { return c.Merchants })
	if err != nil {
		return nil, err
	}
	p := base(16000)
	p.System = []anthropic.BetaTextBlockParam{{Text: merchantSystem}}
	p.OutputConfig = anthropic.BetaOutputConfigParam{Effort: anthropic.BetaOutputConfigEffortMedium}
	p.Tools = []anthropic.BetaToolUnionParam{
		{OfWebSearchTool20260209: &anthropic.BetaWebSearchTool20260209Param{
			MaxUses:      anthropic.Int(3),
			UserLocation: anthropic.BetaUserLocationParam{Country: anthropic.String("SG"), City: anthropic.String("Singapore"), Timezone: anthropic.String("Asia/Singapore")},
		}},
		merchantTool(),
	}
	p.Messages = []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(
		"Descriptor: " + descriptor))}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	for range 4 {
		msg, err := client.Beta.Messages.New(ctx, p)
		if err != nil {
			return nil, apiError(err)
		}
		s.meter(ctx, msg.Usage)
		if err := refusal(msg); err != nil {
			return nil, err
		}
		for _, blk := range msg.Content {
			if tu, ok := blk.AsAny().(anthropic.BetaToolUseBlock); ok && tu.Name == "record_merchant" {
				var a MerchantAnswer
				if err := json.Unmarshal([]byte(tu.JSON.Input.Raw()), &a); err != nil {
					return nil, fmt.Errorf("unexpected answer from Claude: %w", err)
				}
				if !catalog.Valid(a.Category) {
					a.Category = "other"
				}
				return &a, nil
			}
		}
		if msg.StopReason != anthropic.BetaStopReasonPauseTurn {
			break
		}
		// Long server-side search turn: continue it.
		p.Messages = append(p.Messages, msg.ToParam())
	}
	return nil, errors.New("Claude did not identify the merchant")
}

func apiError(err error) error {
	var apierr *anthropic.Error
	if errors.As(err, &apierr) {
		switch apierr.StatusCode {
		case 401:
			return errors.New("the Anthropic API key was rejected")
		case 429:
			return errors.New("Claude is rate-limited right now; try again in a minute")
		}
		return fmt.Errorf("Claude API error %d", apierr.StatusCode)
	}
	return err
}
