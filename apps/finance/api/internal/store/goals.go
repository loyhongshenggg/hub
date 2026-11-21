package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// GoalFilter selects the transactions a spending-limit goal watches.
type GoalFilter struct {
	Categories []string `json:"categories"`
	Merchants  []string `json:"merchants"` // merchant names (case-insensitive)
	Keywords   []string `json:"keywords"`  // matched against merchant + description
}

// Milestone is a step on the way to a goal.
type Milestone struct {
	Label  string `json:"label"`
	Amount *int64 `json:"amount,omitempty"` // reached when progress ≥ amount
	Date   string `json:"date,omitempty"`   // planned date
	DoneAt string `json:"doneAt,omitempty"` // ticked by hand (no amount)
}

// Goal is anything you're working towards (docs/finance/design.md §5).
type Goal struct {
	ID          int64       `json:"id"`
	Kind        string      `json:"kind"` // target | payoff | limit | habit
	Title       string      `json:"title"`
	Why         string      `json:"why"`
	Area        string      `json:"area"`   // safety | home | retirement | lifestyle | debt | growth | other
	Status      string      `json:"status"` // active | paused | achieved | archived
	Target      int64       `json:"target"` // cents (or a count for limit goals with metric=count)
	StartAmount int64       `json:"startAmount"`
	StartDate   string      `json:"startDate"`
	TargetDate  string      `json:"targetDate"`
	Period      string      `json:"period"` // week | month | year (limit, habit)
	Metric      string      `json:"metric"` // amount | count (limit)
	Filter      GoalFilter  `json:"filter"`
	MonthlyPlan int64       `json:"monthlyPlan"`
	Milestones  []Milestone `json:"milestones"`
	Source      string      `json:"source"` // manual | claude
	Prompt      string      `json:"prompt,omitempty"`
	Sort        int         `json:"sort"`
	CreatedAt   time.Time   `json:"createdAt"`
	UpdatedAt   time.Time   `json:"updatedAt"`
	AchievedAt  *time.Time  `json:"achievedAt"`
}

// GoalEntry is a check-in: a balance (target/payoff) or a contribution (habit).
type GoalEntry struct {
	ID     int64  `json:"id"`
	GoalID int64  `json:"goalId"`
	Date   string `json:"date"`
	Amount int64  `json:"amount"`
	Note   string `json:"note"`
}

const goalCols = `id, kind, title, why, area, status, target, start_amount, start_date, target_date, period, metric, filter,
	monthly_plan, milestones, source, prompt, sort, created_at, updated_at, achieved_at`

func scanGoal(sc interface{ Scan(...any) error }) (Goal, error) {
	var g Goal
	var filter, ms string
	var created, updated int64
	var achieved sql.NullInt64
	if err := sc.Scan(&g.ID, &g.Kind, &g.Title, &g.Why, &g.Area, &g.Status, &g.Target, &g.StartAmount, &g.StartDate, &g.TargetDate,
		&g.Period, &g.Metric, &filter, &g.MonthlyPlan, &ms, &g.Source, &g.Prompt, &g.Sort, &created, &updated, &achieved); err != nil {
		return g, err
	}
	_ = json.Unmarshal([]byte(filter), &g.Filter)
	_ = json.Unmarshal([]byte(ms), &g.Milestones)
	if g.Milestones == nil {
		g.Milestones = []Milestone{}
	}
	g.CreatedAt, g.UpdatedAt = time.Unix(created, 0), time.Unix(updated, 0)
	if achieved.Valid {
		t := time.Unix(achieved.Int64, 0)
		g.AchievedAt = &t
	}
	return g, nil
}

// Goals lists goals: active first (by sort), then the rest newest first.
func (s *Store) Goals(ctx context.Context) ([]Goal, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+goalCols+` FROM goals
		ORDER BY CASE status WHEN 'active' THEN 0 WHEN 'paused' THEN 1 WHEN 'achieved' THEN 2 ELSE 3 END, sort, COALESCE(achieved_at, created_at) DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Goal{}
	for rows.Next() {
		g, err := scanGoal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// GetGoal returns one goal.
func (s *Store) GetGoal(ctx context.Context, id int64) (Goal, error) {
	g, err := scanGoal(s.db.QueryRowContext(ctx, "SELECT "+goalCols+" FROM goals WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return g, ErrNotFound
	}
	return g, err
}

// SaveGoal inserts (ID 0) or updates a goal.
func (s *Store) SaveGoal(ctx context.Context, g Goal) (int64, error) {
	filter, _ := json.Marshal(g.Filter)
	if g.Milestones == nil {
		g.Milestones = []Milestone{}
	}
	ms, _ := json.Marshal(g.Milestones)
	now := unix(s.now())
	var achieved any
	if g.AchievedAt != nil {
		achieved = unix(*g.AchievedAt)
	}
	if g.ID == 0 {
		var maxSort sql.NullInt64
		_ = s.db.QueryRowContext(ctx, "SELECT MAX(sort) FROM goals").Scan(&maxSort)
		res, err := s.db.ExecContext(ctx, `INSERT INTO goals (kind, title, why, area, status, target, start_amount, start_date, target_date, period,
			metric, filter, monthly_plan, milestones, source, prompt, sort, created_at, updated_at, achieved_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			g.Kind, g.Title, g.Why, g.Area, g.Status, g.Target, g.StartAmount, g.StartDate, g.TargetDate, g.Period,
			g.Metric, string(filter), g.MonthlyPlan, string(ms), g.Source, g.Prompt, maxSort.Int64+1, now, now, achieved)
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	}
	res, err := s.db.ExecContext(ctx, `UPDATE goals SET kind = ?, title = ?, why = ?, area = ?, status = ?, target = ?, start_amount = ?,
		start_date = ?, target_date = ?, period = ?, metric = ?, filter = ?, monthly_plan = ?, milestones = ?, sort = ?, updated_at = ?, achieved_at = ?
		WHERE id = ?`,
		g.Kind, g.Title, g.Why, g.Area, g.Status, g.Target, g.StartAmount, g.StartDate, g.TargetDate, g.Period, g.Metric,
		string(filter), g.MonthlyPlan, string(ms), g.Sort, now, achieved, g.ID)
	return g.ID, notFound(res, err)
}

// DeleteGoal deletes a goal and its check-ins.
func (s *Store) DeleteGoal(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM goals WHERE id = ?", id)
	return notFound(res, err)
}

// GoalEntries lists a goal's check-ins, oldest first.
func (s *Store) GoalEntries(ctx context.Context, goalID int64) ([]GoalEntry, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, goal_id, date, amount, note FROM goal_entries WHERE goal_id = ? ORDER BY date, id", goalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GoalEntry{}
	for rows.Next() {
		var e GoalEntry
		if err := rows.Scan(&e.ID, &e.GoalID, &e.Date, &e.Amount, &e.Note); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// AddGoalEntry records a check-in.
func (s *Store) AddGoalEntry(ctx context.Context, e GoalEntry) (int64, error) {
	res, err := s.db.ExecContext(ctx, "INSERT INTO goal_entries (goal_id, date, amount, note, created_at) VALUES (?, ?, ?, ?, ?)",
		e.GoalID, e.Date, e.Amount, e.Note, unix(s.now()))
	if err != nil {
		return 0, err
	}
	_, _ = s.db.ExecContext(ctx, "UPDATE goals SET updated_at = ? WHERE id = ?", unix(s.now()), e.GoalID)
	return res.LastInsertId()
}

// DeleteGoalEntry removes a check-in.
func (s *Store) DeleteGoalEntry(ctx context.Context, goalID, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM goal_entries WHERE id = ? AND goal_id = ?", id, goalID)
	return notFound(res, err)
}
