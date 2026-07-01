package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"goalstakes/internal/domain"
	"goalstakes/internal/store"
)

// storeFactory builds a fresh, empty Store and a cleanup func. Implemented by
// both the memory and postgres test harnesses so the same suite runs against
// each (IF0: the memory store must be complete and correct, not a stub).
type storeFactory func(t *testing.T) (store.Store, func())

// runStoreSuite exercises the full Store contract against a factory.
func runStoreSuite(t *testing.T, newStore storeFactory) {
	t.Helper()
	t.Run("UserLifecycle", func(t *testing.T) { testUserLifecycle(t, newStore) })
	t.Run("GoalCRUD", func(t *testing.T) { testGoalCRUD(t, newStore) })
	t.Run("CheckInUpsert", func(t *testing.T) { testCheckInUpsert(t, newStore) })
	t.Run("ViolationIdempotency", func(t *testing.T) { testViolationIdempotency(t, newStore) })
	t.Run("ViolationUpdate", func(t *testing.T) { testViolationUpdate(t, newStore) })
	t.Run("WalletApprovalUpsert", func(t *testing.T) { testWalletApprovalUpsert(t, newStore) })
	t.Run("ApiKeyLifecycle", func(t *testing.T) { testApiKeyLifecycle(t, newStore) })
	t.Run("Conversation", func(t *testing.T) { testConversation(t, newStore) })
	t.Run("NotFound", func(t *testing.T) { testNotFound(t, newStore) })
}

func ctx() context.Context { return context.Background() }

func mkUser(t *testing.T, s store.Store) domain.User {
	t.Helper()
	u, err := s.CreateUser(ctx(), "0xabc123", "UTC")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return u
}

func mkGoal(t *testing.T, s store.Store, userID domain.UUID) domain.Goal {
	t.Helper()
	g, err := s.CreateGoal(ctx(), store.CreateGoalInput{
		UserID:      userID,
		Title:       "Run daily",
		Type:        domain.GoalDo,
		Cadence:     domain.CadenceDaily,
		StakeAmount: "1000000",
		TokenSymbol: "USDC",
		Chain:       "sepolia",
		StartsAt:    time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateGoal: %v", err)
	}
	return g
}

func testUserLifecycle(t *testing.T, newStore storeFactory) {
	s, cleanup := newStore(t)
	defer cleanup()

	u, err := s.CreateUser(ctx(), "0xWALLET", "Europe/Berlin")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.ID == (domain.UUID{}) {
		t.Fatal("CreateUser must assign an ID")
	}
	if u.WalletAddress != "0xWALLET" || u.Timezone != "Europe/Berlin" {
		t.Fatalf("unexpected user fields: %+v", u)
	}
	if u.CreatedAt.IsZero() {
		t.Fatal("CreateUser must set CreatedAt")
	}

	got, err := s.GetUserByWallet(ctx(), "0xWALLET")
	if err != nil {
		t.Fatalf("GetUserByWallet: %v", err)
	}
	if got.ID != u.ID {
		t.Fatalf("GetUserByWallet id=%s want %s", got.ID, u.ID)
	}
	gotByID, err := s.GetUser(ctx(), u.ID)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if gotByID.WalletAddress != u.WalletAddress {
		t.Fatalf("GetUser wallet=%s want %s", gotByID.WalletAddress, u.WalletAddress)
	}

	// Duplicate wallet must error (AS1: wallet is the unique identity).
	if _, err := s.CreateUser(ctx(), "0xWALLET", "UTC"); err == nil {
		t.Fatal("duplicate wallet must error")
	}
}

func testGoalCRUD(t *testing.T, newStore storeFactory) {
	s, cleanup := newStore(t)
	defer cleanup()
	u := mkUser(t, s)

	g := mkGoal(t, s, u.ID)
	if g.ID == (domain.UUID{}) || g.CreatedAt.IsZero() {
		t.Fatalf("CreateGoal must assign ID+CreatedAt: %+v", g)
	}
	if g.Archived {
		t.Fatal("new goal must not be archived")
	}

	got, err := s.GetGoal(ctx(), g.ID)
	if err != nil {
		t.Fatalf("GetGoal: %v", err)
	}
	if got.Title != "Run daily" {
		t.Fatalf("GetGoal title=%q", got.Title)
	}

	// A second goal for the same user; another user's goal must not leak in.
	g2 := mkGoal(t, s, u.ID)
	other := func() domain.User {
		ou, err := s.CreateUser(ctx(), "0xOTHER", "UTC")
		if err != nil {
			t.Fatalf("CreateUser other: %v", err)
		}
		return ou
	}()
	mkGoal(t, s, other.ID)

	list, err := s.GetGoalsByUser(ctx(), u.ID)
	if err != nil {
		t.Fatalf("GetGoalsByUser: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("GetGoalsByUser len=%d want 2", len(list))
	}
	seen := map[domain.UUID]bool{}
	for _, x := range list {
		seen[x.ID] = true
		if x.UserID != u.ID {
			t.Fatalf("leaked goal from another user: %+v", x)
		}
	}
	if !seen[g.ID] || !seen[g2.ID] {
		t.Fatal("GetGoalsByUser missing an expected goal")
	}

	// Update mutable fields.
	end := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	upd, err := s.UpdateGoal(ctx(), store.UpdateGoalInput{
		ID:          g.ID,
		Title:       "Run twice daily",
		Description: "harder",
		StakeAmount: "2000000",
		EndsAt:      &end,
	})
	if err != nil {
		t.Fatalf("UpdateGoal: %v", err)
	}
	if upd.Title != "Run twice daily" || upd.StakeAmount != "2000000" || upd.Description != "harder" {
		t.Fatalf("UpdateGoal did not persist fields: %+v", upd)
	}
	if upd.EndsAt == nil || !upd.EndsAt.Equal(end) {
		t.Fatalf("UpdateGoal EndsAt=%v want %v", upd.EndsAt, end)
	}
	// Immutable fields untouched.
	if upd.Type != domain.GoalDo || upd.Cadence != domain.CadenceDaily || upd.UserID != u.ID {
		t.Fatalf("UpdateGoal mutated immutable fields: %+v", upd)
	}

	// Archive.
	if err := s.ArchiveGoal(ctx(), g.ID); err != nil {
		t.Fatalf("ArchiveGoal: %v", err)
	}
	got, err = s.GetGoal(ctx(), g.ID)
	if err != nil {
		t.Fatalf("GetGoal after archive: %v", err)
	}
	if !got.Archived {
		t.Fatal("ArchiveGoal did not set Archived")
	}
	active, err := s.ListActiveGoals(ctx())
	if err != nil {
		t.Fatalf("ListActiveGoals: %v", err)
	}
	for _, x := range active {
		if x.ID == g.ID {
			t.Fatalf("archived goal leaked from ListActiveGoals: %+v", x)
		}
	}
}

func testCheckInUpsert(t *testing.T, newStore storeFactory) {
	s, cleanup := newStore(t)
	defer cleanup()
	u := mkUser(t, s)
	g := mkGoal(t, s, u.ID)
	period := domain.Period("2026-05-25")

	c1, err := s.UpsertCheckIn(ctx(), g.ID, period, "did it")
	if err != nil {
		t.Fatalf("UpsertCheckIn: %v", err)
	}
	if c1.ID == (domain.UUID{}) {
		t.Fatal("UpsertCheckIn must assign ID")
	}

	// Upsert again: same (goal, period) updates in place, no duplicate.
	c2, err := s.UpsertCheckIn(ctx(), g.ID, period, "did it again")
	if err != nil {
		t.Fatalf("UpsertCheckIn second: %v", err)
	}
	if c2.ID != c1.ID {
		t.Fatalf("UpsertCheckIn created a new row: %s != %s", c2.ID, c1.ID)
	}
	if c2.Note != "did it again" {
		t.Fatalf("UpsertCheckIn did not update note: %q", c2.Note)
	}

	got, err := s.GetCheckIn(ctx(), g.ID, period)
	if err != nil {
		t.Fatalf("GetCheckIn: %v", err)
	}
	if got.ID != c1.ID || got.Note != "did it again" {
		t.Fatalf("GetCheckIn mismatch: %+v", got)
	}
}

// testViolationIdempotency is the core IV6 test: calling CreateViolationIfAbsent
// twice for the same (goal, period) creates exactly one row and returns the
// existing one on the second call.
func testViolationIdempotency(t *testing.T, newStore storeFactory) {
	s, cleanup := newStore(t)
	defer cleanup()
	u := mkUser(t, s)
	g := mkGoal(t, s, u.ID)
	period := domain.Period("2026-W22")
	in := store.CreateViolationInput{
		GoalID: g.ID,
		Period: period,
		Amount: "1000000",
		Reason: "missed deadline",
	}

	v1, created, err := s.CreateViolationIfAbsent(ctx(), in)
	if err != nil {
		t.Fatalf("CreateViolationIfAbsent #1: %v", err)
	}
	if !created {
		t.Fatal("first CreateViolationIfAbsent must report created=true")
	}
	if v1.ID == (domain.UUID{}) {
		t.Fatal("first violation must have an ID")
	}
	if v1.Status != domain.ViolationPending {
		t.Fatalf("new violation status=%q want pending (IV6 write-before-charge)", v1.Status)
	}

	// Second call with the same key returns the SAME row, no new insert.
	in2 := in
	in2.Reason = "different reason should be ignored"
	in2.Amount = "9999999"
	v2, created, err := s.CreateViolationIfAbsent(ctx(), in2)
	if err != nil {
		t.Fatalf("CreateViolationIfAbsent #2: %v", err)
	}
	if created {
		t.Fatal("second CreateViolationIfAbsent must report created=false")
	}
	if v2.ID != v1.ID {
		t.Fatalf("idempotency broken: #2 id=%s != #1 id=%s", v2.ID, v1.ID)
	}
	if v2.Amount != v1.Amount || v2.Reason != v1.Reason {
		t.Fatalf("second call must return existing row unchanged: %+v", v2)
	}

	list, err := s.ListViolations(ctx(), g.ID)
	if err != nil {
		t.Fatalf("ListViolations: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("exactly one violation row expected, got %d (IV6)", len(list))
	}

	// A different period for the same goal yields a distinct row.
	other := in
	other.Period = domain.Period("2026-W23")
	vOther, created, err := s.CreateViolationIfAbsent(ctx(), other)
	if err != nil {
		t.Fatalf("CreateViolationIfAbsent other period: %v", err)
	}
	if !created {
		t.Fatal("different period should create a new violation row")
	}
	if vOther.ID == v1.ID {
		t.Fatal("different period must yield a different violation row")
	}
}

func testViolationUpdate(t *testing.T, newStore storeFactory) {
	s, cleanup := newStore(t)
	defer cleanup()
	u := mkUser(t, s)
	g := mkGoal(t, s, u.ID)

	v, _, err := s.CreateViolationIfAbsent(ctx(), store.CreateViolationInput{
		GoalID: g.ID,
		Period: "2026-05-25",
		Amount: "500",
		Reason: "missed",
	})
	if err != nil {
		t.Fatalf("CreateViolationIfAbsent: %v", err)
	}

	upd, err := s.UpdateViolation(ctx(), store.UpdateViolationInput{
		ID:     v.ID,
		Status: domain.ViolationCharged,
		TxHash: "0xdeadbeef",
	})
	if err != nil {
		t.Fatalf("UpdateViolation: %v", err)
	}
	if upd.Status != domain.ViolationCharged || upd.TxHash != "0xdeadbeef" {
		t.Fatalf("UpdateViolation did not persist: %+v", upd)
	}
	if !upd.UpdatedAt.After(v.UpdatedAt) && !upd.UpdatedAt.Equal(v.UpdatedAt) {
		t.Fatalf("UpdatedAt regressed: %v -> %v", v.UpdatedAt, upd.UpdatedAt)
	}
}

func testWalletApprovalUpsert(t *testing.T, newStore storeFactory) {
	s, cleanup := newStore(t)
	defer cleanup()
	u := mkUser(t, s)

	a1, err := s.UpsertWalletApproval(ctx(), u.ID, "sepolia", "USDC", "1000")
	if err != nil {
		t.Fatalf("UpsertWalletApproval: %v", err)
	}
	a2, err := s.UpsertWalletApproval(ctx(), u.ID, "sepolia", "USDC", "5000")
	if err != nil {
		t.Fatalf("UpsertWalletApproval update: %v", err)
	}
	if a2.ID != a1.ID {
		t.Fatalf("upsert created a new approval row: %s != %s", a2.ID, a1.ID)
	}
	if a2.Allowance != "5000" {
		t.Fatalf("allowance not updated: %q", a2.Allowance)
	}

	got, err := s.GetWalletApproval(ctx(), u.ID, "sepolia", "USDC")
	if err != nil {
		t.Fatalf("GetWalletApproval: %v", err)
	}
	if got.Allowance != "5000" {
		t.Fatalf("GetWalletApproval allowance=%q", got.Allowance)
	}

	// Different token is a distinct row.
	b, err := s.UpsertWalletApproval(ctx(), u.ID, "sepolia", "USDT", "1")
	if err != nil {
		t.Fatalf("UpsertWalletApproval USDT: %v", err)
	}
	if b.ID == a1.ID {
		t.Fatal("different token must be a different approval row")
	}
}

func testApiKeyLifecycle(t *testing.T, newStore storeFactory) {
	s, cleanup := newStore(t)
	defer cleanup()
	u := mkUser(t, s)

	k, err := s.CreateApiKey(ctx(), store.CreateApiKeyInput{
		UserID:  u.ID,
		Name:    "ci",
		Prefix:  "gs_live_abcd",
		KeyHash: "hash-of-secret",
	})
	if err != nil {
		t.Fatalf("CreateApiKey: %v", err)
	}
	if k.ID == (domain.UUID{}) {
		t.Fatal("CreateApiKey must assign ID")
	}
	// IV3: only hash + prefix are stored; there is no raw-key field at all.
	if k.KeyHash != "hash-of-secret" || k.Prefix != "gs_live_abcd" {
		t.Fatalf("CreateApiKey hash/prefix wrong: %+v", k)
	}

	got, err := s.GetApiKeyByHash(ctx(), "hash-of-secret")
	if err != nil {
		t.Fatalf("GetApiKeyByHash: %v", err)
	}
	if got.ID != k.ID {
		t.Fatalf("GetApiKeyByHash id=%s want %s", got.ID, k.ID)
	}

	list, err := s.ListApiKeysByUser(ctx(), u.ID)
	if err != nil {
		t.Fatalf("ListApiKeysByUser: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListApiKeysByUser len=%d want 1", len(list))
	}

	touched, err := s.TouchApiKey(ctx(), k.ID)
	if err != nil {
		t.Fatalf("TouchApiKey: %v", err)
	}
	if touched.LastUsed == nil {
		t.Fatal("TouchApiKey must set LastUsed")
	}
	got, err = s.GetApiKeyByHash(ctx(), "hash-of-secret")
	if err != nil {
		t.Fatalf("GetApiKeyByHash after touch: %v", err)
	}
	if got.LastUsed == nil {
		t.Fatal("TouchApiKey must persist LastUsed")
	}
	list, err = s.ListApiKeysByUser(ctx(), u.ID)
	if err != nil {
		t.Fatalf("ListApiKeysByUser after touch: %v", err)
	}
	if len(list) != 1 || list[0].LastUsed == nil {
		t.Fatalf("ListApiKeysByUser after touch returned %+v", list)
	}

	if err := s.RevokeApiKey(ctx(), k.ID); err != nil {
		t.Fatalf("RevokeApiKey: %v", err)
	}
	got, err = s.GetApiKeyByHash(ctx(), "hash-of-secret")
	if err != nil {
		t.Fatalf("GetApiKeyByHash after revoke: %v", err)
	}
	if !got.Revoked() {
		t.Fatal("RevokeApiKey did not set RevokedAt")
	}
}

func testConversation(t *testing.T, newStore storeFactory) {
	s, cleanup := newStore(t)
	defer cleanup()
	u := mkUser(t, s)

	c, err := s.CreateConversation(ctx(), u.ID, "coaching")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if c.ID == (domain.UUID{}) {
		t.Fatal("CreateConversation must assign ID")
	}

	m1, err := s.AppendMessage(ctx(), c.ID, domain.RoleUser, "hello")
	if err != nil {
		t.Fatalf("AppendMessage 1: %v", err)
	}
	m2, err := s.AppendMessage(ctx(), c.ID, domain.RoleAssistant, "hi there")
	if err != nil {
		t.Fatalf("AppendMessage 2: %v", err)
	}
	if m1.ID == m2.ID {
		t.Fatal("messages must have distinct IDs")
	}

	gotConv, msgs, err := s.GetConversation(ctx(), c.ID)
	if err != nil {
		t.Fatalf("GetConversation: %v", err)
	}
	if gotConv.ID != c.ID {
		t.Fatalf("GetConversation id=%s want %s", gotConv.ID, c.ID)
	}
	if len(msgs) != 2 {
		t.Fatalf("GetConversation msgs len=%d want 2", len(msgs))
	}
	// Order must be append order.
	if msgs[0].Content != "hello" || msgs[1].Content != "hi there" {
		t.Fatalf("message order wrong: %+v", msgs)
	}
}

func testNotFound(t *testing.T, newStore storeFactory) {
	s, cleanup := newStore(t)
	defer cleanup()

	missing := domain.NewID()
	if _, err := s.GetGoal(ctx(), missing); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetGoal missing: err=%v want ErrNotFound", err)
	}
	if _, err := s.GetUserByWallet(ctx(), "0xNOPE"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetUserByWallet missing: err=%v want ErrNotFound", err)
	}
	if _, err := s.GetCheckIn(ctx(), missing, "2026-05-25"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetCheckIn missing: err=%v want ErrNotFound", err)
	}
	if _, err := s.GetApiKeyByHash(ctx(), "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetApiKeyByHash missing: err=%v want ErrNotFound", err)
	}
	if _, err := s.TouchApiKey(ctx(), missing); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("TouchApiKey missing: err=%v want ErrNotFound", err)
	}
	if _, err := s.GetWalletApproval(ctx(), missing, "sepolia", "USDC"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetWalletApproval missing: err=%v want ErrNotFound", err)
	}
	if _, _, err := s.GetConversation(ctx(), missing); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetConversation missing: err=%v want ErrNotFound", err)
	}
}
