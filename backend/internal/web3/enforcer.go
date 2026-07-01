// Package web3 contains the StakeEnforcer contract client.
package web3

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"goalstakes/internal/config"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

const stakeEnforcerABI = `[{"type":"constructor","inputs":[{"name":"enforcer_","type":"address"}],"stateMutability":"nonpayable"},{"type":"function","name":"BURN","inputs":[],"outputs":[{"name":"","type":"address"}],"stateMutability":"view"},{"type":"function","name":"enforcer","inputs":[],"outputs":[{"name":"","type":"address"}],"stateMutability":"view"},{"type":"function","name":"penalize","inputs":[{"name":"user","type":"address"},{"name":"token","type":"address"},{"name":"amount","type":"uint256"}],"outputs":[],"stateMutability":"nonpayable"}]`

const erc20ABI = `[{"type":"function","name":"allowance","inputs":[{"name":"owner","type":"address"},{"name":"spender","type":"address"}],"outputs":[{"name":"","type":"uint256"}],"stateMutability":"view"}]`

var burnAddress = common.HexToAddress("0x000000000000000000000000000000000000dEaD")

var expectedChainIDs = map[string]int64{
	"ethereum":     1,
	"polygon":      137,
	"sepolia":      11155111,
	"polygon-amoy": 80002,
}

type Enforcer struct {
	privateKeyHex string
	chains        map[string]*chainClient
}

type evmBackend interface {
	bind.ContractBackend
	bind.DeployBackend
	ChainID(context.Context) (*big.Int, error)
}

type chainClient struct {
	client       evmBackend
	close        func()
	enforcerAddr common.Address
	enforcerABI  abi.ABI
	erc20ABI     abi.ABI
}

func NewEnforcer(ctx context.Context, chains map[string]config.ChainConfig, privateKeyHex string) (*Enforcer, error) {
	if strings.TrimSpace(privateKeyHex) == "" {
		return nil, errors.New("web3: ENFORCER_PRIVATE_KEY is required")
	}
	privateKey, err := crypto.HexToECDSA(strings.TrimPrefix(privateKeyHex, "0x"))
	if err != nil {
		return nil, fmt.Errorf("web3: invalid ENFORCER_PRIVATE_KEY: %w", err)
	}
	signerAddress := crypto.PubkeyToAddress(privateKey.PublicKey)
	enforcerABI, err := abi.JSON(strings.NewReader(stakeEnforcerABI))
	if err != nil {
		return nil, err
	}
	tokenABI, err := abi.JSON(strings.NewReader(erc20ABI))
	if err != nil {
		return nil, err
	}
	out := &Enforcer{privateKeyHex: privateKeyHex, chains: make(map[string]*chainClient)}
	for name, cfg := range chains {
		if !common.IsHexAddress(cfg.StakeEnforcerAddress) {
			out.Close()
			return nil, fmt.Errorf("web3: chain %q has invalid StakeEnforcer address", name)
		}
		client, err := ethclient.DialContext(ctx, cfg.RPCURL)
		if err != nil {
			out.Close()
			return nil, fmt.Errorf("web3: dial chain %q: %w", name, err)
		}
		enforcerAddr := common.HexToAddress(cfg.StakeEnforcerAddress)
		if err := validateExpectedChainID(ctx, name, client); err != nil {
			client.Close()
			out.Close()
			return nil, fmt.Errorf("web3: chain %q: %w", name, err)
		}
		if err := validateStakeEnforcerContract(ctx, client, enforcerABI, enforcerAddr, signerAddress); err != nil {
			client.Close()
			out.Close()
			return nil, fmt.Errorf("web3: chain %q: %w", name, err)
		}
		out.chains[name] = &chainClient{
			client:       client,
			close:        client.Close,
			enforcerAddr: enforcerAddr,
			enforcerABI:  enforcerABI,
			erc20ABI:     tokenABI,
		}
	}
	return out, nil
}

func newEnforcerWithBackend(ctx context.Context, chain string, backend evmBackend, enforcerAddr common.Address, privateKeyHex string, close func()) (*Enforcer, error) {
	privateKey, err := crypto.HexToECDSA(strings.TrimPrefix(privateKeyHex, "0x"))
	if err != nil {
		return nil, fmt.Errorf("web3: invalid ENFORCER_PRIVATE_KEY: %w", err)
	}
	signerAddress := crypto.PubkeyToAddress(privateKey.PublicKey)
	enforcerABI, err := abi.JSON(strings.NewReader(stakeEnforcerABI))
	if err != nil {
		return nil, err
	}
	tokenABI, err := abi.JSON(strings.NewReader(erc20ABI))
	if err != nil {
		return nil, err
	}
	if err := validateStakeEnforcerContract(ctx, backend, enforcerABI, enforcerAddr, signerAddress); err != nil {
		return nil, fmt.Errorf("web3: chain %q: %w", chain, err)
	}
	return &Enforcer{
		privateKeyHex: privateKeyHex,
		chains: map[string]*chainClient{
			chain: {
				client:       backend,
				close:        close,
				enforcerAddr: enforcerAddr,
				enforcerABI:  enforcerABI,
				erc20ABI:     tokenABI,
			},
		},
	}, nil
}

func (e *Enforcer) Penalize(ctx context.Context, chain, userWallet, tokenAddress, amount string) (string, error) {
	cc, ok := e.chains[chain]
	if !ok {
		return "", fmt.Errorf("web3: unknown chain %q", chain)
	}
	if !common.IsHexAddress(userWallet) {
		return "", fmt.Errorf("web3: invalid user wallet %q", userWallet)
	}
	if !common.IsHexAddress(tokenAddress) {
		return "", fmt.Errorf("web3: invalid token address %q", tokenAddress)
	}
	amountInt, ok := new(big.Int).SetString(amount, 10)
	if !ok || amountInt.Sign() <= 0 {
		return "", fmt.Errorf("web3: invalid penalty amount %q", amount)
	}
	privateKey, err := crypto.HexToECDSA(strings.TrimPrefix(e.privateKeyHex, "0x"))
	if err != nil {
		return "", fmt.Errorf("web3: invalid enforcer key: %w", err)
	}
	chainID, err := cc.client.ChainID(ctx)
	if err != nil {
		return "", fmt.Errorf("web3: chain id %q: %w", chain, err)
	}
	txOpts, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	if err != nil {
		return "", err
	}
	txOpts.Context = ctx
	contract := bind.NewBoundContract(cc.enforcerAddr, cc.enforcerABI, cc.client, cc.client, cc.client)
	tx, err := contract.Transact(txOpts, "penalize", common.HexToAddress(userWallet), common.HexToAddress(tokenAddress), amountInt)
	if err != nil {
		return "", fmt.Errorf("web3: penalize: %w", err)
	}
	receipt, err := bind.WaitMined(ctx, cc.client, tx)
	return penaltyReceiptResult(tx.Hash(), receipt, err)
}

func penaltyReceiptResult(submittedHash common.Hash, receipt *types.Receipt, waitErr error) (string, error) {
	if waitErr != nil {
		return submittedHash.Hex(), fmt.Errorf("web3: wait for penalize tx %s: %w", submittedHash.Hex(), waitErr)
	}
	if receipt == nil {
		return submittedHash.Hex(), fmt.Errorf("web3: penalize tx %s mined without receipt", submittedHash.Hex())
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return receipt.TxHash.Hex(), fmt.Errorf("web3: penalize tx %s reverted", receipt.TxHash.Hex())
	}
	return receipt.TxHash.Hex(), nil
}

func validateStakeEnforcerContract(ctx context.Context, caller bind.ContractCaller, contractABI abi.ABI, contractAddress, expectedEnforcer common.Address) error {
	code, err := caller.CodeAt(ctx, contractAddress, nil)
	if err != nil {
		return fmt.Errorf("read StakeEnforcer code at %s: %w", contractAddress.Hex(), err)
	}
	if len(code) == 0 {
		return fmt.Errorf("StakeEnforcer %s has no contract code", contractAddress.Hex())
	}
	burn, err := readAddressConstant(ctx, caller, contractABI, contractAddress, "BURN")
	if err != nil {
		return fmt.Errorf("read StakeEnforcer %s BURN: %w", contractAddress.Hex(), err)
	}
	if burn != burnAddress {
		return fmt.Errorf("StakeEnforcer %s BURN is %s, want %s", contractAddress.Hex(), burn.Hex(), burnAddress.Hex())
	}
	enforcer, err := readAddressConstant(ctx, caller, contractABI, contractAddress, "enforcer")
	if err != nil {
		return fmt.Errorf("read StakeEnforcer %s enforcer: %w", contractAddress.Hex(), err)
	}
	if enforcer != expectedEnforcer {
		return fmt.Errorf("StakeEnforcer %s enforcer is %s, want backend signer %s", contractAddress.Hex(), enforcer.Hex(), expectedEnforcer.Hex())
	}
	return nil
}

type chainIDReader interface {
	ChainID(context.Context) (*big.Int, error)
}

func validateExpectedChainID(ctx context.Context, chain string, backend chainIDReader) error {
	want, ok := expectedChainIDs[chain]
	if !ok {
		return nil
	}
	got, err := backend.ChainID(ctx)
	if err != nil {
		return fmt.Errorf("read chain ID: %w", err)
	}
	if got == nil {
		return errors.New("read chain ID: empty response")
	}
	wantBig := big.NewInt(want)
	if got.Cmp(wantBig) != 0 {
		return fmt.Errorf("chain ID is %s, want %s", got.String(), wantBig.String())
	}
	return nil
}

func readAddressConstant(ctx context.Context, caller bind.ContractCaller, contractABI abi.ABI, contractAddress common.Address, method string) (common.Address, error) {
	contract := bind.NewBoundContract(contractAddress, contractABI, caller, nil, nil)
	var out []interface{}
	if err := contract.Call(&bind.CallOpts{Context: ctx}, &out, method); err != nil {
		return common.Address{}, err
	}
	if len(out) != 1 {
		return common.Address{}, fmt.Errorf("%s returned %d values", method, len(out))
	}
	address, ok := out[0].(common.Address)
	if !ok {
		return common.Address{}, fmt.Errorf("%s returned %T", method, out[0])
	}
	return address, nil
}

func (e *Enforcer) AllowanceOf(ctx context.Context, chain, userWallet, tokenAddress string) (*big.Int, error) {
	cc, ok := e.chains[chain]
	if !ok {
		return nil, fmt.Errorf("web3: unknown chain %q", chain)
	}
	if !common.IsHexAddress(userWallet) {
		return nil, fmt.Errorf("web3: invalid user wallet %q", userWallet)
	}
	if !common.IsHexAddress(tokenAddress) {
		return nil, fmt.Errorf("web3: invalid token address %q", tokenAddress)
	}
	contract := bind.NewBoundContract(common.HexToAddress(tokenAddress), cc.erc20ABI, cc.client, cc.client, cc.client)
	var out []interface{}
	if err := contract.Call(&bind.CallOpts{Context: ctx}, &out, "allowance", common.HexToAddress(userWallet), cc.enforcerAddr); err != nil {
		return nil, fmt.Errorf("web3: allowance: %w", err)
	}
	if len(out) != 1 {
		return nil, fmt.Errorf("web3: allowance returned %d values", len(out))
	}
	allowance, ok := out[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("web3: allowance returned %T", out[0])
	}
	return allowance, nil
}

func (e *Enforcer) Close() {
	for _, cc := range e.chains {
		if cc.close != nil {
			cc.close()
		}
	}
}

type DisabledCharger struct {
	Reason string
}

func (d DisabledCharger) Penalize(context.Context, string, string, string, string) (string, error) {
	if d.Reason == "" {
		return "", errors.New("web3: penalty charger disabled")
	}
	return "", errors.New(d.Reason)
}
