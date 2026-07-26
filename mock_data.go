package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/canopy-network/canopy/fsm"
	"github.com/canopy-network/canopy/lib"
	"github.com/canopy-network/canopy/lib/crypto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	mockNetworkID = 1
)

var baseBlockTimeMicro = time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC).UnixMicro()

type mockChain struct {
	chainID              uint64
	blocks               map[uint64]*lib.BlockResult
	certs                map[uint64]*lib.QuorumCertificate
	txs                  map[uint64][]*lib.TxResult
	events               map[uint64][]*lib.Event
	dexBatches           map[uint64]*lib.DexBatch
	nextDexBatches       map[uint64]*lib.DexBatch
	states               map[uint64]*fsm.GenesisState
	accounts             []*fsm.Account
	pools                []*fsm.Pool
	validators           []*fsm.Validator
	orderBooks           *lib.OrderBooks
	dexPrices            []lib.DexPrice
	params               *fsm.Params
	committees           *lib.CommitteesData
	subsidizedCommittees []uint64
	retiredCommittees    []uint64
	poll                 fsm.Poll
	proposals            fsm.GovProposals
	lockedBatch          *lib.DexBatch
	lockedBatchHeight    uint64
}

func newMockChain(numBlocks int, chainID uint64) *mockChain {
	mc := &mockChain{
		chainID:        chainID,
		blocks:         make(map[uint64]*lib.BlockResult),
		certs:          make(map[uint64]*lib.QuorumCertificate),
		txs:            make(map[uint64][]*lib.TxResult),
		events:         make(map[uint64][]*lib.Event),
		dexBatches:     make(map[uint64]*lib.DexBatch),
		nextDexBatches: make(map[uint64]*lib.DexBatch),
		states:         make(map[uint64]*fsm.GenesisState),
	}

	rng := rand.New(rand.NewSource(int64(chainID)))
	mc.validators = buildValidators(chainID, rng)
	mc.accounts = buildAccounts(mc.validators, rng)
	mc.pools = buildPools(mc.accounts, chainID, rng)
	mc.orderBooks = buildOrderBooks(mc.validators, chainID)
	mc.dexPrices = buildDexPrices(chainID)
	mc.params = buildParams(chainID)
	mc.committees = buildCommittees(mc.validators)
	mc.subsidizedCommittees = []uint64{chainID, chainID + 1}
	mc.retiredCommittees = []uint64{42}
	mc.poll = buildPoll(mc.validators)
	mc.proposals = buildProposals(mc.validators)

	profile := profileForChain(mc.chainID)
	addrs := make([][]byte, len(mc.accounts))
	for i, a := range mc.accounts {
		addrs[i] = a.Address
	}
	mc.lockedBatch = mc.generateDexBatch(0, false, nil, nil, nil)
	mc.states[0] = mc.snapshotStateAt(addrs, 0)
	for h := 1; h <= numBlocks; h++ {
		height := uint64(h)
		txResults := mc.generateClosedFormTxs(height, profile)
		events := mc.generateClosedFormEvents(height, profile)

		mc.txs[height] = txResults
		mc.events[height] = events

		block := mc.generateBlock(height, txResults, events)
		mc.blocks[height] = block
		mc.dexBatches[height] = mc.lockedBatch
		mc.nextDexBatches[height] = mc.generateDexBatch(height, true, nil, nil, nil)
		mc.states[height] = mc.snapshotStateAt(addrs, height)
		mc.certs[height] = mc.buildCertificateForTx(height)
	}

	return mc
}

func buildAccounts(validators []*fsm.Validator, rng *rand.Rand) []*fsm.Account {
	accounts := make([]*fsm.Account, 0, len(validators))
	for i, v := range validators {
		accounts = append(accounts, &fsm.Account{
			Address: v.Address,
			Amount:  1_000_000 + uint64(i)*100_000 + uint64(rng.Intn(50_000)),
		})
	}
	return accounts
}

func buildPools(accounts []*fsm.Account, chainID uint64, rng *rand.Rand) []*fsm.Pool {
	holdingID := chainID + fsm.HoldingPoolAddend
	liquidityID := chainID + fsm.LiquidityPoolAddend
	return []*fsm.Pool{
		{Id: chainID, Amount: 5_000_000 + uint64(rng.Intn(500_000))},   // stake/reward pool
		{Id: holdingID, Amount: 2_000_000 + uint64(rng.Intn(200_000))}, // holding pool
		{Id: liquidityID, Amount: 3_000_000 + uint64(rng.Intn(300_000)), Points: []*lib.PoolPoints{{Address: accounts[0].Address, Points: 100}}, TotalPoolPoints: 100},
	}
}

var printOnce = new(sync.Once)

func buildValidators(chainID uint64, rng *rand.Rand) []*fsm.Validator {
	seeds := []string{
		"000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
		"1f1e1d1c1b1a191817161514131211100f0e0d0c0b0a09080706050403020100",
		"1111111111111111111111111111111111111111111111111111111111111111",
		"2222222222222222222222222222222222222222222222222222222222222222",
		"3333333333333333333333333333333333333333333333333333333333333333",
		"4444444444444444444444444444444444444444444444444444444444444444",
		"5555555555555555555555555555555555555555555555555555555555555555",
		"6666666666666666666666666666666666666666666666666666666666666666",
		"7777777777777777777777777777777777777777777777777777777777777777",
		"8888888888888888888888888888888888888888888888888888888888888888",
	}
	validators := make([]*fsm.Validator, 0, len(seeds))
	for i, seedHex := range seeds {
		seed, _ := hex.DecodeString(seedHex)
		priv := ed25519.NewKeyFromSeed(seed)
		pub := priv.Public().(ed25519.PublicKey)
		pubKey, _ := crypto.NewPublicKeyFromBytes(pub)
		validators = append(validators, &fsm.Validator{
			Address:         pubKey.Address().Bytes(),
			PublicKey:       pubKey.Bytes(),
			NetAddress:      fmt.Sprintf("127.0.0.1:%d", 26650+i+int(chainID*10)),
			StakedAmount:    2_000_000 + uint64(i)*100_000 + uint64(rng.Intn(50_000)),
			Committees:      []uint64{chainID},
			MaxPausedHeight: 0,
			UnstakingHeight: 0,
			Output:          pubKey.Address().Bytes(),
			Delegate:        rng.Intn(2) == 0,
			Compound:        rng.Intn(2) == 0,
		})
	}
	printOnce.Do(func() {
		ks := crypto.NewKeystoreInMemory()
		for i, seedHex := range seeds {
			seed, _ := hex.DecodeString(seedHex)
			priv := ed25519.NewKeyFromSeed(seed)
			pk, err := crypto.NewPrivateKeyFromBytes(priv)
			if err != nil {
				panic(err)
			}
			if _, err = ks.ImportRaw(pk.Bytes(), "test", crypto.ImportRawOpts{
				Nickname: fmt.Sprintf("generated-key-%d", i+1)}); err != nil {
				panic(err)
			}
			fmt.Printf("Private Key %d: %x\n", i+1, priv)
		}
		fmt.Println(lib.MarshalJSONIndentString(ks))
	})
	return validators
}

func buildOrderBooks(validators []*fsm.Validator, chainID uint64) *lib.OrderBooks {
	v0 := validators[0].Address
	v1 := validators[1].Address
	orders := []*lib.SellOrder{
		{
			Id:                   hashBytes("order-1"),
			Committee:            chainID,
			Data:                 []byte("swap CNPY for sidechain asset"),
			AmountForSale:        50_000,
			RequestedAmount:      1_000,
			SellerReceiveAddress: v0,
			BuyerSendAddress:     v1,
			BuyerReceiveAddress:  v1,
			BuyerChainDeadline:   500,
			SellersSendAddress:   v0,
		},
		{
			Id:                   hashBytes("order-2"),
			Committee:            2,
			Data:                 []byte("swap sidechain for CNPY"),
			AmountForSale:        75_000,
			RequestedAmount:      2_500,
			SellerReceiveAddress: v1,
			BuyerSendAddress:     v0,
			BuyerReceiveAddress:  v0,
			BuyerChainDeadline:   700,
			SellersSendAddress:   v1,
		},
	}
	return &lib.OrderBooks{
		OrderBooks: []*lib.OrderBook{
			{ChainId: chainID, Orders: orders[:1]},
			{ChainId: 2, Orders: orders[1:]},
		},
	}
}

func buildDexPrices(chainID uint64) []lib.DexPrice {
	// scale price directly as localPool/remotePool in micro-units
	scale := func(local, remote uint64) uint64 {
		return (local * 1_000_000) / remote
	}
	return []lib.DexPrice{
		{LocalChainId: chainID, RemoteChainId: chainID + 1, LocalPool: 1, RemotePool: 10, E6ScaledPrice: scale(1, 10)},
		{LocalChainId: chainID + 1, RemoteChainId: chainID, LocalPool: 10, RemotePool: 1, E6ScaledPrice: scale(10, 1)},
	}
}

func buildParams(chainID uint64) *fsm.Params {
	return &fsm.Params{
		Consensus: &fsm.ConsensusParams{
			BlockSize:       1_000_000,
			ProtocolVersion: "mock-v1",
			RootChainId:     chainID,
			Retired:         0,
		},
		Validator: &fsm.ValidatorParams{
			UnstakingBlocks:                    100,
			MaxPauseBlocks:                     25,
			DoubleSignSlashPercentage:          5,
			NonSignSlashPercentage:             2,
			MaxNonSign:                         3,
			NonSignWindow:                      50,
			MaxCommittees:                      5,
			MaxCommitteeSize:                   15,
			EarlyWithdrawalPenalty:             10,
			DelegateUnstakingBlocks:            50,
			MinimumOrderSize:                   1000,
			StakePercentForSubsidizedCommittee: 25,
			MaxSlashPerCommittee:               1_000,
			DelegateRewardPercentage:           10,
			BuyDeadlineBlocks:                  15,
			LockOrderFeeMultiplier:             3,
			MinimumStakeForValidators:          50_000,
			MinimumStakeForDelegates:           10_000,
			MaximumDelegatesPerCommittee:       25,
		},
		Fee: &fsm.FeeParams{
			SendFee:                 10,
			StakeFee:                100,
			EditStakeFee:            5,
			UnstakeFee:              50,
			PauseFee:                1,
			UnpauseFee:              1,
			ChangeParameterFee:      500,
			DaoTransferFee:          250,
			CertificateResultsFee:   0,
			SubsidyFee:              0,
			CreateOrderFee:          125,
			EditOrderFee:            75,
			DeleteOrderFee:          60,
			DexLimitOrderFee:        80,
			DexLiquidityDepositFee:  25,
			DexLiquidityWithdrawFee: 25,
		},
		Governance: &fsm.GovernanceParams{DaoRewardPercentage: 15},
	}
}

func buildCommittees(validators []*fsm.Validator) *lib.CommitteesData {
	payment := func(recipient []byte, percent uint64, chainID uint64) *lib.PaymentPercents {
		return &lib.PaymentPercents{Address: recipient, Percent: percent, ChainId: chainID}
	}
	if len(validators) < 2 {
		return &lib.CommitteesData{}
	}
	chainID := validators[0].Committees[0]
	return &lib.CommitteesData{
		List: []*lib.CommitteeData{
			{
				ChainId:                chainID,
				LastRootHeightUpdated:  1,
				LastChainHeightUpdated: 1,
				PaymentPercents: []*lib.PaymentPercents{
					payment(validators[0].Address, 60, chainID),
					payment(validators[1].Address, 40, chainID),
				},
				NumberOfSamples: 10,
			},
		},
	}
}

// pickAccountIndex maps a hash seed onto [0, n) without ever going through a
// signed int cast of the raw uint64 first. addrHeightSeed's return value
// routinely exceeds math.MaxInt64; casting that straight to int wraps to a
// negative number, and Go's % keeps the sign of the dividend — so
// int(seed) % n can be negative and panic on the subsequent slice index.
// Reducing mod n while still in uint64 space avoids that entirely.
func pickAccountIndex(seed uint64, n int) int {
	return int(seed % uint64(n))
}

func (mc *mockChain) generateClosedFormTxs(height uint64, profile chainProfile) []*lib.TxResult {
	count := txCountAt(mc.chainID, profile, height)
	results := make([]*lib.TxResult, 0, count)
	for i := 0; i < count; i++ {
		typeLabel := txTypeMixAt(mc.chainID, profile, height+uint64(i)*1_000_003) // distinct salt per slot
		idx := pickAccountIndex(addrHeightSeed(mc.accounts[0].Address, height+uint64(i)), len(mc.accounts))
		sender := mc.accounts[idx].Address
		recipientIdx := pickAccountIndex(addrHeightSeed(mc.accounts[0].Address, height+uint64(i)+1), len(mc.accounts))
		recipient := mc.accounts[recipientIdx].Address
		txHash := hashBytes("tx", mc.chainID, height, i)
		msgType := messageTypeForLabel(typeLabel)
		anyMsg := mc.buildTxMsg(typeLabel, sender, recipient, height)
		results = append(results, &lib.TxResult{
			Sender:      sender,
			MessageType: msgType,
			Height:      height,
			Index:       uint64(i),
			Transaction: &lib.Transaction{
				MessageType:   msgType,
				Msg:           anyMsg,
				CreatedHeight: height,
				Time:          uint64(blockTimeMicro(height)),
				Fee:           10,
				NetworkId:     mockNetworkID,
				ChainId:       mc.chainID,
			},
			TxHash: hex.EncodeToString(txHash),
		})
	}
	return results
}

// buildTxMsg constructs a minimal-but-real fsm message for the given
// txTypeMixAt label and packs it into an *anypb.Any for Transaction.Msg.
// Field values are placeholders (not statistically calibrated) — the goal
// is a valid, non-empty, decodable payload, not realistic message content.
func (mc *mockChain) buildTxMsg(label string, sender, recipient []byte, height uint64) *anypb.Any {
	var msg proto.Message
	switch label {
	case "send":
		msg = &fsm.MessageSend{FromAddress: sender, ToAddress: recipient, Amount: 1_000 + height}
	case "dexLimitOrder":
		msg = &fsm.MessageDexLimitOrder{
			ChainId:         mc.chainID,
			AmountForSale:   2_500,
			RequestedAmount: 200,
			Address:         sender,
		}
	case "editStake":
		msg = &fsm.MessageEditStake{
			Address:       sender,
			Amount:        10_000 + height,
			Committees:    []uint64{mc.chainID},
			NetAddress:    fmt.Sprintf("127.0.0.1:%d", 26650),
			OutputAddress: sender,
			Compound:      false,
			Signer:        sender,
		}
	case "certResults":
		msg = &fsm.MessageCertificateResults{Qc: mc.buildCertificateForTx(height)}
	case "dexLiqDeposit":
		msg = &fsm.MessageDexLiquidityDeposit{ChainId: mc.chainID, Amount: 4_000, Address: sender}
	case "dexLiqWithdraw":
		msg = &fsm.MessageDexLiquidityWithdraw{ChainId: mc.chainID, Percent: 10, Address: sender}
	case "stake":
		msg = &fsm.MessageStake{
			PublicKey:     sender,
			Amount:        10_000,
			Committees:    []uint64{mc.chainID},
			NetAddress:    fmt.Sprintf("127.0.0.1:%d", 26650),
			OutputAddress: sender,
			Delegate:      false,
			Compound:      false,
			Signer:        sender,
		}
	case "unstake":
		msg = &fsm.MessageUnstake{Address: sender}
	default:
		msg = &fsm.MessageSend{FromAddress: sender, ToAddress: recipient, Amount: 1_000 + height}
	}
	anyMsg, err := anypb.New(msg)
	if err != nil {
		return nil
	}
	return anyMsg
}

// messageTypeForLabel maps the txTypeMixAt() weighted labels onto real fsm
// message-type name constants, so downstream consumers (e.g. tx-type-mix
// counters in canopy-indexer tests) see real message types.
func messageTypeForLabel(label string) string {
	switch label {
	case "send":
		return fsm.MessageSendName
	case "dexLimitOrder":
		return fsm.MessageDexLimitOrderName
	case "editStake":
		return fsm.MessageEditStakeName
	case "certResults":
		return fsm.MessageCertificateResultsName
	case "dexLiqDeposit":
		return fsm.MessageDexLiquidityDepositName
	case "dexLiqWithdraw":
		return fsm.MessageDexLiquidityWithdrawName
	case "stake":
		return fsm.MessageStakeName
	case "unstake":
		return fsm.MessageUnstakeName
	default:
		return fsm.MessageSendName
	}
}

func (mc *mockChain) generateClosedFormEvents(height uint64, profile chainProfile) []*lib.Event {
	count := eventCountAt(mc.chainID, profile, height)
	events := make([]*lib.Event, 0, count)
	bh := blockHash(height)
	for i := 0; i < count; i++ {
		label := eventTypeMixAt(mc.chainID, profile, height+uint64(i)*1_000_003)
		idx := pickAccountIndex(addrHeightSeed(mc.accounts[0].Address, height+uint64(i)), len(mc.accounts))
		addr := mc.accounts[idx].Address
		events = append(events, buildClosedFormEvent(label, height, mc.chainID, addr, bh)...)
	}
	return events
}

func buildClosedFormEvent(label string, height, chainID uint64, addr, blockHash []byte) []*lib.Event {
	base := &lib.Event{Height: height, ChainId: chainID, BlockHeight: height, BlockHash: blockHash, Address: addr}
	switch label {
	case "reward":
		base.EventType = string(lib.EventTypeReward)
		base.Msg = &lib.Event_Reward{Reward: &lib.EventReward{Amount: 5}}
	case "dexSwap":
		base.EventType = string(lib.EventTypeDexSwap)
		base.Msg = &lib.Event_DexSwap{DexSwap: &lib.EventDexSwap{SoldAmount: 1_000, BoughtAmount: 900, LocalOrigin: true, Success: true}}
	case "dexLiquidityDeposit":
		base.EventType = string(lib.EventTypeDexLiquidityDeposit)
		base.Msg = &lib.Event_DexLiquidityDeposit{DexLiquidityDeposit: &lib.EventDexLiquidityDeposit{Amount: 500, Points: 50}}
	default:
		base.EventType = string(lib.EventTypeReward)
		base.Msg = &lib.Event_Reward{Reward: &lib.EventReward{Amount: 5}}
	}
	return []*lib.Event{base}
}

// snapshotStateAt builds a GenesisState purely from closed-form lookups —
// no mutable ledger, no replay.
func (mc *mockChain) snapshotStateAt(addrs [][]byte, height uint64) *fsm.GenesisState {
	balances := snapshotBalancesAt(addrs, mc.chainID, height)
	accounts := make([]*fsm.Account, len(addrs))
	for i, a := range addrs {
		accounts[i] = &fsm.Account{Address: a, Amount: balances[i]}
	}
	validators := make([]*fsm.Validator, len(mc.validators))
	for i, v := range mc.validators {
		status := validatorStatusAt(v.Address, 0, height)
		unstakingHeight := uint64(0)
		if status == validatorUnstaked {
			unstakingHeight = unstakeHeightFor(v.Address, 0)
		}
		validators[i] = &fsm.Validator{
			Address:         v.Address,
			PublicKey:       v.PublicKey,
			NetAddress:      v.NetAddress,
			StakedAmount:    stakeAt(v.Address, mc.chainID, height),
			Committees:      v.Committees,
			UnstakingHeight: unstakingHeight,
			Output:          v.Output,
			Delegate:        v.Delegate,
			Compound:        v.Compound,
		}
	}
	var nonSigners fsm.NonSigners
	if len(mc.validators) >= 2 {
		nonSigners = fsm.NonSigners{
			{Address: mc.validators[0].Address, Counter: height%3 + 1},
			{Address: mc.validators[1].Address, Counter: height%2 + 1},
		}
	}

	var doubleSigners []*lib.DoubleSigner
	if height > 0 && height%30 == 0 && len(mc.validators) >= 2 {
		doubleSigners = []*lib.DoubleSigner{{Id: mc.validators[1].Address, Heights: []uint64{height - 1, height}}}
	}

	return &fsm.GenesisState{
		Time:              uint64(blockTimeMicro(height)),
		Pools:             mc.pools,
		Accounts:          accounts,
		Validators:        validators,
		Params:            mc.params,
		Supply:            &fsm.Supply{Total: totalSupplyAt(mc.chainID, height)},
		Committees:        mc.committees,
		OrderBooks:        mc.orderBooks,
		NonSigners:        nonSigners,
		DoubleSigners:     doubleSigners,
		RetiredCommittees: []uint64{42},
	}
}

func buildPoll(validators []*fsm.Validator) fsm.Poll {
	return fsm.Poll{
		hex.EncodeToString(hashBytes("proposal:change-fee")): fsm.PollResult{
			ProposalHash: hex.EncodeToString(hashBytes("proposal:change-fee")),
			ProposalURL:  "https://example.com/proposals/change-fee",
			Accounts: fsm.VoteStats{
				ApproveTokens:     600,
				RejectTokens:      200,
				TotalVotedTokens:  800,
				TotalTokens:       1000,
				ApprovePercentage: 60,
				RejectPercentage:  20,
				VotedPercentage:   80,
			},
			Validators: fsm.VoteStats{
				ApproveTokens:     300,
				RejectTokens:      100,
				TotalVotedTokens:  400,
				TotalTokens:       500,
				ApprovePercentage: 60,
				RejectPercentage:  20,
				VotedPercentage:   80,
			},
		},
	}
}

func buildProposals(validators []*fsm.Validator) fsm.GovProposals {
	signer := hex.EncodeToString(validators[0].Address)
	recipient := hex.EncodeToString(validators[1].Address)
	paramProposal := map[string]any{
		"type": "changeParameter",
		"msg": map[string]any{
			"parameterSpace": "fee",
			"parameterKey":   "send",
			"parameterValue": "15",
			"startHeight":    5,
			"endHeight":      50,
			"signer":         signer,
		},
	}
	daoProposal := map[string]any{
		"type": "daoTransfer",
		"msg": map[string]any{
			"address":     recipient,
			"amount":      250_000,
			"startHeight": 10,
			"endHeight":   100,
		},
	}
	paramRaw, _ := json.Marshal(paramProposal)
	daoRaw, _ := json.Marshal(daoProposal)
	return fsm.GovProposals{
		hex.EncodeToString(hashBytes("proposal:change-fee")): {
			Proposal: paramRaw,
			Approve:  true,
		},
		hex.EncodeToString(hashBytes("proposal:dao-treasury")): {
			Proposal: daoRaw,
			Approve:  true,
		},
	}
}

func (mc *mockChain) generateBlock(height uint64, txs []*lib.TxResult, events []*lib.Event) *lib.BlockResult {
	totalTxs := uint64(len(txs))
	if prev, ok := mc.blocks[height-1]; ok && prev != nil && prev.BlockHeader != nil {
		totalTxs += prev.BlockHeader.TotalTxs
	}
	header := &lib.BlockHeader{
		Height:             height,
		Hash:               blockHash(height),
		NetworkId:          mockNetworkID,
		Time:               uint64(blockTimeMicro(height)),
		NumTxs:             uint64(len(txs)),
		TotalTxs:           totalTxs,
		TotalVdfIterations: height * 100,
		LastBlockHash:      blockHash(height - 1),
		StateRoot:          hashBytes("state", height),
		TransactionRoot:    hashBytes("txroot", height),
		ValidatorRoot:      hashBytes("valroot", height),
		NextValidatorRoot:  hashBytes("nextval", height),
		ProposerAddress:    mc.validators[0].Address,
	}
	result := &lib.BlockResult{
		BlockHeader:  header,
		Transactions: txs,
		Events:       events,
	}
	bz, err := lib.Marshal(result)
	size := uint64(1024)
	if err == nil {
		size = uint64(len(bz))
	}
	result.Meta = &lib.BlockResultMeta{
		Size: size,
		Took: 2 + height%3,
	}
	return result
}

func (mc *mockChain) generateCertificate(height uint64, blockHash []byte) *lib.QuorumCertificate {
	return mc.buildCertificateForTx(height)
}

func (mc *mockChain) generateDexBatch(height uint64, next bool, orders []*lib.DexLimitOrder, deposits []*lib.DexLiquidityDeposit, withdrawals []*lib.DexLiquidityWithdraw) *lib.DexBatch {
	return &lib.DexBatch{
		Committee:        mc.chainID,
		ReceiptHash:      hashBytes("receipt", height, next),
		Orders:           orders,
		Deposits:         deposits,
		Withdrawals:      withdrawals,
		PoolSize:         1_000_000 + height*1_000,
		CounterPoolSize:  2_000_000 + height*500,
		PoolPoints:       []*lib.PoolPoints{},
		TotalPoolPoints:  100 + height,
		Receipts:         []uint64{height * 10},
		LockedHeight:     height,
		LivenessFallback: next,
	}
}

func (mc *mockChain) supplyAt(height uint64) *fsm.Supply {
	state := mc.stateAt(height)
	if state != nil && state.Supply != nil {
		return state.Supply
	}
	return nil
}

func (mc *mockChain) accountsAt(height uint64) []*fsm.Account {
	if state := mc.stateAt(height); state != nil {
		return state.Accounts
	}
	return nil
}

func (mc *mockChain) poolsAt(height uint64) []*fsm.Pool {
	if state := mc.stateAt(height); state != nil {
		return state.Pools
	}
	return nil
}

func (mc *mockChain) validatorsAt(height uint64) []*fsm.Validator {
	if state := mc.stateAt(height); state != nil {
		return state.Validators
	}
	return nil
}

func (mc *mockChain) stateAt(height uint64) *fsm.GenesisState {
	if gs, ok := mc.states[height]; ok {
		return gs
	}
	return nil
}

func blockTimeMicro(height uint64) int64 {
	return baseBlockTimeMicro + int64(height)*6_000_000
}

// buildCertificateForTx builds a QC populated for transaction payloads and cert-by-height responses
func (mc *mockChain) buildCertificateForTx(height uint64) *lib.QuorumCertificate {
	block := mc.blocks[height]
	var bh []byte
	if block != nil && block.BlockHeader != nil {
		bh = block.BlockHeader.Hash
	} else {
		bh = blockHash(height)
	}

	slashing := &lib.SlashRecipients{
		DoubleSigners: mc.doubleSigners(height),
	}
	return &lib.QuorumCertificate{
		Header: &lib.View{
			NetworkId:  mockNetworkID,
			ChainId:    mc.chainID,
			Height:     height,
			RootHeight: height,
			Round:      1,
			Phase:      lib.Phase_PRECOMMIT_VOTE,
		},
		Results: &lib.CertificateResult{
			RewardRecipients: &lib.RewardRecipients{
				PaymentPercents: []*lib.PaymentPercents{
					{Address: mc.validators[0].Address, Percent: 60, ChainId: mc.chainID},
					{Address: mc.validators[1].Address, Percent: 40, ChainId: mc.chainID},
				},
				NumberOfSamples: 10 + height,
			},
			Orders: &lib.Orders{
				LockOrders:  []*lib.LockOrder{},
				ResetOrders: [][]byte{},
				CloseOrders: [][]byte{},
			},
			Checkpoint: &lib.Checkpoint{
				Height:    height,
				BlockHash: bh,
			},
			DexBatch:        mc.dexBatches[height],
			SlashRecipients: slashing,
		},
		ResultsHash: hashBytes("result-hash", height),
		Block:       bh,
		BlockHash:   bh,
		ProposerKey: mc.validators[0].PublicKey,
		Signature: &lib.AggregateSignature{
			Signature: hashBytes("agg-sig", height)[:32],
			Bitmap:    []byte{0xff, 0x00},
		},
	}
}

func orderIDFromTxHashString(txHash string) []byte {
	if txHash == "" {
		return nil
	}
	hashBytes, err := hex.DecodeString(txHash)
	if err != nil {
		return nil
	}
	return orderIDFromTxHash(hashBytes)
}

func (mc *mockChain) latestHeight() uint64 {
	return uint64(len(mc.blocks))
}

func (mc *mockChain) dexPricesAt(height uint64) []lib.DexPrice {
	if height == 0 {
		height = mc.latestHeight()
	}
	batch, ok := mc.dexBatches[height]
	if !ok || batch == nil || batch.PoolSize == 0 || batch.CounterPoolSize == 0 {
		return nil
	}
	scale := func(local, remote uint64) uint64 {
		return lib.SafeMulDiv(local, 1_000_000, remote)
	}
	return []lib.DexPrice{
		{
			LocalChainId:  mc.chainID,
			RemoteChainId: mc.chainID + 1,
			LocalPool:     batch.PoolSize,
			RemotePool:    batch.CounterPoolSize,
			E6ScaledPrice: scale(batch.PoolSize, batch.CounterPoolSize),
		},
		{
			LocalChainId:  mc.chainID + 1,
			RemoteChainId: mc.chainID,
			LocalPool:     batch.CounterPoolSize,
			RemotePool:    batch.PoolSize,
			E6ScaledPrice: scale(batch.CounterPoolSize, batch.PoolSize),
		},
	}
}

func (mc *mockChain) nonSigners(height uint64) fsm.NonSigners {
	if height != 0 && height%10 == 0 {
		return nil
	}
	if state := mc.stateAt(height); state != nil {
		return state.NonSigners
	}
	return nil
}

func (mc *mockChain) doubleSigners(height uint64) []*lib.DoubleSigner {
	if state := mc.stateAt(height); state != nil {
		return state.DoubleSigners
	}
	return nil
}

func blockHash(height uint64) []byte {
	if height == 0 {
		return make([]byte, sha256.Size)
	}
	return hashBytes("block", height)
}

func hashBytes(parts ...any) []byte {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(fmt.Sprint(p)))
	}
	return h.Sum(nil)
}

func addressBytes(label string) []byte {
	return hashBytes(label)[:20]
}
