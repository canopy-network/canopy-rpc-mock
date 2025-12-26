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
	scheduledBatches     map[uint64]*lib.DexBatch
	nextBatchOrders      []*lib.DexLimitOrder
	nextBatchDeposits    []*lib.DexLiquidityDeposit
	nextBatchWithdrawals []*lib.DexLiquidityWithdraw
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
	mc.scheduledBatches = make(map[uint64]*lib.DexBatch)

	state := newMockState(mc.chainID, mc.accounts, mc.validators, mc.pools, mc.params, mc.orderBooks, mc.dexPrices, mc.committees)
	mc.states[0] = state.snapshot()
	mc.lockedBatch = mc.generateDexBatch(0, false, nil, nil, nil)
	for h := 1; h <= numBlocks; h++ {
		height := uint64(h)
		qc := state.beginBlock(height)

		mc.applyScheduledDex(state, height)
		mc.rotateDexBatches(state, height)
		blockTxs, txResults, _ := mc.buildBlockTransactions(state, height)
		mc.trackDexOps(txResults, height)

		allEvents := state.endBlock(mc.validators[0].Address)

		mc.txs[height] = txResults
		mc.events[height] = allEvents

		block := mc.generateBlock(height, txResults, allEvents)
		mc.blocks[height] = block
		mc.certs[height] = qc
		mc.dexBatches[height] = mc.lockedBatch
		mc.nextDexBatches[height] = mc.generateDexBatch(height, true, mc.nextBatchOrders, mc.nextBatchDeposits, mc.nextBatchWithdrawals)
		mc.states[height] = state.snapshot()

		_ = blockTxs // retained for future use/debug
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

// buildBlockTransactions creates a deterministic mix of transaction types for a height
func (mc *mockChain) buildBlockTransactions(state *mockState, height uint64) ([]*lib.Transaction, []*lib.TxResult, lib.Events) {
	var txs []*lib.Transaction
	var results []*lib.TxResult
	var events lib.Events
	builders := mc.txBuilders(state, height)
	for idx, builder := range builders {
		tx, res, ev := state.applyTx(builder.msgType, builder.msg)
		res.Index = uint64(idx)
		txs = append(txs, tx)
		results = append(results, res)
		events = append(events, ev...)
	}
	return txs, results, events
}

type txBuilder struct {
	msgType string
	msg     any
}

func (mc *mockChain) txBuilders(state *mockState, height uint64) []txBuilder {
	val := state.validatorsOrdered()[int(height)%len(state.validatorsOrdered())]
	nextVal := state.validatorsOrdered()[(int(height)+1)%len(state.validatorsOrdered())]
	recipient := state.validatorsOrdered()[(int(height)+2)%len(state.validatorsOrdered())].addr
	from := val.addr

	// cycle message types across blocks
	switch (height - 1) % 15 {
	case 0:
		return []txBuilder{{
			msgType: fsm.MessageSendName,
			msg:     &fsm.MessageSend{FromAddress: from, ToAddress: recipient, Amount: 1_000 + height},
		}}
	case 1:
		return []txBuilder{{
			msgType: fsm.MessageStakeName,
			msg:     &fsm.MessageStake{PublicKey: val.pubKey, Amount: 10_000, Committees: []uint64{mc.chainID}, NetAddress: val.netAddress, OutputAddress: val.outputAddress, Delegate: val.delegate, Compound: val.compound, Signer: from},
		}}
	case 2:
		return []txBuilder{{
			msgType: fsm.MessageEditStakeName,
			msg:     &fsm.MessageEditStake{Address: val.addr, Amount: val.stake + 5_000, Committees: val.committees, NetAddress: val.netAddress, OutputAddress: val.outputAddress, Compound: val.compound},
		}}
	case 3:
		return []txBuilder{{msgType: fsm.MessageUnstakeName, msg: &fsm.MessageUnstake{Address: val.addr}}}
	case 4:
		return []txBuilder{{msgType: fsm.MessagePauseName, msg: &fsm.MessagePause{Address: val.addr}}}
	case 5:
		return []txBuilder{{msgType: fsm.MessageUnpauseName, msg: &fsm.MessageUnpause{Address: val.addr}}}
	case 6:
		paramValue, _ := anypb.New(&lib.UInt64Wrapper{Value: 20})
		return []txBuilder{{msgType: fsm.MessageChangeParameterName, msg: &fsm.MessageChangeParameter{
			ParameterSpace: "fee",
			ParameterKey:   "sendFee",
			ParameterValue: paramValue,
			Signer:         from,
			ProposalHash:   hex.EncodeToString(hashBytes("param", height)),
		}}}
	case 7:
		return []txBuilder{{msgType: fsm.MessageDAOTransferName, msg: &fsm.MessageDAOTransfer{Address: recipient, Amount: 5_000, StartHeight: height, EndHeight: height + 10, ProposalHash: hex.EncodeToString(hashBytes("dao", height))}}}
	case 8:
		return []txBuilder{{
			msgType: fsm.MessageCertificateResultsName,
			msg:     &fsm.MessageCertificateResults{Qc: mc.buildCertificateForTx(height - 1)},
		}}
	case 9:
		return []txBuilder{{msgType: fsm.MessageSubsidyName, msg: &fsm.MessageSubsidy{Address: from, ChainId: mc.chainID, Amount: 2_000, Opcode: []byte("reinvest")}}}
	case 10:
		return []txBuilder{{msgType: fsm.MessageCreateOrderName, msg: &fsm.MessageCreateOrder{
			ChainId:              mc.chainID,
			Data:                 []byte("sell"),
			AmountForSale:        10_000,
			RequestedAmount:      500,
			SellerReceiveAddress: from,
			SellersSendAddress:   from,
		}}}
	case 11:
		return []txBuilder{{msgType: fsm.MessageEditOrderName, msg: &fsm.MessageEditOrder{
			ChainId:         mc.chainID,
			OrderId:         hashBytes("order", height-1, 0),
			AmountForSale:   12_000,
			RequestedAmount: 600,
		}}}
	case 12:
		return []txBuilder{{msgType: fsm.MessageDeleteOrderName, msg: &fsm.MessageDeleteOrder{
			ChainId: mc.chainID,
			OrderId: hashBytes("order", height-2, 0),
		}}}
	case 13:
		return []txBuilder{{msgType: fsm.MessageDexLimitOrderName, msg: &fsm.MessageDexLimitOrder{
			ChainId:         mc.chainID,
			AmountForSale:   2_500,
			RequestedAmount: 200,
			Address:         from,
		}}}
	case 14:
		return []txBuilder{
			{msgType: fsm.MessageDexLiquidityDepositName, msg: &fsm.MessageDexLiquidityDeposit{ChainId: mc.chainID, Amount: 4_000, Address: from}},
			{msgType: fsm.MessageDexLiquidityWithdrawName, msg: &fsm.MessageDexLiquidityWithdraw{ChainId: mc.chainID, Percent: 10, Address: nextVal.addr}},
		}
	default:
		return nil
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
	proposal := map[string]any{
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
	raw, _ := json.Marshal(proposal)
	return fsm.GovProposals{
		hex.EncodeToString(hashBytes("proposal:change-fee")): {
			Proposal: raw,
			Approve:  true,
		},
	}
}

func (mc *mockChain) generateTxs(height uint64) []*lib.TxResult {
	rng := rand.New(rand.NewSource(int64(height)))
	results := make([]*lib.TxResult, 0, 2)
	baseTime := blockTimeMicro(height)
	for i := 0; i < 2; i++ {
		sender := mc.accounts[(int(height)+i)%len(mc.accounts)].Address
		recipient := mc.accounts[(int(height)+i+1)%len(mc.accounts)].Address
		amount := uint64(1_000 + i*250 + int(height))
		message := &fsm.MessageSend{
			FromAddress: sender,
			ToAddress:   recipient,
			Amount:      amount,
		}
		anyMsg, _ := anypb.New(message)
		tx := &lib.Transaction{
			MessageType:   fsm.MessageSendName,
			Msg:           anyMsg,
			CreatedHeight: height,
			Time:          uint64(baseTime + int64(i)*250_000),
			Fee:           10,
			Memo:          fmt.Sprintf("mock transfer %d", height),
			NetworkId:     mockNetworkID,
			ChainId:       mc.chainID,
		}
		results = append(results, &lib.TxResult{
			Sender:      sender,
			Recipient:   recipient,
			MessageType: tx.MessageType,
			Height:      height,
			Index:       uint64(i),
			Transaction: tx,
			TxHash:      hex.EncodeToString(hashBytes("tx", height, i, rng.Int())),
		})
	}
	return results
}

func (mc *mockChain) generateEvents(height uint64) []*lib.Event {
	blockHash := blockHash(height)
	evts := make([]*lib.Event, 0, len(mc.txs[height])+1)
	for idx, tx := range mc.txs[height] {
		evts = append(evts, &lib.Event{
			EventType:   "reward",
			Msg:         &lib.Event_Reward{Reward: &lib.EventReward{Amount: 5 + uint64(idx)}},
			Height:      height,
			Reference:   tx.TxHash,
			ChainId:     mc.chainID,
			BlockHeight: height,
			BlockHash:   blockHash,
			Address:     tx.Recipient,
		})
	}
	// add a synthetic dex swap event every 5 blocks
	if height%5 == 0 {
		evts = append(evts, &lib.Event{
			EventType: "dexSwap",
			Msg: &lib.Event_DexSwap{
				DexSwap: &lib.EventDexSwap{
					SoldAmount:   1_000,
					BoughtAmount: 900,
					LocalOrigin:  true,
					Success:      true,
					OrderId:      orderIDFromTxHash(hashBytes("dex-swap", height)),
				},
			},
			Height:      height,
			Reference:   fmt.Sprintf("dex-%d", height),
			ChainId:     mc.chainID,
			BlockHeight: height,
			BlockHash:   blockHash,
			Address:     mc.accounts[0].Address,
		})
	}
	return evts
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
	return &lib.BlockResult{
		BlockHeader:  header,
		Transactions: txs,
		Events:       events,
		Meta: &lib.BlockResultMeta{
			Size: 1024 + height*10,
			Took: 2 + height%3,
		},
	}
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

func (mc *mockChain) trackDexOps(results []*lib.TxResult, height uint64) {
	for _, res := range results {
		if res == nil || res.Transaction == nil {
			continue
		}
		orderID := orderIDFromTxHashString(res.TxHash)
		switch res.Transaction.MessageType {
		case fsm.MessageDexLimitOrderName:
			msg := new(fsm.MessageDexLimitOrder)
			_ = anypb.UnmarshalTo(res.Transaction.Msg, msg, proto.UnmarshalOptions{})
			mc.nextBatchOrders = append(mc.nextBatchOrders, &lib.DexLimitOrder{
				AmountForSale:   msg.AmountForSale,
				RequestedAmount: msg.RequestedAmount,
				Address:         msg.Address,
				OrderId:         orderID,
			})
		case fsm.MessageDexLiquidityDepositName:
			msg := new(fsm.MessageDexLiquidityDeposit)
			_ = anypb.UnmarshalTo(res.Transaction.Msg, msg, proto.UnmarshalOptions{})
			mc.nextBatchDeposits = append(mc.nextBatchDeposits, &lib.DexLiquidityDeposit{Address: msg.Address, Amount: msg.Amount, OrderId: orderID})
		case fsm.MessageDexLiquidityWithdrawName:
			msg := new(fsm.MessageDexLiquidityWithdraw)
			_ = anypb.UnmarshalTo(res.Transaction.Msg, msg, proto.UnmarshalOptions{})
			mc.nextBatchWithdrawals = append(mc.nextBatchWithdrawals, &lib.DexLiquidityWithdraw{Address: msg.Address, Percent: msg.Percent, OrderId: orderID})
		}
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

func (mc *mockChain) rotateDexBatches(state *mockState, height uint64) {
	if height > 1 && height%4 == 2 {
		mc.lockedBatch = mc.generateDexBatch(height, false, mc.nextBatchOrders, mc.nextBatchDeposits, mc.nextBatchWithdrawals)
		mc.lockedBatchHeight = height
		mc.scheduledBatches[height+4] = mc.lockedBatch
		mc.nextBatchOrders = nil
		mc.nextBatchDeposits = nil
		mc.nextBatchWithdrawals = nil
	}
}

func (mc *mockChain) applyScheduledDex(state *mockState, height uint64) {
	batch, ok := mc.scheduledBatches[height]
	if !ok || batch == nil {
		return
	}
	delete(mc.scheduledBatches, height)

	state.applyBatchOrders(batch.Orders)
	state.applyBatchDeposits(batch.Deposits)
	state.applyBatchWithdrawals(batch.Withdrawals)

	var ev lib.Events
	for _, o := range batch.Orders {
		ev = append(ev, &lib.Event{
			EventType:   "dexSwap",
			Msg:         &lib.Event_DexSwap{DexSwap: &lib.EventDexSwap{SoldAmount: o.AmountForSale, BoughtAmount: o.RequestedAmount, LocalOrigin: true, Success: true, OrderId: o.OrderId}},
			Height:      height,
			BlockHeight: height,
			BlockHash:   blockHash(height),
			ChainId:     mc.chainID,
			Address:     o.Address,
			Reference:   "BEGIN_BLOCK",
		})
	}
	for _, d := range batch.Deposits {
		ev = append(ev, &lib.Event{
			EventType:   "dexLiquidityDeposit",
			Msg:         &lib.Event_DexLiquidityDeposit{DexLiquidityDeposit: &lib.EventDexLiquidityDeposit{Amount: d.Amount, Points: d.Amount / 10, OrderId: d.OrderId}},
			Height:      height,
			BlockHeight: height,
			BlockHash:   blockHash(height),
			ChainId:     mc.chainID,
			Address:     d.Address,
			Reference:   "BEGIN_BLOCK",
		})
	}
	for _, w := range batch.Withdrawals {
		ev = append(ev, &lib.Event{
			EventType: "dexLiquidityWithdrawal",
			Msg: &lib.Event_DexLiquidityWithdrawal{DexLiquidityWithdrawal: &lib.EventDexLiquidityWithdrawal{
				LocalAmount: 0, RemoteAmount: 0, OrderId: w.OrderId, PointsBurned: w.Percent,
			}},
			Height:      height,
			BlockHeight: height,
			BlockHash:   blockHash(height),
			ChainId:     mc.chainID,
			Address:     w.Address,
			Reference:   "BEGIN_BLOCK",
		})
	}
	if len(ev) > 0 {
		state.addEvents(nil, ev)
	}
}

func (mc *mockChain) latestHeight() uint64 {
	return uint64(len(mc.blocks))
}

func (mc *mockChain) nonSigners(height uint64) fsm.NonSigners {
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
