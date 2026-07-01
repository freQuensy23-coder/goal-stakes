//go:build e2e

package web3_test

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"goalstakes/internal/config"
	"goalstakes/internal/domain"
	"goalstakes/internal/scheduler"
	"goalstakes/internal/service"
	"goalstakes/internal/store"
	"goalstakes/internal/web3"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/accounts/abi/bind/backends"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

const simulatedChainID = 1337

func TestLocalE2EChargesEveryAvoidViolationThroughStakeEnforcer(t *testing.T) {
	ctx := context.Background()
	deployerKey := mustKey(t, "1000000000000000000000000000000000000000000000000000000000000001")
	enforcerKey := mustKey(t, "1000000000000000000000000000000000000000000000000000000000000002")
	userKey := mustKey(t, "1000000000000000000000000000000000000000000000000000000000000003")
	deployer := crypto.PubkeyToAddress(deployerKey.PublicKey)
	enforcerSigner := crypto.PubkeyToAddress(enforcerKey.PublicKey)
	userWallet := crypto.PubkeyToAddress(userKey.PublicKey)

	backend := &autoCommitBackend{SimulatedBackend: backends.NewSimulatedBackend(core.GenesisAlloc{
		deployer:       {Balance: big.NewInt(0).Mul(big.NewInt(100), big.NewInt(1e18))},
		enforcerSigner: {Balance: big.NewInt(0).Mul(big.NewInt(100), big.NewInt(1e18))},
		userWallet:     {Balance: big.NewInt(0).Mul(big.NewInt(100), big.NewInt(1e18))},
	}, 12_000_000)}
	defer backend.Close()

	tokenABI, tokenAddr := deployArtifact(t, backend, deployerKey, "MockERC20.sol", "MockERC20")
	stakeABI, stakeAddr := deployArtifact(t, backend, deployerKey, "StakeEnforcer.sol", "StakeEnforcer", enforcerSigner)

	token := bind.NewBoundContract(tokenAddr, tokenABI, backend, backend, backend)
	stakeAmount := big.NewInt(1_000_000)
	mintAmount := big.NewInt(2_000_000)
	totalPenalty := new(big.Int).Mul(stakeAmount, big.NewInt(2))
	transact(t, token, deployerKey, "mint", userWallet, mintAmount)
	transact(t, token, userKey, "approve", stakeAddr, totalPenalty)

	enforcer, err := web3.NewEnforcerWithBackend(ctx, "local", backend, stakeAddr, keyHex(enforcerKey), nil)
	if err != nil {
		t.Fatalf("newEnforcerWithBackend: %v", err)
	}
	st := store.NewMemory()
	user, err := st.CreateUser(ctx, userWallet.Hex(), "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	svc, err := service.New(st, map[string]config.ChainConfig{
		"local": {
			RPCURL:               "simulated://local",
			StakeEnforcerAddress: stakeAddr.Hex(),
			Tokens:               map[string]string{"USDC": tokenAddr.Hex()},
		},
	}, service.WithPenaltyCharger(enforcer), service.WithApprovalChecker(enforcer))
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}

	approval, err := svc.GetApprovalStatus(ctx, user.ID, "local", "USDC")
	if err != nil {
		t.Fatalf("GetApprovalStatus: %v", err)
	}
	if approval.Allowance != totalPenalty.String() {
		t.Fatalf("live allowance = %s, want %s", approval.Allowance, totalPenalty)
	}
	goal, err := svc.CreateGoal(ctx, user.ID, service.CreateGoalInput{
		Title:       "Avoid soda",
		Type:        domain.GoalAvoid,
		Cadence:     domain.CadenceDaily,
		StakeAmount: stakeAmount.String(),
		TokenSymbol: "USDC",
		Chain:       "local",
	})
	if err != nil {
		t.Fatalf("CreateGoal: %v", err)
	}
	violation, err := svc.ReportViolation(ctx, user.ID, goal.ID, service.ReportViolationInput{Reason: "local e2e slip"})
	if err != nil {
		t.Fatalf("ReportViolation #1: %v", err)
	}
	if violation.Status != domain.ViolationCharged || !strings.HasPrefix(violation.TxHash, "0x") {
		t.Fatalf("violation = %+v, want charged with tx hash", violation)
	}
	second, err := svc.ReportViolation(ctx, user.ID, goal.ID, service.ReportViolationInput{Reason: "local e2e second slip"})
	if err != nil {
		t.Fatalf("ReportViolation #2: %v", err)
	}
	if second.Status != domain.ViolationCharged || !strings.HasPrefix(second.TxHash, "0x") {
		t.Fatalf("second violation = %+v, want charged with tx hash", second)
	}
	if second.ID == violation.ID {
		t.Fatalf("avoid-goal reports should create separate violations: first=%s second=%s", violation.ID, second.ID)
	}
	if second.TxHash == violation.TxHash {
		t.Fatalf("each penalty should mine a distinct tx hash: %s", second.TxHash)
	}
	violations, err := st.ListViolations(ctx, goal.ID)
	if err != nil {
		t.Fatalf("ListViolations: %v", err)
	}
	if len(violations) != 2 {
		t.Fatalf("violations len=%d, want 2: %+v", len(violations), violations)
	}
	if got := readTokenBalance(t, token, userWallet); got.Cmp(new(big.Int).Sub(mintAmount, totalPenalty)) != 0 {
		t.Fatalf("user token balance = %s, want %s", got, new(big.Int).Sub(mintAmount, totalPenalty))
	}
	if got := readTokenBalance(t, token, web3.BurnAddress); got.Cmp(totalPenalty) != 0 {
		t.Fatalf("burn token balance = %s, want %s", got, totalPenalty)
	}
	remainingAllowance, err := enforcer.AllowanceOf(ctx, "local", userWallet.Hex(), tokenAddr.Hex())
	if err != nil {
		t.Fatalf("AllowanceOf after penalties: %v", err)
	}
	if remainingAllowance.Sign() != 0 {
		t.Fatalf("remaining allowance = %s, want 0", remainingAllowance)
	}
	if got := readAddress(t, bind.NewBoundContract(stakeAddr, stakeABI, backend, backend, backend), "enforcer"); got != enforcerSigner {
		t.Fatalf("StakeEnforcer enforcer = %s, want %s", got.Hex(), enforcerSigner.Hex())
	}
}

func TestLocalE2EChargesEthereumAndPolygonConfiguredChains(t *testing.T) {
	ctx := context.Background()
	deployerKey := mustKey(t, "1000000000000000000000000000000000000000000000000000000000000011")
	enforcerKey := mustKey(t, "1000000000000000000000000000000000000000000000000000000000000012")
	userKey := mustKey(t, "1000000000000000000000000000000000000000000000000000000000000013")
	enforcerSigner := crypto.PubkeyToAddress(enforcerKey.PublicKey)
	userWallet := crypto.PubkeyToAddress(userKey.PublicKey)

	ethereum := deploySimulatedStakeChain(t, deployerKey, enforcerKey, userKey, "ethereum", 1_000_000)
	defer ethereum.backend.Close()
	polygon := deploySimulatedStakeChain(t, deployerKey, enforcerKey, userKey, "polygon", 2_000_000)
	defer polygon.backend.Close()

	ethereumEnforcer, err := web3.NewEnforcerWithBackend(ctx, "ethereum", ethereum.backend, ethereum.stakeAddr, keyHex(enforcerKey), nil)
	if err != nil {
		t.Fatalf("new ethereum enforcer: %v", err)
	}
	polygonEnforcer, err := web3.NewEnforcerWithBackend(ctx, "polygon", polygon.backend, polygon.stakeAddr, keyHex(enforcerKey), nil)
	if err != nil {
		t.Fatalf("new polygon enforcer: %v", err)
	}
	enforcer := multiChainEnforcer{"ethereum": ethereumEnforcer, "polygon": polygonEnforcer}

	st := store.NewMemory()
	user, err := st.CreateUser(ctx, userWallet.Hex(), "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	svc, err := service.New(st, map[string]config.ChainConfig{
		"ethereum": {
			RPCURL:               "simulated://ethereum",
			StakeEnforcerAddress: ethereum.stakeAddr.Hex(),
			Tokens: map[string]string{
				"USDC": ethereum.tokenAddr.Hex(),
			},
		},
		"polygon": {
			RPCURL:               "simulated://polygon",
			StakeEnforcerAddress: polygon.stakeAddr.Hex(),
			Tokens: map[string]string{
				"USDT": polygon.tokenAddr.Hex(),
			},
		},
	}, service.WithPenaltyCharger(enforcer), service.WithApprovalChecker(enforcer))
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}

	assertConfiguredChainPenalty(t, ctx, svc, st, ethereum, user.ID, "ethereum", "USDC", "Ethereum USDC penalty")
	assertConfiguredChainPenalty(t, ctx, svc, st, polygon, user.ID, "polygon", "USDT", "Polygon USDT penalty")

	if got := readAddress(t, bind.NewBoundContract(ethereum.stakeAddr, ethereum.stakeABI, ethereum.backend, ethereum.backend, ethereum.backend), "enforcer"); got != enforcerSigner {
		t.Fatalf("ethereum StakeEnforcer enforcer = %s, want %s", got.Hex(), enforcerSigner.Hex())
	}
	if got := readAddress(t, bind.NewBoundContract(polygon.stakeAddr, polygon.stakeABI, polygon.backend, polygon.backend, polygon.backend), "enforcer"); got != enforcerSigner {
		t.Fatalf("polygon StakeEnforcer enforcer = %s, want %s", got.Hex(), enforcerSigner.Hex())
	}
}

func TestLocalE2ESchedulerChargesMissedDoGoalThroughStakeEnforcer(t *testing.T) {
	ctx := context.Background()
	deployerKey := mustKey(t, "1000000000000000000000000000000000000000000000000000000000000021")
	enforcerKey := mustKey(t, "1000000000000000000000000000000000000000000000000000000000000022")
	userKey := mustKey(t, "1000000000000000000000000000000000000000000000000000000000000023")
	userWallet := crypto.PubkeyToAddress(userKey.PublicKey)

	chain := deploySimulatedStakeChain(t, deployerKey, enforcerKey, userKey, "ethereum", 1_000_000)
	defer chain.backend.Close()
	enforcer, err := web3.NewEnforcerWithBackend(ctx, "ethereum", chain.backend, chain.stakeAddr, keyHex(enforcerKey), nil)
	if err != nil {
		t.Fatalf("new enforcer: %v", err)
	}

	st := store.NewMemory()
	user, err := st.CreateUser(ctx, userWallet.Hex(), "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	svc, err := service.New(st, map[string]config.ChainConfig{
		"ethereum": {
			RPCURL:               "simulated://ethereum",
			StakeEnforcerAddress: chain.stakeAddr.Hex(),
			Tokens: map[string]string{
				"USDC": chain.tokenAddr.Hex(),
			},
		},
	}, service.WithPenaltyCharger(enforcer), service.WithApprovalChecker(enforcer))
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	goal, err := svc.CreateGoal(ctx, user.ID, service.CreateGoalInput{
		Title:       "Do 100 push-ups every day",
		Type:        domain.GoalDo,
		Cadence:     domain.CadenceDaily,
		StakeAmount: chain.stakeAmount.String(),
		TokenSymbol: "USDC",
		Chain:       "ethereum",
		StartsAt:    time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateGoal: %v", err)
	}

	s := scheduler.New(st, svc)
	if err := s.RunOnce(ctx, time.Date(2026, 5, 26, 0, 1, 0, 0, time.UTC)); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	violations, err := st.ListViolations(ctx, goal.ID)
	if err != nil {
		t.Fatalf("ListViolations: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("violations len=%d, want 1: %+v", len(violations), violations)
	}
	if violations[0].Period != domain.Period("2026-05-25") || violations[0].Status != domain.ViolationCharged || !strings.HasPrefix(violations[0].TxHash, "0x") {
		t.Fatalf("scheduled violation = %+v, want charged missed 2026-05-25 with tx hash", violations[0])
	}
	if got := readTokenBalance(t, chain.token, chain.userWallet); got.Cmp(new(big.Int).Sub(chain.mintAmount, chain.stakeAmount)) != 0 {
		t.Fatalf("user token balance = %s, want %s", got, new(big.Int).Sub(chain.mintAmount, chain.stakeAmount))
	}
	if got := readTokenBalance(t, chain.token, web3.BurnAddress); got.Cmp(chain.stakeAmount) != 0 {
		t.Fatalf("burn token balance = %s, want %s", got, chain.stakeAmount)
	}

	if err := s.RunOnce(ctx, time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("RunOnce second: %v", err)
	}
	again, err := st.ListViolations(ctx, goal.ID)
	if err != nil {
		t.Fatalf("ListViolations second: %v", err)
	}
	if len(again) != 1 {
		t.Fatalf("second scheduler run created duplicate violations: %+v", again)
	}
	if got := readTokenBalance(t, chain.token, web3.BurnAddress); got.Cmp(chain.stakeAmount) != 0 {
		t.Fatalf("burn token balance after second run = %s, want still %s", got, chain.stakeAmount)
	}
}

type autoCommitBackend struct {
	*backends.SimulatedBackend
}

type multiChainEnforcer map[string]*web3.Enforcer

func (m multiChainEnforcer) Penalize(ctx context.Context, chain, userWallet, tokenAddress, amount string) (string, error) {
	enforcer, ok := m[chain]
	if !ok {
		return "", service.ErrInvalid
	}
	return enforcer.Penalize(ctx, chain, userWallet, tokenAddress, amount)
}

func (m multiChainEnforcer) AllowanceOf(ctx context.Context, chain, userWallet, tokenAddress string) (*big.Int, error) {
	enforcer, ok := m[chain]
	if !ok {
		return nil, service.ErrInvalid
	}
	return enforcer.AllowanceOf(ctx, chain, userWallet, tokenAddress)
}

func (b *autoCommitBackend) ChainID(context.Context) (*big.Int, error) {
	return big.NewInt(simulatedChainID), nil
}

func (b *autoCommitBackend) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	if err := b.SimulatedBackend.SendTransaction(ctx, tx); err != nil {
		return err
	}
	b.Commit()
	return nil
}

func deployArtifact(t *testing.T, backend *autoCommitBackend, key *ecdsa.PrivateKey, source, contract string, args ...any) (abi.ABI, common.Address) {
	t.Helper()
	contractABI, bytecode := loadArtifact(t, source, contract)
	addr, _, _, err := bind.DeployContract(txOpts(t, key), contractABI, bytecode, backend, args...)
	if err != nil {
		t.Fatalf("Deploy %s: %v", contract, err)
	}
	if code, err := backend.CodeAt(context.Background(), addr, nil); err != nil || len(code) == 0 {
		t.Fatalf("Deploy %s produced no code at %s: code=%d err=%v", contract, addr.Hex(), len(code), err)
	}
	return contractABI, addr
}

type simulatedStakeChain struct {
	name        string
	backend     *autoCommitBackend
	tokenABI    abi.ABI
	tokenAddr   common.Address
	token       *bind.BoundContract
	stakeABI    abi.ABI
	stakeAddr   common.Address
	stakeAmount *big.Int
	mintAmount  *big.Int
	userWallet  common.Address
}

func deploySimulatedStakeChain(t *testing.T, deployerKey, enforcerKey, userKey *ecdsa.PrivateKey, name string, stakeAmount int64) simulatedStakeChain {
	t.Helper()
	deployer := crypto.PubkeyToAddress(deployerKey.PublicKey)
	enforcerSigner := crypto.PubkeyToAddress(enforcerKey.PublicKey)
	userWallet := crypto.PubkeyToAddress(userKey.PublicKey)
	backend := &autoCommitBackend{SimulatedBackend: backends.NewSimulatedBackend(core.GenesisAlloc{
		deployer:       {Balance: big.NewInt(0).Mul(big.NewInt(100), big.NewInt(1e18))},
		enforcerSigner: {Balance: big.NewInt(0).Mul(big.NewInt(100), big.NewInt(1e18))},
		userWallet:     {Balance: big.NewInt(0).Mul(big.NewInt(100), big.NewInt(1e18))},
	}, 12_000_000)}
	tokenABI, tokenAddr := deployArtifact(t, backend, deployerKey, "MockERC20.sol", "MockERC20")
	stakeABI, stakeAddr := deployArtifact(t, backend, deployerKey, "StakeEnforcer.sol", "StakeEnforcer", enforcerSigner)
	token := bind.NewBoundContract(tokenAddr, tokenABI, backend, backend, backend)
	stake := big.NewInt(stakeAmount)
	mint := new(big.Int).Mul(stake, big.NewInt(2))
	transact(t, token, deployerKey, "mint", userWallet, mint)
	transact(t, token, userKey, "approve", stakeAddr, mint)
	return simulatedStakeChain{
		name:        name,
		backend:     backend,
		tokenABI:    tokenABI,
		tokenAddr:   tokenAddr,
		token:       token,
		stakeABI:    stakeABI,
		stakeAddr:   stakeAddr,
		stakeAmount: stake,
		mintAmount:  mint,
		userWallet:  userWallet,
	}
}

func assertConfiguredChainPenalty(t *testing.T, ctx context.Context, svc *service.Service, st store.Store, chain simulatedStakeChain, userID domain.UUID, chainName, tokenSymbol, title string) {
	t.Helper()
	approval, err := svc.GetApprovalStatus(ctx, userID, chainName, tokenSymbol)
	if err != nil {
		t.Fatalf("%s GetApprovalStatus: %v", chainName, err)
	}
	if approval.Allowance != chain.mintAmount.String() {
		t.Fatalf("%s live allowance = %s, want %s", chainName, approval.Allowance, chain.mintAmount)
	}
	goal, err := svc.CreateGoal(ctx, userID, service.CreateGoalInput{
		Title:       title,
		Type:        domain.GoalAvoid,
		Cadence:     domain.CadenceDaily,
		StakeAmount: chain.stakeAmount.String(),
		TokenSymbol: tokenSymbol,
		Chain:       chainName,
	})
	if err != nil {
		t.Fatalf("%s CreateGoal: %v", chainName, err)
	}
	violation, err := svc.ReportViolation(ctx, userID, goal.ID, service.ReportViolationInput{Reason: chainName + " configured-chain e2e slip"})
	if err != nil {
		t.Fatalf("%s ReportViolation: %v", chainName, err)
	}
	if violation.Status != domain.ViolationCharged || !strings.HasPrefix(violation.TxHash, "0x") {
		t.Fatalf("%s violation = %+v, want charged with tx hash", chainName, violation)
	}
	violations, err := st.ListViolations(ctx, goal.ID)
	if err != nil {
		t.Fatalf("%s ListViolations: %v", chainName, err)
	}
	if len(violations) != 1 {
		t.Fatalf("%s violations len=%d, want 1: %+v", chainName, len(violations), violations)
	}
	if got := readTokenBalance(t, chain.token, chain.userWallet); got.Cmp(new(big.Int).Sub(chain.mintAmount, chain.stakeAmount)) != 0 {
		t.Fatalf("%s user token balance = %s, want %s", chainName, got, new(big.Int).Sub(chain.mintAmount, chain.stakeAmount))
	}
	if got := readTokenBalance(t, chain.token, web3.BurnAddress); got.Cmp(chain.stakeAmount) != 0 {
		t.Fatalf("%s burn token balance = %s, want %s", chainName, got, chain.stakeAmount)
	}
}

func loadArtifact(t *testing.T, source, contract string) (abi.ABI, []byte) {
	t.Helper()
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "web3", "out", source, contract+".json"))
	if err != nil {
		t.Fatalf("read Foundry artifact for %s: %v; run `cd web3 && forge build` before e2e tests", contract, err)
	}
	var artifact struct {
		ABI      json.RawMessage `json:"abi"`
		Bytecode struct {
			Object string `json:"object"`
		} `json:"bytecode"`
	}
	if err := json.Unmarshal(raw, &artifact); err != nil {
		t.Fatalf("decode artifact %s: %v", contract, err)
	}
	contractABI, err := abi.JSON(strings.NewReader(string(artifact.ABI)))
	if err != nil {
		t.Fatalf("parse ABI %s: %v", contract, err)
	}
	bytecode := common.FromHex(artifact.Bytecode.Object)
	if len(bytecode) == 0 {
		t.Fatalf("artifact %s has empty bytecode; run `cd web3 && forge build`", contract)
	}
	return contractABI, bytecode
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func transact(t *testing.T, contract *bind.BoundContract, key *ecdsa.PrivateKey, method string, args ...any) {
	t.Helper()
	if _, err := contract.Transact(txOpts(t, key), method, args...); err != nil {
		t.Fatalf("%s transaction: %v", method, err)
	}
}

func txOpts(t *testing.T, key *ecdsa.PrivateKey) *bind.TransactOpts {
	t.Helper()
	opts, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(simulatedChainID))
	if err != nil {
		t.Fatalf("transactor: %v", err)
	}
	opts.Context = context.Background()
	return opts
}

func readTokenBalance(t *testing.T, token *bind.BoundContract, owner common.Address) *big.Int {
	t.Helper()
	var out []interface{}
	if err := token.Call(&bind.CallOpts{Context: context.Background()}, &out, "balanceOf", owner); err != nil {
		t.Fatalf("balanceOf(%s): %v", owner.Hex(), err)
	}
	if len(out) != 1 {
		t.Fatalf("balanceOf returned %d values", len(out))
	}
	balance, ok := out[0].(*big.Int)
	if !ok {
		t.Fatalf("balanceOf returned %T", out[0])
	}
	return balance
}

func readAddress(t *testing.T, contract *bind.BoundContract, method string) common.Address {
	t.Helper()
	var out []interface{}
	if err := contract.Call(&bind.CallOpts{Context: context.Background()}, &out, method); err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	address, ok := out[0].(common.Address)
	if !ok {
		t.Fatalf("%s returned %T", method, out[0])
	}
	return address
}

func mustKey(t *testing.T, hexKey string) *ecdsa.PrivateKey {
	t.Helper()
	key, err := crypto.HexToECDSA(hexKey)
	if err != nil {
		t.Fatalf("private key: %v", err)
	}
	return key
}

func keyHex(key *ecdsa.PrivateKey) string {
	return "0x" + common.Bytes2Hex(crypto.FromECDSA(key))
}
