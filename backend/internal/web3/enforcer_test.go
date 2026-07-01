package web3

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestPenaltyReceiptResultReturnsSubmittedHashWhenWaitFails(t *testing.T) {
	submitted := common.HexToHash("0x01")

	txHash, err := penaltyReceiptResult(submitted, nil, errors.New("context deadline exceeded"))
	if err == nil {
		t.Fatal("penaltyReceiptResult should surface wait errors")
	}
	if txHash != submitted.Hex() {
		t.Fatalf("txHash = %s, want submitted hash %s", txHash, submitted.Hex())
	}
	if !strings.Contains(err.Error(), submitted.Hex()) || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("error should include submitted hash and root cause: %v", err)
	}
}

func TestPenaltyReceiptResultRejectsRevertedReceipt(t *testing.T) {
	reverted := common.HexToHash("0x02")
	receipt := &types.Receipt{TxHash: reverted, Status: types.ReceiptStatusFailed}

	txHash, err := penaltyReceiptResult(common.HexToHash("0x01"), receipt, nil)
	if err == nil {
		t.Fatal("penaltyReceiptResult should reject reverted receipts")
	}
	if txHash != reverted.Hex() {
		t.Fatalf("txHash = %s, want receipt hash %s", txHash, reverted.Hex())
	}
	if !strings.Contains(err.Error(), "reverted") {
		t.Fatalf("error should describe reverted receipt: %v", err)
	}
}

func TestPenaltyReceiptResultReturnsSuccessfulReceiptHash(t *testing.T) {
	mined := common.HexToHash("0x03")
	receipt := &types.Receipt{TxHash: mined, Status: types.ReceiptStatusSuccessful}

	txHash, err := penaltyReceiptResult(common.HexToHash("0x01"), receipt, nil)
	if err != nil {
		t.Fatalf("penaltyReceiptResult: %v", err)
	}
	if txHash != mined.Hex() {
		t.Fatalf("txHash = %s, want receipt hash %s", txHash, mined.Hex())
	}
}

func TestValidateStakeEnforcerContract(t *testing.T) {
	ctx := context.Background()
	contractABI := mustStakeEnforcerABI(t)
	contract := common.HexToAddress("0x1111111111111111111111111111111111111111")
	expectedEnforcer := common.HexToAddress("0x2222222222222222222222222222222222222222")

	t.Run("accepts expected contract", func(t *testing.T) {
		caller := fakeContractCaller{
			code: []byte{0x60, 0x00},
			responses: map[string][]byte{
				selector(contractABI, "BURN"):     addressResult(t, contractABI, "BURN", burnAddress),
				selector(contractABI, "enforcer"): addressResult(t, contractABI, "enforcer", expectedEnforcer),
			},
		}

		if err := validateStakeEnforcerContract(ctx, &caller, contractABI, contract, expectedEnforcer); err != nil {
			t.Fatalf("validateStakeEnforcerContract: %v", err)
		}
	})

	t.Run("rejects missing code", func(t *testing.T) {
		caller := fakeContractCaller{code: nil}

		err := validateStakeEnforcerContract(ctx, &caller, contractABI, contract, expectedEnforcer)
		if err == nil {
			t.Fatal("expected missing-code error")
		}
		if !strings.Contains(err.Error(), "no contract code") {
			t.Fatalf("error = %v, want no contract code", err)
		}
	})

	t.Run("rejects wrong burn address", func(t *testing.T) {
		caller := fakeContractCaller{
			code: []byte{0x60, 0x00},
			responses: map[string][]byte{
				selector(contractABI, "BURN"):     addressResult(t, contractABI, "BURN", common.HexToAddress("0x3333333333333333333333333333333333333333")),
				selector(contractABI, "enforcer"): addressResult(t, contractABI, "enforcer", expectedEnforcer),
			},
		}

		err := validateStakeEnforcerContract(ctx, &caller, contractABI, contract, expectedEnforcer)
		if err == nil {
			t.Fatal("expected BURN mismatch error")
		}
		if !strings.Contains(err.Error(), "BURN") || !strings.Contains(err.Error(), burnAddress.Hex()) {
			t.Fatalf("error = %v, want BURN mismatch with expected burn address", err)
		}
	})

	t.Run("rejects enforcer mismatch", func(t *testing.T) {
		caller := fakeContractCaller{
			code: []byte{0x60, 0x00},
			responses: map[string][]byte{
				selector(contractABI, "BURN"):     addressResult(t, contractABI, "BURN", burnAddress),
				selector(contractABI, "enforcer"): addressResult(t, contractABI, "enforcer", common.HexToAddress("0x3333333333333333333333333333333333333333")),
			},
		}

		err := validateStakeEnforcerContract(ctx, &caller, contractABI, contract, expectedEnforcer)
		if err == nil {
			t.Fatal("expected enforcer mismatch error")
		}
		if !strings.Contains(err.Error(), "enforcer") || !strings.Contains(err.Error(), expectedEnforcer.Hex()) {
			t.Fatalf("error = %v, want enforcer mismatch with expected signer", err)
		}
	})
}

func TestValidateExpectedChainIDRejectsKnownChainMismatch(t *testing.T) {
	ctx := context.Background()
	err := validateExpectedChainID(ctx, "ethereum", fakeChainIDReader{chainID: big.NewInt(1337)})
	if err == nil {
		t.Fatal("expected ethereum chain ID mismatch")
	}
	if !strings.Contains(err.Error(), "chain ID") || !strings.Contains(err.Error(), "want 1") {
		t.Fatalf("error = %v, want ethereum chain ID mismatch", err)
	}
}

type fakeChainIDReader struct {
	chainID *big.Int
	err     error
}

func (f fakeChainIDReader) ChainID(context.Context) (*big.Int, error) {
	return f.chainID, f.err
}

type fakeContractCaller struct {
	code      []byte
	responses map[string][]byte
}

func (f *fakeContractCaller) CodeAt(context.Context, common.Address, *big.Int) ([]byte, error) {
	return f.code, nil
}

func (f *fakeContractCaller) CallContract(_ context.Context, call ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	if len(call.Data) < 4 {
		return nil, fmt.Errorf("missing selector")
	}
	out, ok := f.responses[string(call.Data[:4])]
	if !ok {
		return nil, fmt.Errorf("unexpected selector 0x%x", call.Data[:4])
	}
	return out, nil
}

func mustStakeEnforcerABI(t *testing.T) abi.ABI {
	t.Helper()
	contractABI, err := abi.JSON(strings.NewReader(stakeEnforcerABI))
	if err != nil {
		t.Fatalf("parse StakeEnforcer ABI: %v", err)
	}
	return contractABI
}

func selector(contractABI abi.ABI, method string) string {
	return string(contractABI.Methods[method].ID)
}

func addressResult(t *testing.T, contractABI abi.ABI, method string, address common.Address) []byte {
	t.Helper()
	raw, err := contractABI.Methods[method].Outputs.Pack(address)
	if err != nil {
		t.Fatalf("pack %s result: %v", method, err)
	}
	return raw
}
