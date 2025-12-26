package main

import (
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/canopy-network/canopy/fsm"
	"github.com/canopy-network/canopy/lib"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type validatorInfo struct {
	addr          []byte
	pubKey        []byte
	stake         uint64
	paused        bool
	delegate      bool
	compound      bool
	unstakingAt   uint64
	outputAddress []byte
	netAddress    string
	committees    []uint64
}

type mockState struct {
	chainID       uint64
	height        uint64
	accounts      map[string]uint64
	validators    map[string]*validatorInfo
	orderBooks    map[uint64][]*lib.SellOrder
	pools         map[uint64]*fsm.Pool
	params        *fsm.Params
	committeeSet  *lib.CommitteesData
	nonSigners    fsm.NonSigners
	doubleSign    []*lib.DoubleSigner
	dexPrices     []lib.DexPrice
	events        lib.Events
	delayedEvents map[uint64]lib.Events
}

func newMockState(chainID uint64, accounts []*fsm.Account, validators []*fsm.Validator, pools []*fsm.Pool, params *fsm.Params, orderBooks *lib.OrderBooks, dexPrices []lib.DexPrice, committees *lib.CommitteesData) *mockState {
	accMap := make(map[string]uint64)
	for _, a := range accounts {
		accMap[hex.EncodeToString(a.Address)] = a.Amount
	}
	valMap := make(map[string]*validatorInfo)
	for _, v := range validators {
		valMap[hex.EncodeToString(v.Address)] = &validatorInfo{
			addr:          v.Address,
			pubKey:        v.PublicKey,
			stake:         v.StakedAmount,
			paused:        false,
			delegate:      v.Delegate,
			compound:      v.Compound,
			unstakingAt:   v.UnstakingHeight,
			outputAddress: v.Output,
			netAddress:    v.NetAddress,
			committees:    v.Committees,
		}
	}
	poolMap := make(map[uint64]*fsm.Pool)
	for _, p := range pools {
		poolMap[p.Id] = p
	}
	ob := make(map[uint64][]*lib.SellOrder)
	for _, book := range orderBooks.OrderBooks {
		ob[book.ChainId] = book.Orders
	}
	return &mockState{
		chainID:       chainID,
		accounts:      accMap,
		validators:    valMap,
		pools:         poolMap,
		params:        params,
		orderBooks:    ob,
		committeeSet:  committees,
		dexPrices:     dexPrices,
		delayedEvents: make(map[uint64]lib.Events),
	}
}

func (s *mockState) beginBlock(height uint64) *lib.QuorumCertificate {
	s.height = height
	s.events = nil
	s.doubleSign = nil
	// simple inflation into pool 1 to mimic reward funding
	if pool, ok := s.pools[1]; ok {
		pool.Amount += 1_000
	}
	// non-signers tracking with non-zero counters
	vals := s.validatorsOrdered()
	if len(vals) >= 2 {
		s.nonSigners = fsm.NonSigners{
			{Address: vals[0].addr, Counter: (height%3 + 1)},
			{Address: vals[1].addr, Counter: (height%2 + 1)},
		}
	}
	// occasional slash evidence event
	if height%25 == 0 && len(vals) > 0 {
		s.addEvents(nil, buildEvent("slash", &lib.Event_Slash{Slash: &lib.EventSlash{Amount: 100}}, height, s.chainID, vals[0].addr, "BEGIN_BLOCK"))
	}
	if height%30 == 0 && len(vals) > 1 {
		s.doubleSign = []*lib.DoubleSigner{{Id: vals[1].addr, Heights: []uint64{height - 1, height}}}
	}
	// synthetic QC referencing current block
	qc := &lib.QuorumCertificate{
		Header: &lib.View{NetworkId: mockNetworkID, ChainId: s.chainID, Height: height, RootHeight: height, Round: 1, Phase: lib.Phase_PRECOMMIT_VOTE},
		Results: &lib.CertificateResult{
			RewardRecipients: &lib.RewardRecipients{
				PaymentPercents: []*lib.PaymentPercents{
					{Address: s.validatorsOrdered()[0].addr, Percent: 60, ChainId: s.chainID},
					{Address: s.validatorsOrdered()[1].addr, Percent: 40, ChainId: s.chainID},
				},
				NumberOfSamples: height,
			},
			Checkpoint: &lib.Checkpoint{Height: height, BlockHash: blockHash(height)},
		},
		BlockHash: blockHash(height),
	}
	return qc
}

func (s *mockState) endBlock(proposer []byte) lib.Events {
	// distribute a small reward to proposer account
	propKey := hex.EncodeToString(proposer)
	s.accounts[propKey] += 500
	// unpause anyone scheduled to unpause and delete finished unstaking
	for _, val := range s.validatorsOrdered() {
		key := hex.EncodeToString(val.addr)
		if val.unstakingAt > 0 && val.unstakingAt <= s.height {
			s.accounts[hex.EncodeToString(val.outputAddress)] += val.stake
			val.stake = 0
			val.unstakingAt = 0
			evt := buildEvent("finish-unstaking", &lib.Event_FinishUnstaking{FinishUnstaking: &lib.EventFinishUnstaking{}}, s.height, s.chainID, val.addr, "BEGIN_BLOCK")
			s.addEvents(nil, evt)
		}
		if val.paused && s.height%5 == 0 {
			val.paused = false
		}
		s.validators[key] = val
	}
	events := append(lib.Events{}, s.events...)
	if delayed := s.delayedEvents[s.height]; len(delayed) > 0 {
		events = append(events, delayed...)
		delete(s.delayedEvents, s.height)
	}
	return events
}

func (s *mockState) applyTx(txType string, msg any) (tx *lib.Transaction, result *lib.TxResult, events lib.Events) {
	var sender []byte
	txHash := hashBytes(txType, s.height)
	txHashStr := hex.EncodeToString(txHash)
	orderID := orderIDFromTxHashString(txHashStr)
	switch m := msg.(type) {
	case *fsm.MessageSend:
		sender = m.FromAddress
		fromKey, toKey := hex.EncodeToString(m.FromAddress), hex.EncodeToString(m.ToAddress)
		if s.accounts[fromKey] >= m.Amount {
			s.accounts[fromKey] -= m.Amount
			s.accounts[toKey] += m.Amount
		}
		events = s.addEvents(events, lib.Events{{
			EventType:   "reward",
			Msg:         &lib.Event_Reward{Reward: &lib.EventReward{Amount: m.Amount / 100}},
			Height:      s.height,
			Reference:   txHashStr,
			ChainId:     s.chainID,
			BlockHeight: s.height,
			BlockHash:   blockHash(s.height),
			Address:     m.ToAddress,
		}})
	case *fsm.MessageStake:
		sender = m.Signer
		key := hex.EncodeToString(m.Signer)
		if s.accounts[key] >= m.Amount {
			s.accounts[key] -= m.Amount
			if val, ok := s.validators[key]; ok {
				val.stake += m.Amount
			}
		}
	case *fsm.MessageEditStake:
		sender = m.Address
		key := hex.EncodeToString(m.Address)
		if val, ok := s.validators[key]; ok {
			val.stake = m.Amount
		}
	case *fsm.MessageUnstake:
		sender = m.Address
		key := hex.EncodeToString(m.Address)
		if val, ok := s.validators[key]; ok {
			val.unstakingAt = s.height + 5
			s.validators[key] = val
			events = s.addEvents(events, buildEvent("auto-begin-unstaking", &lib.Event_AutoBeginUnstaking{AutoBeginUnstaking: &lib.EventAutoBeginUnstaking{}}, s.height, s.chainID, m.Address, txHashStr))
		}
	case *fsm.MessagePause:
		sender = m.Address
		key := hex.EncodeToString(m.Address)
		if val, ok := s.validators[key]; ok {
			val.paused = true
			s.validators[key] = val
		}
		events = s.addEvents(events, buildEvent("auto-pause", &lib.Event_AutoPause{AutoPause: &lib.EventAutoPause{}}, s.height, s.chainID, m.Address, txHashStr))
	case *fsm.MessageUnpause:
		sender = m.Address
		key := hex.EncodeToString(m.Address)
		if val, ok := s.validators[key]; ok {
			val.paused = false
		}
	case *fsm.MessageChangeParameter:
		sender = m.Signer
		switch m.ParameterSpace {
		case "fee":
			s.params.Fee.SendFee = 20
		case "val":
			s.params.Validator.MinimumStakeForValidators = 50_000
		}
	case *fsm.MessageDAOTransfer:
		sender = m.Address
		destKey := hex.EncodeToString(m.Address)
		s.accounts[destKey] += m.Amount
		ev := buildEvent("reward", &lib.Event_Reward{Reward: &lib.EventReward{Amount: m.Amount}}, s.height, s.chainID, m.Address, txHashStr)
		events = s.addEvents(events, ev)
	case *fsm.MessageSubsidy:
		sender = m.Address
		if pool, ok := s.pools[m.ChainId+fsm.HoldingPoolAddend]; ok {
			pool.Amount += m.Amount
		}
	case *fsm.MessageCreateOrder:
		sender = m.SellersSendAddress
		ob := s.orderBooks[m.ChainId]
		ob = append(ob, &lib.SellOrder{
			Id:                   hashBytes("order", s.height, len(ob)),
			Committee:            m.ChainId,
			Data:                 m.Data,
			AmountForSale:        m.AmountForSale,
			RequestedAmount:      m.RequestedAmount,
			SellerReceiveAddress: m.SellerReceiveAddress,
			BuyerSendAddress:     m.SellersSendAddress,
			BuyerReceiveAddress:  m.SellersSendAddress,
			BuyerChainDeadline:   s.height + 100,
			SellersSendAddress:   m.SellersSendAddress,
		})
		s.orderBooks[m.ChainId] = ob
		ev := buildEvent("order-book-swap", &lib.Event_OrderBookSwap{OrderBookSwap: &lib.EventOrderBookSwap{
			SoldAmount:           m.AmountForSale,
			BoughtAmount:         m.RequestedAmount,
			SellerReceiveAddress: m.SellerReceiveAddress,
			BuyerSendAddress:     m.SellersSendAddress,
			SellersSendAddress:   m.SellersSendAddress,
			OrderId:              hashBytes("order", s.height, len(ob)),
			Data:                 m.Data,
		}}, s.height, s.chainID, m.SellersSendAddress, txHashStr)
		events = s.addEvents(events, ev)
	case *fsm.MessageEditOrder:
		ob := s.orderBooks[m.ChainId]
		for _, o := range ob {
			if hex.EncodeToString(o.Id) == hex.EncodeToString(m.OrderId) {
				o.AmountForSale = m.AmountForSale
				o.RequestedAmount = m.RequestedAmount
			}
		}
	case *fsm.MessageDeleteOrder:
		ob := s.orderBooks[m.ChainId]
		filter := ob[:0]
		for _, o := range ob {
			if hex.EncodeToString(o.Id) != hex.EncodeToString(m.OrderId) {
				filter = append(filter, o)
			}
		}
		s.orderBooks[m.ChainId] = filter
		ev := buildEvent("order-book-swap", &lib.Event_OrderBookSwap{OrderBookSwap: &lib.EventOrderBookSwap{
			SoldAmount: 50, BoughtAmount: 100, SellersSendAddress: sender, OrderId: m.OrderId, SellerReceiveAddress: sender, BuyerSendAddress: sender, Data: []byte("delete"),
		}}, s.height, s.chainID, sender, txHashStr)
		events = s.addEvents(events, ev)
	case *fsm.MessageDexLimitOrder:
		sender = m.Address
		m.OrderId = orderID
	case *fsm.MessageDexLiquidityDeposit:
		sender = m.Address
		m.OrderId = orderID
	case *fsm.MessageDexLiquidityWithdraw:
		sender = m.Address
		m.OrderId = orderID
	case *fsm.MessageCertificateResults:
		sender = m.Qc.ProposerKey
	}

	tx = &lib.Transaction{
		MessageType:   txType,
		Msg:           mustAny(msg),
		CreatedHeight: s.height,
		Time:          uint64(blockTimeMicro(s.height) + 150_000),
		Fee:           10,
		Memo:          fmt.Sprintf("%s-%d", txType, s.height),
		NetworkId:     mockNetworkID,
		ChainId:       s.chainID,
	}
	result = &lib.TxResult{
		Sender:      sender,
		Recipient:   nil,
		MessageType: txType,
		Height:      s.height,
		Index:       0,
		Transaction: tx,
		TxHash:      txHashStr,
	}
	return tx, result, events
}

func orderIDFromTxHash(txHash []byte) []byte {
	if len(txHash) < 20 {
		return txHash
	}
	orderID := make([]byte, 20)
	copy(orderID, txHash[:20])
	return orderID
}

func (s *mockState) snapshot() *fsm.GenesisState {
	accounts := make([]*fsm.Account, 0, len(s.accounts))
	for k, v := range s.accounts {
		addr, _ := hex.DecodeString(k)
		accounts = append(accounts, &fsm.Account{Address: addr, Amount: v})
	}
	sort.Slice(accounts, func(i, j int) bool {
		return hex.EncodeToString(accounts[i].Address) < hex.EncodeToString(accounts[j].Address)
	})

	validators := make([]*fsm.Validator, 0, len(s.validators))
	for _, v := range s.validators {
		validators = append(validators, &fsm.Validator{
			Address:         v.addr,
			PublicKey:       v.pubKey,
			NetAddress:      v.netAddress,
			StakedAmount:    v.stake,
			Committees:      v.committees,
			MaxPausedHeight: 0,
			UnstakingHeight: v.unstakingAt,
			Output:          v.outputAddress,
			Delegate:        v.delegate,
			Compound:        v.compound,
		})
	}
	pools := make([]*fsm.Pool, 0, len(s.pools))
	for _, p := range s.pools {
		clone := *p
		if clone.Id != s.chainID+fsm.LiquidityPoolAddend {
			clone.Points = nil
			clone.TotalPoolPoints = 0
		}
		pools = append(pools, &clone)
	}
	orderBooks := &lib.OrderBooks{}
	keys := make([]int, 0, len(s.orderBooks))
	for chain := range s.orderBooks {
		keys = append(keys, int(chain))
	}
	sort.Ints(keys)
	for _, chain := range keys {
		orders := s.orderBooks[uint64(chain)]
		orderBooks.OrderBooks = append(orderBooks.OrderBooks, &lib.OrderBook{ChainId: uint64(chain), Orders: orders})
	}
	supply := s.computeSupply(validators, pools)
	sort.Slice(validators, func(i, j int) bool {
		return hex.EncodeToString(validators[i].Address) < hex.EncodeToString(validators[j].Address)
	})
	sort.Slice(pools, func(i, j int) bool { return pools[i].Id < pools[j].Id })
	return &fsm.GenesisState{
		Time:              uint64(blockTimeMicro(s.height)),
		Pools:             pools,
		Accounts:          accounts,
		Validators:        validators,
		NonSigners:        s.nonSigners,
		DoubleSigners:     s.doubleSign,
		OrderBooks:        orderBooks,
		Params:            s.params,
		Supply:            supply,
		Committees:        s.committeeSet,
		RetiredCommittees: []uint64{42},
	}
}

func (s *mockState) computeSupply(validators []*fsm.Validator, pools []*fsm.Pool) *fsm.Supply {
	total := uint64(0)
	for _, a := range s.accounts {
		total += a
	}
	staked := uint64(0)
	for _, v := range validators {
		staked += v.StakedAmount
	}
	for _, p := range pools {
		total += p.Amount
	}
	return &fsm.Supply{
		Total:                  total + staked,
		Staked:                 staked,
		DelegatedOnly:          staked / 4,
		CommitteeStaked:        pools,
		CommitteeDelegatedOnly: pools,
	}
}

func (s *mockState) validatorsOrdered() []*validatorInfo {
	vals := make([]*validatorInfo, 0, len(s.validators))
	for _, v := range s.validators {
		vals = append(vals, v)
	}
	sort.Slice(vals, func(i, j int) bool {
		return hex.EncodeToString(vals[i].addr) < hex.EncodeToString(vals[j].addr)
	})
	return vals
}

func (s *mockState) addEvents(events lib.Events, newEvents lib.Events) lib.Events {
	s.events = append(s.events, newEvents...)
	return append(events, newEvents...)
}

func (s *mockState) applyBatchOrders(orders []*lib.DexLimitOrder) {
	for _, o := range orders {
		holdingID := s.chainID + fsm.HoldingPoolAddend
		if pool, ok := s.pools[holdingID]; ok {
			pool.Amount += o.AmountForSale
		}
	}
}

func (s *mockState) applyBatchDeposits(deposits []*lib.DexLiquidityDeposit) {
	for _, d := range deposits {
		liqID := s.chainID + fsm.LiquidityPoolAddend
		if pool, ok := s.pools[liqID]; ok {
			pool.Amount += d.Amount
			share := d.Amount / 10
			pool.TotalPoolPoints += share
			pool.Points = append(pool.Points, &lib.PoolPoints{Address: d.Address, Points: share})
		}
		holdingID := s.chainID + fsm.HoldingPoolAddend
		if hold, ok := s.pools[holdingID]; ok && hold.Amount >= d.Amount {
			hold.Amount -= d.Amount
		}
	}
}

func (s *mockState) applyBatchWithdrawals(withdrawals []*lib.DexLiquidityWithdraw) {
	for _, w := range withdrawals {
		liqID := s.chainID + fsm.LiquidityPoolAddend
		if pool, ok := s.pools[liqID]; ok {
			withdraw := (pool.Amount * w.Percent) / 100
			if pool.Amount > withdraw {
				pool.Amount -= withdraw
			}
			burn := (pool.TotalPoolPoints * w.Percent) / 100
			if pool.TotalPoolPoints > burn {
				pool.TotalPoolPoints -= burn
			}
		}
	}
}

func mustAny(msg any) *anypb.Any {
	anyMsg, _ := anypb.New(msg.(proto.Message))
	return anyMsg
}

func buildEvent(typ string, msg interface{}, height uint64, chainID uint64, addr []byte, reference string) lib.Events {
	ev := &lib.Event{
		EventType:   typ,
		Height:      height,
		Reference:   reference,
		ChainId:     chainID,
		BlockHeight: height,
		BlockHash:   blockHash(height),
		Address:     addr,
	}
	switch m := msg.(type) {
	case *lib.Event_Reward:
		ev.Msg = m
	case *lib.Event_Slash:
		ev.Msg = m
	case *lib.Event_DexLiquidityDeposit:
		ev.Msg = m
	case *lib.Event_DexLiquidityWithdrawal:
		ev.Msg = m
	case *lib.Event_DexSwap:
		ev.Msg = m
	case *lib.Event_OrderBookSwap:
		ev.Msg = m
	case *lib.Event_AutoPause:
		ev.Msg = m
	case *lib.Event_AutoBeginUnstaking:
		ev.Msg = m
	case *lib.Event_FinishUnstaking:
		ev.Msg = m
	}
	return lib.Events{ev}
}
