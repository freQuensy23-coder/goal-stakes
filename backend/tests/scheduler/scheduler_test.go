package scheduler_test

import (
	"context"
	"testing"
	"time"

	"goalstakes/internal/config"
	"goalstakes/internal/domain"
	"goalstakes/internal/scheduler"
	"goalstakes/internal/service"
	"goalstakes/internal/store"
)

func TestRunOnceReportsMissedCompletedDoGoalPeriod(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user := mustUser(t, st, "0x0000000000000000000000000000000000000abc")
	charger := &fakeCharger{txHash: "0xcharge"}
	svc := mustService(t, st, charger)
	goal := mustGoal(t, svc, user.ID, domain.GoalDo)

	s := scheduler.New(st, svc)
	if err := s.RunOnce(ctx, time.Date(2026, 5, 26, 0, 1, 0, 0, time.UTC)); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	violations, err := st.ListViolations(ctx, goal.ID)
	if err != nil {
		t.Fatalf("ListViolations: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("violations len=%d want 1", len(violations))
	}
	if violations[0].Period != domain.Period("2026-05-25") || violations[0].Status != domain.ViolationCharged {
		t.Fatalf("unexpected violation: %+v", violations[0])
	}
	if charger.calls != 1 {
		t.Fatalf("charger calls = %d, want 1", charger.calls)
	}

	if err := s.RunOnce(ctx, time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("RunOnce second: %v", err)
	}
	if charger.calls != 1 {
		t.Fatalf("second RunOnce must not double-charge; calls=%d", charger.calls)
	}
}

func TestRunOnceCatchesUpAllCompletedDoGoalPeriods(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user := mustUser(t, st, "0x0000000000000000000000000000000000000abc")
	charger := &fakeCharger{txHash: "0xcharge"}
	svc := mustService(t, st, charger)
	goal := mustGoal(t, svc, user.ID, domain.GoalDo)

	s := scheduler.New(st, svc)
	if err := s.RunOnce(ctx, time.Date(2026, 5, 29, 0, 1, 0, 0, time.UTC)); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	violations, err := st.ListViolations(ctx, goal.ID)
	if err != nil {
		t.Fatalf("ListViolations: %v", err)
	}
	got := make(map[domain.Period]bool, len(violations))
	for _, violation := range violations {
		got[violation.Period] = true
	}
	want := []domain.Period{"2026-05-25", "2026-05-26", "2026-05-27", "2026-05-28"}
	for _, period := range want {
		if !got[period] {
			t.Fatalf("missing violation period %s; got %+v", period, violations)
		}
	}
	if len(violations) != len(want) {
		t.Fatalf("violations len=%d want %d: %+v", len(violations), len(want), violations)
	}
	if charger.calls != len(want) {
		t.Fatalf("charger calls = %d, want %d", charger.calls, len(want))
	}

	if err := s.RunOnce(ctx, time.Date(2026, 5, 29, 1, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("RunOnce second: %v", err)
	}
	if charger.calls != len(want) {
		t.Fatalf("second RunOnce must not double-charge; calls=%d", charger.calls)
	}
}

func TestRunOnceSkipsCompletedAndAvoidGoals(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user := mustUser(t, st, "0x0000000000000000000000000000000000000abc")
	charger := &fakeCharger{txHash: "0xcharge"}
	svc := mustService(t, st, charger)
	doneGoal := mustGoal(t, svc, user.ID, domain.GoalDo)
	avoidGoal := mustGoal(t, svc, user.ID, domain.GoalAvoid)
	if _, err := st.UpsertCheckIn(ctx, doneGoal.ID, domain.Period("2026-05-25"), "done"); err != nil {
		t.Fatalf("UpsertCheckIn: %v", err)
	}

	s := scheduler.New(st, svc)
	if err := s.RunOnce(ctx, time.Date(2026, 5, 26, 0, 1, 0, 0, time.UTC)); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	doneViolations, err := st.ListViolations(ctx, doneGoal.ID)
	if err != nil {
		t.Fatalf("ListViolations done: %v", err)
	}
	avoidViolations, err := st.ListViolations(ctx, avoidGoal.ID)
	if err != nil {
		t.Fatalf("ListViolations avoid: %v", err)
	}
	if len(doneViolations) != 0 || len(avoidViolations) != 0 || charger.calls != 0 {
		t.Fatalf("scheduler should skip completed/avoid goals; done=%v avoid=%v calls=%d", doneViolations, avoidViolations, charger.calls)
	}
}

func mustService(t *testing.T, st store.Store, charger service.PenaltyCharger) *service.Service {
	t.Helper()
	svc, err := service.New(st, map[string]config.ChainConfig{
		"sepolia": {
			RPCURL:               "https://sepolia.example/rpc",
			StakeEnforcerAddress: "0x1111111111111111111111111111111111111111",
			Tokens: map[string]string{
				"USDC": "0x2222222222222222222222222222222222222222",
			},
		},
	}, service.WithPenaltyCharger(charger))
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	return svc
}

func mustUser(t *testing.T, st store.Store, wallet string) domain.User {
	t.Helper()
	u, err := st.CreateUser(context.Background(), wallet, "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return u
}

func mustGoal(t *testing.T, svc *service.Service, userID domain.UUID, typ domain.GoalType) domain.Goal {
	t.Helper()
	if _, err := svc.RecordApproval(context.Background(), userID, service.RecordApprovalInput{
		Chain:           "sepolia",
		TokenSymbol:     "USDC",
		TxHash:          "0xtest-approval",
		DryRunAllowance: "1000000",
	}); err != nil {
		t.Fatalf("RecordApproval: %v", err)
	}
	goal, err := svc.CreateGoal(context.Background(), userID, service.CreateGoalInput{
		Title:       "Goal",
		Type:        typ,
		Cadence:     domain.CadenceDaily,
		StakeAmount: "1000000",
		TokenSymbol: "USDC",
		Chain:       "sepolia",
		StartsAt:    time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateGoal: %v", err)
	}
	return goal
}

type fakeCharger struct {
	txHash string
	calls  int
}

func (f *fakeCharger) Penalize(context.Context, string, string, string, string) (string, error) {
	f.calls++
	return f.txHash, nil
}
