package service_test

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"goalstakes/internal/auth"
	"goalstakes/internal/config"
	"goalstakes/internal/domain"
	"goalstakes/internal/service"
	"goalstakes/internal/store"
)

func TestCreateGoalRejectsDisallowedToken(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user := mustUser(t, st, "0xabc")
	svc := newTestService(t, st)

	_, err := svc.CreateGoal(ctx, user.ID, service.CreateGoalInput{
		Title:       "Avoid soda",
		Type:        domain.GoalAvoid,
		Cadence:     domain.CadenceDaily,
		StakeAmount: "1000000",
		TokenSymbol: "DAI",
		Chain:       "sepolia",
	})
	if err == nil {
		t.Fatal("CreateGoal must reject tokens outside the configured allow-list")
	}

	if _, err := svc.RecordApproval(ctx, user.ID, service.RecordApprovalInput{Chain: "sepolia", TokenSymbol: "USDC", TxHash: "0xtest", DryRunAllowance: "1000000"}); err != nil {
		t.Fatalf("RecordApproval: %v", err)
	}
	goal, err := svc.CreateGoal(ctx, user.ID, service.CreateGoalInput{
		Title:       "Do push-ups",
		Type:        domain.GoalDo,
		Cadence:     domain.CadenceDaily,
		StakeAmount: "1000000",
		TokenSymbol: "usdc",
		Chain:       "sepolia",
	})
	if err != nil {
		t.Fatalf("CreateGoal valid: %v", err)
	}
	if goal.TokenSymbol != "USDC" || goal.Chain != "sepolia" {
		t.Fatalf("CreateGoal normalized stake fields incorrectly: %+v", goal)
	}
}

func TestCreateGoalRequiresAllowanceCoveringStake(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user := mustUser(t, st, "0xabc")
	svc := newTestService(t, st)
	_, err := svc.CreateGoal(ctx, user.ID, service.CreateGoalInput{
		Title:       "Do push-ups",
		Type:        domain.GoalDo,
		Cadence:     domain.CadenceDaily,
		StakeAmount: "1000000",
		TokenSymbol: "USDC",
		Chain:       "sepolia",
	})
	if err == nil || !strings.Contains(err.Error(), "approval allowance") {
		t.Fatalf("CreateGoal without allowance error = %v, want approval allowance error", err)
	}

	if _, err := svc.RecordApproval(ctx, user.ID, service.RecordApprovalInput{Chain: "sepolia", TokenSymbol: "USDC", TxHash: "0xtest-low", DryRunAllowance: "999999"}); err != nil {
		t.Fatalf("RecordApproval low: %v", err)
	}
	_, err = svc.CreateGoal(ctx, user.ID, service.CreateGoalInput{
		Title:       "Do push-ups",
		Type:        domain.GoalDo,
		Cadence:     domain.CadenceDaily,
		StakeAmount: "1000000",
		TokenSymbol: "USDC",
		Chain:       "sepolia",
	})
	if err == nil || !strings.Contains(err.Error(), "approval allowance") {
		t.Fatalf("CreateGoal with low allowance error = %v, want approval allowance error", err)
	}

	if _, err := svc.RecordApproval(ctx, user.ID, service.RecordApprovalInput{Chain: "sepolia", TokenSymbol: "USDC", TxHash: "0xtest-exact", DryRunAllowance: "1000000"}); err != nil {
		t.Fatalf("RecordApproval exact: %v", err)
	}
	goal, err := svc.CreateGoal(ctx, user.ID, service.CreateGoalInput{
		Title:       "Do push-ups",
		Type:        domain.GoalDo,
		Cadence:     domain.CadenceDaily,
		StakeAmount: "1000000",
		TokenSymbol: "USDC",
		Chain:       "sepolia",
	})
	if err != nil {
		t.Fatalf("CreateGoal with exact allowance: %v", err)
	}
	if goal.StakeAmount != "1000000" {
		t.Fatalf("goal stake = %q", goal.StakeAmount)
	}
}

func TestCreateGoalRejectsCustomCadence(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user := mustUser(t, st, "0xabc")
	svc := newTestService(t, st)
	if _, err := svc.RecordApproval(ctx, user.ID, service.RecordApprovalInput{Chain: "sepolia", TokenSymbol: "USDC", TxHash: "0xtest", DryRunAllowance: "1000000"}); err != nil {
		t.Fatalf("RecordApproval: %v", err)
	}

	_, err := svc.CreateGoal(ctx, user.ID, service.CreateGoalInput{
		Title:       "Custom schedule",
		Type:        domain.GoalDo,
		Cadence:     domain.CadenceCustom,
		StakeAmount: "1000000",
		TokenSymbol: "USDC",
		Chain:       "sepolia",
	})
	if err == nil || !strings.Contains(err.Error(), "custom cadence is not supported") {
		t.Fatalf("CreateGoal custom cadence error = %v, want unsupported custom cadence", err)
	}
}

func TestCreateGoalRejectsInvalidTimezone(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user := mustUser(t, st, "0xabc")
	svc := newTestService(t, st)
	if _, err := svc.RecordApproval(ctx, user.ID, service.RecordApprovalInput{Chain: "sepolia", TokenSymbol: "USDC", TxHash: "0xtest", DryRunAllowance: "1000000"}); err != nil {
		t.Fatalf("RecordApproval: %v", err)
	}

	_, err := svc.CreateGoal(ctx, user.ID, service.CreateGoalInput{
		Title:       "Do push-ups",
		Type:        domain.GoalDo,
		Cadence:     domain.CadenceDaily,
		StakeAmount: "1000000",
		TokenSymbol: "USDC",
		Chain:       "sepolia",
		Timezone:    "not/a-zone",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid timezone") {
		t.Fatalf("CreateGoal invalid timezone error = %v, want invalid timezone", err)
	}
}

func TestUpdateGoalRequiresAllowanceCoveringIncreasedStake(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user := mustUser(t, st, "0xabc")
	svc := newTestService(t, st)
	goal := mustGoal(t, svc, user.ID)

	_, err := svc.UpdateGoal(ctx, user.ID, goal.ID, service.UpdateGoalInput{
		Title:       "Push-ups",
		StakeAmount: "2000000",
	})
	if err == nil || !strings.Contains(err.Error(), "approval allowance") {
		t.Fatalf("UpdateGoal increased stake error = %v, want approval allowance error", err)
	}

	if _, err := svc.RecordApproval(ctx, user.ID, service.RecordApprovalInput{Chain: "sepolia", TokenSymbol: "USDC", TxHash: "0xtest-increase", DryRunAllowance: "2000000"}); err != nil {
		t.Fatalf("RecordApproval: %v", err)
	}
	updated, err := svc.UpdateGoal(ctx, user.ID, goal.ID, service.UpdateGoalInput{
		Title:       "Push-ups",
		StakeAmount: "2000000",
	})
	if err != nil {
		t.Fatalf("UpdateGoal with covering allowance: %v", err)
	}
	if updated.StakeAmount != "2000000" {
		t.Fatalf("updated stake = %q", updated.StakeAmount)
	}
}

func TestLogCheckInUsesCurrentPeriodAndEnforcesOwnership(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	owner := mustUser(t, st, "0xowner")
	other := mustUser(t, st, "0xother")
	svc := newTestService(t, st)
	goal := mustGoal(t, svc, owner.ID)

	checkIn, err := svc.LogCheckIn(ctx, owner.ID, goal.ID, service.LogCheckInInput{Note: "done"})
	if err != nil {
		t.Fatalf("LogCheckIn: %v", err)
	}
	if checkIn.Period != domain.Period("2026-05-25") {
		t.Fatalf("LogCheckIn period = %q, want 2026-05-25", checkIn.Period)
	}

	if _, err := svc.LogCheckIn(ctx, other.ID, goal.ID, service.LogCheckInInput{}); err == nil {
		t.Fatal("LogCheckIn must reject access to another user's goal")
	}
}

func TestReportViolationIsIdempotentPerGoalPeriod(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user := mustUser(t, st, "0xabc")
	svc := newTestService(t, st)
	goal := mustGoal(t, svc, user.ID)

	v1, err := svc.ReportViolation(ctx, user.ID, goal.ID, service.ReportViolationInput{Reason: "missed"})
	if err != nil {
		t.Fatalf("ReportViolation #1: %v", err)
	}
	v2, err := svc.ReportViolation(ctx, user.ID, goal.ID, service.ReportViolationInput{Reason: "missed again"})
	if err != nil {
		t.Fatalf("ReportViolation #2: %v", err)
	}
	if v1.ID != v2.ID || v1.Status != domain.ViolationPending {
		t.Fatalf("ReportViolation must be idempotent and pending before chain charge: v1=%+v v2=%+v", v1, v2)
	}
	if v1.Amount != goal.StakeAmount {
		t.Fatalf("violation amount = %q, want goal stake %q", v1.Amount, goal.StakeAmount)
	}
}

func TestReportViolationChargesOnlyNewViolations(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user := mustUser(t, st, "0x0000000000000000000000000000000000000abc")
	charger := &fakeCharger{txHash: "0xcharge"}
	svc := newTestServiceWithCharger(t, st, charger)
	goal := mustGoal(t, svc, user.ID)

	v1, err := svc.ReportViolation(ctx, user.ID, goal.ID, service.ReportViolationInput{Reason: "missed"})
	if err != nil {
		t.Fatalf("ReportViolation #1: %v", err)
	}
	if v1.Status != domain.ViolationCharged || v1.TxHash != "0xcharge" {
		t.Fatalf("charged violation = %+v, want charged with tx", v1)
	}
	if charger.calls != 1 {
		t.Fatalf("charger calls = %d, want 1", charger.calls)
	}
	if charger.lastChain != "sepolia" || charger.lastUser != user.WalletAddress || charger.lastToken != "0x2222222222222222222222222222222222222222" || charger.lastAmount != "1000000" {
		t.Fatalf("charger args chain=%s user=%s token=%s amount=%s", charger.lastChain, charger.lastUser, charger.lastToken, charger.lastAmount)
	}

	v2, err := svc.ReportViolation(ctx, user.ID, goal.ID, service.ReportViolationInput{Reason: "missed again"})
	if err != nil {
		t.Fatalf("ReportViolation #2: %v", err)
	}
	if v2.ID != v1.ID || charger.calls != 1 {
		t.Fatalf("duplicate report must not create or charge again: v1=%+v v2=%+v calls=%d", v1, v2, charger.calls)
	}
}

func TestReportViolationChargesEveryAvoidReport(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user := mustUser(t, st, "0x0000000000000000000000000000000000000abc")
	charger := &fakeCharger{txHash: "0xcharge"}
	svc := newTestServiceWithCharger(t, st, charger)
	goal := mustGoalWithType(t, svc, user.ID, domain.GoalAvoid)

	v1, err := svc.ReportViolation(ctx, user.ID, goal.ID, service.ReportViolationInput{Reason: "drank soda"})
	if err != nil {
		t.Fatalf("ReportViolation #1: %v", err)
	}
	v2, err := svc.ReportViolation(ctx, user.ID, goal.ID, service.ReportViolationInput{Reason: "drank soda again"})
	if err != nil {
		t.Fatalf("ReportViolation #2: %v", err)
	}
	if v1.ID == v2.ID {
		t.Fatalf("avoid reports must create separate violations: v1=%+v v2=%+v", v1, v2)
	}
	if charger.calls != 2 {
		t.Fatalf("charger calls = %d, want 2", charger.calls)
	}
	violations, err := st.ListViolations(ctx, goal.ID)
	if err != nil {
		t.Fatalf("ListViolations: %v", err)
	}
	if len(violations) != 2 {
		t.Fatalf("violations len=%d, want 2: %+v", len(violations), violations)
	}
}

func TestReportViolationMarksFailedWhenChargeFails(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user := mustUser(t, st, "0x0000000000000000000000000000000000000abc")
	charger := &fakeCharger{txHash: "0xreverted", err: errors.New("penalize tx reverted")}
	svc := newTestServiceWithCharger(t, st, charger)
	goal := mustGoal(t, svc, user.ID)

	violation, err := svc.ReportViolation(ctx, user.ID, goal.ID, service.ReportViolationInput{Reason: "missed"})
	if err == nil {
		t.Fatal("ReportViolation should surface charge failure")
	}
	if !errors.Is(err, service.ErrChargeFailed) {
		t.Fatalf("ReportViolation error = %v, want ErrChargeFailed", err)
	}
	if violation.Status != domain.ViolationFailed {
		t.Fatalf("violation status = %q, want failed", violation.Status)
	}
	if violation.TxHash != "0xreverted" {
		t.Fatalf("violation tx hash = %q, want failed tx hash", violation.TxHash)
	}
}

func TestListGoalsOmitsArchivedGoals(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user := mustUser(t, st, "0xabc")
	svc := newTestService(t, st)
	archived := mustGoal(t, svc, user.ID)
	active := mustGoal(t, svc, user.ID)

	if err := svc.ArchiveGoal(ctx, user.ID, archived.ID); err != nil {
		t.Fatalf("ArchiveGoal: %v", err)
	}
	goals, err := svc.ListGoals(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListGoals: %v", err)
	}
	if len(goals) != 1 || goals[0].ID != active.ID {
		t.Fatalf("ListGoals = %+v, want only active goal %s", goals, active.ID)
	}
}

func TestSetStakeUpdatesAmountTokenAndChain(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user := mustUser(t, st, "0xabc")
	svc := newTestService(t, st)
	goal := mustGoal(t, svc, user.ID)

	if _, err := svc.RecordApproval(ctx, user.ID, service.RecordApprovalInput{Chain: "sepolia", TokenSymbol: "USDT", TxHash: "0xtest-low-usdt", DryRunAllowance: "2499999"}); err != nil {
		t.Fatalf("RecordApproval low USDT: %v", err)
	}
	_, err := svc.SetStake(ctx, user.ID, goal.ID, service.SetStakeInput{
		StakeAmount: "2500000",
		TokenSymbol: "USDT",
		Chain:       "sepolia",
	})
	if err == nil || !strings.Contains(err.Error(), "approval allowance") {
		t.Fatalf("SetStake low allowance error = %v, want approval allowance error", err)
	}

	if _, err := svc.RecordApproval(ctx, user.ID, service.RecordApprovalInput{Chain: "sepolia", TokenSymbol: "USDT", TxHash: "0xtest-usdt", DryRunAllowance: "2500000"}); err != nil {
		t.Fatalf("RecordApproval exact USDT: %v", err)
	}
	updated, err := svc.SetStake(ctx, user.ID, goal.ID, service.SetStakeInput{
		StakeAmount: "2500000",
		TokenSymbol: "USDT",
		Chain:       "sepolia",
	})
	if err != nil {
		t.Fatalf("SetStake: %v", err)
	}
	if updated.StakeAmount != "2500000" || updated.TokenSymbol != "USDT" || updated.Chain != "sepolia" {
		t.Fatalf("SetStake did not persist stake fields: %+v", updated)
	}

	if _, err := svc.SetStake(ctx, user.ID, goal.ID, service.SetStakeInput{
		StakeAmount: "2500000",
		TokenSymbol: "USDC",
		Chain:       "unknown-chain",
	}); err == nil {
		t.Fatal("SetStake must reject unknown chains")
	}
}

func TestRecordAndGetApprovalStatus(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user := mustUser(t, st, "0xabc")
	svc := newTestService(t, st)

	empty, err := svc.GetApprovalStatus(ctx, user.ID, "sepolia", "USDC")
	if err != nil {
		t.Fatalf("GetApprovalStatus empty: %v", err)
	}
	if empty.Allowance != "0" || empty.Approved {
		t.Fatalf("empty approval = %+v, want zero/not approved", empty)
	}

	status, err := svc.RecordApproval(ctx, user.ID, service.RecordApprovalInput{
		Chain:           "sepolia",
		TokenSymbol:     "USDC",
		TxHash:          "0xapproval",
		DryRunAllowance: "1000000",
	})
	if err != nil {
		t.Fatalf("RecordApproval: %v", err)
	}
	if status.Allowance != "1000000" || !status.Approved {
		t.Fatalf("recorded approval = %+v, want approved allowance", status)
	}
}

func TestRecordApprovalRequiresTxHash(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user := mustUser(t, st, "0xabc")
	svc := newTestService(t, st)

	_, err := svc.RecordApproval(ctx, user.ID, service.RecordApprovalInput{
		Chain:           "sepolia",
		TokenSymbol:     "USDC",
		DryRunAllowance: "1000000",
	})
	if err == nil || !strings.Contains(err.Error(), "tx_hash is required") {
		t.Fatalf("RecordApproval without tx_hash error = %v, want tx_hash required", err)
	}
}

func TestRecordApprovalRequiresDryRunAllowanceWithoutChecker(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user := mustUser(t, st, "0xabc")
	svc := newTestService(t, st)

	_, err := svc.RecordApproval(ctx, user.ID, service.RecordApprovalInput{
		Chain:       "sepolia",
		TokenSymbol: "USDC",
		TxHash:      "0xapproval",
	})
	if err == nil || !strings.Contains(err.Error(), "dry_run_allowance is required") {
		t.Fatalf("RecordApproval without dry_run_allowance error = %v, want dry_run_allowance required", err)
	}
}

func TestGetApprovalStatusRefreshesFromChecker(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user := mustUser(t, st, "0x0000000000000000000000000000000000000abc")
	checker := &fakeApprovalChecker{allowance: big.NewInt(2500000)}
	svc := newTestServiceWithApprovalChecker(t, st, checker)

	status, err := svc.GetApprovalStatus(ctx, user.ID, "sepolia", "USDC")
	if err != nil {
		t.Fatalf("GetApprovalStatus: %v", err)
	}
	if status.Allowance != "2500000" || !status.Approved {
		t.Fatalf("approval status = %+v, want live approved allowance", status)
	}
	if checker.calls != 1 {
		t.Fatalf("checker calls = %d, want 1", checker.calls)
	}
	if checker.lastChain != "sepolia" || checker.lastUser != user.WalletAddress || checker.lastToken != "0x2222222222222222222222222222222222222222" {
		t.Fatalf("checker args chain=%s user=%s token=%s", checker.lastChain, checker.lastUser, checker.lastToken)
	}
	cached, err := st.GetWalletApproval(ctx, user.ID, "sepolia", "USDC")
	if err != nil {
		t.Fatalf("cached approval: %v", err)
	}
	if cached.Allowance != "2500000" {
		t.Fatalf("cached allowance = %q, want 2500000", cached.Allowance)
	}
}

func TestRecordApprovalVerifiesWithChecker(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user := mustUser(t, st, "0x0000000000000000000000000000000000000abc")
	checker := &fakeApprovalChecker{allowance: big.NewInt(0)}
	svc := newTestServiceWithApprovalChecker(t, st, checker)

	status, err := svc.RecordApproval(ctx, user.ID, service.RecordApprovalInput{
		Chain:           "sepolia",
		TokenSymbol:     "USDC",
		TxHash:          "0xapproval",
		DryRunAllowance: "999999999",
	})
	if err != nil {
		t.Fatalf("RecordApproval: %v", err)
	}
	if status.Allowance != "0" || status.Approved {
		t.Fatalf("recorded approval = %+v, want live zero allowance despite client input", status)
	}
	if checker.calls != 1 {
		t.Fatalf("checker calls = %d, want 1", checker.calls)
	}
	cached, err := st.GetWalletApproval(ctx, user.ID, "sepolia", "USDC")
	if err != nil {
		t.Fatalf("cached approval: %v", err)
	}
	if cached.Allowance != "0" {
		t.Fatalf("cached allowance = %q, want live zero", cached.Allowance)
	}
}

func TestListChainsReturnsPublicConfig(t *testing.T) {
	st := store.NewMemory()
	svc := newTestService(t, st)

	chains := svc.ListChains()
	if len(chains) != 1 {
		t.Fatalf("ListChains len = %d, want 1", len(chains))
	}
	if chains[0].Key != "sepolia" {
		t.Fatalf("chain key = %q, want sepolia", chains[0].Key)
	}
	if chains[0].StakeEnforcerAddress != "0x1111111111111111111111111111111111111111" {
		t.Fatalf("enforcer = %q", chains[0].StakeEnforcerAddress)
	}
	if chains[0].Tokens["USDC"] != "0x2222222222222222222222222222222222222222" || chains[0].Tokens["USDT"] != "0x3333333333333333333333333333333333333333" {
		t.Fatalf("tokens = %+v", chains[0].Tokens)
	}

	chains[0].Tokens["USDC"] = "mutated"
	again := svc.ListChains()
	if again[0].Tokens["USDC"] == "mutated" {
		t.Fatal("ListChains must return a cloned token map")
	}
}

func TestCreateAPIKeyReturnsRawOnceAndStoresOnlyHash(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user := mustUser(t, st, "0xabc")
	svc := newTestService(t, st)

	created, err := svc.CreateAPIKey(ctx, user.ID, "zapier")
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if !strings.HasPrefix(created.Raw, "sk_") {
		t.Fatalf("raw key = %q, want sk_ prefix", created.Raw)
	}
	if created.Key.KeyHash == "" || strings.Contains(created.Key.KeyHash, created.Raw) {
		t.Fatalf("stored key should contain only a hash, got %+v", created.Key)
	}
	keys, err := svc.ListAPIKeys(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListAPIKeys: %v", err)
	}
	if len(keys) != 1 || keys[0].KeyHash != created.Key.KeyHash {
		t.Fatalf("ListAPIKeys mismatch: %+v", keys)
	}

	verified, err := auth.NewAPIKeyManager(st).Verify(ctx, created.Raw)
	if err != nil {
		t.Fatalf("raw key should verify against stored hash: %v", err)
	}
	if verified.ID != created.Key.ID {
		t.Fatalf("verified key id = %s, want %s", verified.ID, created.Key.ID)
	}
}

func TestListAPIKeysOmitsRevokedKeys(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user := mustUser(t, st, "0xabc")
	svc := newTestService(t, st)

	created, err := svc.CreateAPIKey(ctx, user.ID, "zapier")
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if err := svc.RevokeAPIKey(ctx, user.ID, created.Key.ID); err != nil {
		t.Fatalf("RevokeAPIKey: %v", err)
	}

	keys, err := svc.ListAPIKeys(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListAPIKeys: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("ListAPIKeys returned revoked keys: %+v", keys)
	}
}

func TestCreateAgentLinkGeneratesSkillWithoutPersistingRawSecrets(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user := mustUser(t, st, "0xabc")
	svc := newTestService(t, st)

	created, err := svc.CreateAgentLink(ctx, user.ID, service.CreateAgentLinkInput{Name: "codex"}, "https://api.goalstakes.test")
	if err != nil {
		t.Fatalf("CreateAgentLink: %v", err)
	}
	if !strings.HasPrefix(created.SkillURL, "https://api.goalstakes.test/agent-skills/agt_") || !strings.HasSuffix(created.SkillURL, ".md") {
		t.Fatalf("skill url = %q", created.SkillURL)
	}
	if created.AgentLink.APIKeyID == (domain.UUID{}) || created.AgentLink.ID == (domain.UUID{}) {
		t.Fatalf("agent link metadata = %+v", created.AgentLink)
	}

	links, err := svc.ListAgentLinks(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListAgentLinks: %v", err)
	}
	if len(links) != 1 || links[0].ID != created.AgentLink.ID {
		t.Fatalf("links = %+v", links)
	}

	token := strings.TrimSuffix(strings.TrimPrefix(created.SkillURL, "https://api.goalstakes.test/agent-skills/"), ".md")
	markdown, err := svc.AgentSkillMarkdown(ctx, token, "https://api.goalstakes.test")
	if err != nil {
		t.Fatalf("AgentSkillMarkdown: %v", err)
	}
	for _, expected := range []string{
		"Goal Stakes lets the user create do and avoid goals",
		"API base: https://api.goalstakes.test",
		"Authorization: Bearer sk_",
		"GET /api/v1/goals",
		"POST /api/v1/chat/audio",
		"Run once per day in the user's timezone.",
		"If at least one active unarchived goal exists",
		"remind the user to check in or report a violation",
		"Never ask for wallet seed phrases",
		"Do not mark a goal done from the reminder alone",
	} {
		if !strings.Contains(markdown, expected) {
			t.Fatalf("markdown missing %q in:\n%s", expected, markdown)
		}
	}
	rawSecret := extractServiceAgentSecret(t, markdown)
	if _, err := auth.NewAPIKeyManager(st).Verify(ctx, rawSecret); err != nil {
		t.Fatalf("generated agent secret should verify against stored hash: %v", err)
	}
	storedLinks, err := st.ListAgentLinksByUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListAgentLinksByUser: %v", err)
	}
	if strings.Contains(storedLinks[0].TokenHash, token) || strings.Contains(storedLinks[0].TokenHash, rawSecret) {
		t.Fatalf("agent link stored raw secret material: %+v", storedLinks[0])
	}
}

func TestAgentLinkRevocationAndExpirationInvalidateSkillAndSecret(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user := mustUser(t, st, "0xabc")
	current := fixedNow()
	svc, err := service.New(st, testChains(), service.WithClock(func() time.Time { return current }))
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}

	created, err := svc.CreateAgentLink(ctx, user.ID, service.CreateAgentLinkInput{Name: "codex"}, "https://api.goalstakes.test")
	if err != nil {
		t.Fatalf("CreateAgentLink: %v", err)
	}
	token := strings.TrimSuffix(strings.TrimPrefix(created.SkillURL, "https://api.goalstakes.test/agent-skills/"), ".md")
	markdown, err := svc.AgentSkillMarkdown(ctx, token, "https://api.goalstakes.test")
	if err != nil {
		t.Fatalf("AgentSkillMarkdown: %v", err)
	}
	rawSecret := extractServiceAgentSecret(t, markdown)

	if err := svc.RevokeAgentLink(ctx, user.ID, created.AgentLink.ID); err != nil {
		t.Fatalf("RevokeAgentLink: %v", err)
	}
	if _, err := svc.AgentSkillMarkdown(ctx, token, "https://api.goalstakes.test"); err == nil || !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("revoked AgentSkillMarkdown error = %v, want ErrNotFound", err)
	}
	if _, err := auth.NewAPIKeyManager(st).Verify(ctx, rawSecret); err == nil {
		t.Fatal("revoked agent API secret should not verify")
	}

	created, err = svc.CreateAgentLink(ctx, user.ID, service.CreateAgentLinkInput{Name: "codex 2"}, "https://api.goalstakes.test")
	if err != nil {
		t.Fatalf("CreateAgentLink #2: %v", err)
	}
	token = strings.TrimSuffix(strings.TrimPrefix(created.SkillURL, "https://api.goalstakes.test/agent-skills/"), ".md")
	current = created.AgentLink.ExpiresAt.Add(time.Second)
	if _, err := svc.AgentSkillMarkdown(ctx, token, "https://api.goalstakes.test"); err == nil || !errors.Is(err, service.ErrExpired) {
		t.Fatalf("expired AgentSkillMarkdown error = %v, want ErrExpired", err)
	}
}

func TestTelegramLinkCodeConsumesOnceAndResolvesChat(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user := mustUser(t, st, "0xabc")
	svc := newTestService(t, st)

	created, err := svc.CreateTelegramLinkCode(ctx, user.ID)
	if err != nil {
		t.Fatalf("CreateTelegramLinkCode: %v", err)
	}
	if len(created.Code) < 8 || strings.HasPrefix(created.Code, "sk_") {
		t.Fatalf("link code should be short and not an API key: %+v", created)
	}
	if !created.ExpiresAt.After(fixedNow()) {
		t.Fatalf("expires_at = %v, want after now", created.ExpiresAt)
	}

	link, err := svc.LinkTelegramChat(ctx, service.LinkTelegramChatInput{
		Code:     created.Code,
		ChatID:   -1001234567890,
		ChatKind: "channel",
	})
	if err != nil {
		t.Fatalf("LinkTelegramChat: %v", err)
	}
	if link.UserID != user.ID || link.ChatID != -1001234567890 || link.ChatKind != "channel" {
		t.Fatalf("telegram link = %+v", link)
	}

	resolved, err := svc.ResolveTelegramChat(ctx, -1001234567890)
	if err != nil {
		t.Fatalf("ResolveTelegramChat: %v", err)
	}
	if resolved.UserID != user.ID {
		t.Fatalf("resolved link = %+v, want user %s", resolved, user.ID)
	}

	if _, err := svc.LinkTelegramChat(ctx, service.LinkTelegramChatInput{Code: created.Code, ChatID: 42, ChatKind: "private"}); err == nil {
		t.Fatal("LinkTelegramChat reused code must fail")
	} else if !strings.Contains(err.Error(), "invalid or expired telegram link code") {
		t.Fatalf("reused code error = %v", err)
	}
}

func TestTelegramLinkCodeExpires(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	user := mustUser(t, st, "0xabc")
	current := fixedNow()
	svc, err := service.New(st, testChains(), service.WithClock(func() time.Time { return current }))
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}

	created, err := svc.CreateTelegramLinkCode(ctx, user.ID)
	if err != nil {
		t.Fatalf("CreateTelegramLinkCode: %v", err)
	}
	current = created.ExpiresAt.Add(time.Second)

	if _, err := svc.LinkTelegramChat(ctx, service.LinkTelegramChatInput{Code: created.Code, ChatID: 42, ChatKind: "private"}); err == nil {
		t.Fatal("expired code must fail")
	} else if !strings.Contains(err.Error(), "invalid or expired telegram link code") {
		t.Fatalf("expired code error = %v", err)
	}
	if _, err := svc.ResolveTelegramChat(ctx, 42); err == nil {
		t.Fatal("expired code must not create a telegram link")
	}
}

func newTestService(t *testing.T, st store.Store) *service.Service {
	t.Helper()
	svc, err := service.New(st, testChains(), service.WithClock(fixedNow))
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	return svc
}

func newTestServiceWithCharger(t *testing.T, st store.Store, charger service.PenaltyCharger) *service.Service {
	t.Helper()
	svc, err := service.New(st, testChains(), service.WithClock(fixedNow), service.WithPenaltyCharger(charger))
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	return svc
}

func newTestServiceWithApprovalChecker(t *testing.T, st store.Store, checker service.ApprovalChecker) *service.Service {
	t.Helper()
	svc, err := service.New(st, testChains(), service.WithClock(fixedNow), service.WithApprovalChecker(checker))
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	return svc
}

func fixedNow() time.Time {
	return time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
}

type fakeCharger struct {
	txHash     string
	err        error
	calls      int
	lastChain  string
	lastUser   string
	lastToken  string
	lastAmount string
}

func (f *fakeCharger) Penalize(_ context.Context, chain, userWallet, tokenAddress, amount string) (string, error) {
	f.calls++
	f.lastChain = chain
	f.lastUser = userWallet
	f.lastToken = tokenAddress
	f.lastAmount = amount
	if f.err != nil {
		return f.txHash, f.err
	}
	return f.txHash, nil
}

type fakeApprovalChecker struct {
	allowance *big.Int
	err       error
	calls     int
	lastChain string
	lastUser  string
	lastToken string
}

func (f *fakeApprovalChecker) AllowanceOf(_ context.Context, chain, userWallet, tokenAddress string) (*big.Int, error) {
	f.calls++
	f.lastChain = chain
	f.lastUser = userWallet
	f.lastToken = tokenAddress
	if f.err != nil {
		return nil, f.err
	}
	if f.allowance == nil {
		return big.NewInt(0), nil
	}
	return new(big.Int).Set(f.allowance), nil
}

func testChains() map[string]config.ChainConfig {
	return map[string]config.ChainConfig{
		"sepolia": {
			RPCURL:               "https://sepolia.example/rpc",
			StakeEnforcerAddress: "0x1111111111111111111111111111111111111111",
			Tokens: map[string]string{
				"USDC": "0x2222222222222222222222222222222222222222",
				"USDT": "0x3333333333333333333333333333333333333333",
			},
		},
	}
}

func mustUser(t *testing.T, st store.Store, wallet string) domain.User {
	t.Helper()
	u, err := st.CreateUser(context.Background(), wallet, "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return u
}

func mustGoal(t *testing.T, svc *service.Service, userID domain.UUID) domain.Goal {
	t.Helper()
	return mustGoalWithType(t, svc, userID, domain.GoalDo)
}

func mustGoalWithType(t *testing.T, svc *service.Service, userID domain.UUID, goalType domain.GoalType) domain.Goal {
	t.Helper()
	if _, err := svc.RecordApproval(context.Background(), userID, service.RecordApprovalInput{
		Chain:           "sepolia",
		TokenSymbol:     "USDC",
		TxHash:          "0xtest-goal",
		DryRunAllowance: "1000000",
	}); err != nil {
		t.Fatalf("RecordApproval: %v", err)
	}
	goal, err := svc.CreateGoal(context.Background(), userID, service.CreateGoalInput{
		Title:       "Push-ups",
		Type:        goalType,
		Cadence:     domain.CadenceDaily,
		StakeAmount: "1000000",
		TokenSymbol: "USDC",
		Chain:       "sepolia",
	})
	if err != nil {
		t.Fatalf("CreateGoal: %v", err)
	}
	return goal
}

func extractServiceAgentSecret(t *testing.T, markdown string) string {
	t.Helper()
	for _, line := range strings.Split(markdown, "\n") {
		if secret, ok := strings.CutPrefix(strings.TrimSpace(line), "Authorization: Bearer "); ok {
			return strings.TrimSpace(secret)
		}
	}
	t.Fatalf("skill markdown did not contain Authorization bearer line:\n%s", markdown)
	return ""
}
