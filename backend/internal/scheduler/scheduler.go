// Package scheduler detects missed "do" goal periods and reports violations.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"goalstakes/internal/domain"
	"goalstakes/internal/service"
	"goalstakes/internal/store"
)

type Scheduler struct {
	store   store.Store
	service *service.Service
}

func New(st store.Store, svc *service.Service) *Scheduler {
	return &Scheduler{store: st, service: svc}
}

func (s *Scheduler) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := s.RunOnce(ctx, now.UTC()); err != nil {
				log.Printf("scheduler: missed-period run failed: %v", err)
			}
		}
	}
}

// RunOnce checks every completed period for every active "do" goal. Idempotency
// is delegated to Service.ReportViolation + the store's (goal,period) uniqueness
// guarantee, so repeated calls cannot double-charge.
func (s *Scheduler) RunOnce(ctx context.Context, now time.Time) error {
	goals, err := s.store.ListActiveGoals(ctx)
	if err != nil {
		return err
	}
	for _, goal := range goals {
		if goal.Type != domain.GoalDo {
			continue
		}
		for _, period := range completedPeriods(goal, now) {
			if _, err := s.store.GetCheckIn(ctx, goal.ID, period); err == nil {
				continue
			} else if !errors.Is(err, store.ErrNotFound) {
				return err
			}
			if _, err := s.service.ReportViolation(ctx, goal.UserID, goal.ID, service.ReportViolationInput{
				Period: period,
				Reason: "missed deadline",
			}); err != nil {
				return fmt.Errorf("report missed goal %s period %s: %w", goal.ID, period, err)
			}
		}
	}
	return nil
}

func periodIntersectsGoal(goal domain.Goal, period domain.Period) bool {
	start, end, err := goal.PeriodBounds(period)
	if err != nil {
		return false
	}
	if !end.After(goal.StartsAt) {
		return false
	}
	if goal.EndsAt != nil && !start.Before(*goal.EndsAt) {
		return false
	}
	return true
}

func completedPeriods(goal domain.Goal, now time.Time) []domain.Period {
	period := goal.CurrentPeriod(goal.StartsAt)
	if period == "" {
		return nil
	}

	now = now.UTC()
	var periods []domain.Period
	for {
		start, end, err := goal.PeriodBounds(period)
		if err != nil || end.After(now) {
			break
		}
		if goal.EndsAt != nil && !start.Before(*goal.EndsAt) {
			break
		}
		if periodIntersectsGoal(goal, period) {
			periods = append(periods, period)
		}
		next := goal.CurrentPeriod(end)
		if next == "" || next == period {
			break
		}
		period = next
	}
	return periods
}
