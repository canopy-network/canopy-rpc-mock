package rpc

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strconv"

	"github.com/canopy-network/canopy-rpc-mock/mockserver/internal/chain"
	"github.com/canopy-network/canopy/fsm"
	"github.com/canopy-network/canopy/lib"
	"google.golang.org/protobuf/proto"
)

// simplePageResp matches the format expected by pgindexer's listPaged function
type simplePageResp struct {
	PageNumber int `json:"pageNumber"`
	PerPage    int `json:"perPage"`
	Results    any `json:"results"`
	Count      int `json:"count"`
	TotalPages int `json:"totalPages"`
	TotalCount int `json:"totalCount"`
}

const (
	headPath                 = "/v1/query/height"
	blockByHeightPath        = "/v1/query/block-by-height"
	certByHeightPath         = "/v1/query/cert-by-height"
	statePath                = "/v1/query/state"
	txsByHeightPath          = "/v1/query/txs-by-height"
	eventsByHeightPath       = "/v1/query/events-by-height"
	accountsByHeightPath     = "/v1/query/accounts"
	ordersByHeightPath       = "/v1/query/orders"
	poolsPath                = "/v1/query/pools"
	dexPricePath             = "/v1/query/dex-price"
	dexBatchByHeightPath     = "/v1/query/dex-batch"
	nextDexBatchByHeightPath = "/v1/query/next-dex-batch"
	allParamsPath            = "/v1/query/params"
	feeParamsPath            = "/v1/query/fee-params"
	conParamsPath            = "/v1/query/con-params"
	valParamsPath            = "/v1/query/val-params"
	govParamsPath            = "/v1/query/gov-params"
	supplyPath               = "/v1/query/supply"
	validatorsPath           = "/v1/query/validators"
	nonSignersPath           = "/v1/query/non-signers"
	doubleSignersPath        = "/v1/query/double-signers"
	committeesDataPath       = "/v1/query/committees-data"
	subsidizedCommitteesPath = "/v1/query/subsidized-committees"
	retiredCommitteesPath    = "/v1/query/retired-committees"
	pollPath                 = "/v1/gov/poll"
	proposalsPath            = "/v1/gov/proposals"
)

type heightPageRequest struct {
	Height     uint64 `json:"height"`
	PageNumber int    `json:"pageNumber"`
	PerPage    int    `json:"perPage"`
}

type idHeightRequest struct {
	Height uint64 `json:"height"`
	ID     uint64 `json:"id"`
}

type dexRequest struct {
	Height uint64 `json:"height"`
	ID     uint64 `json:"id"`
	Points bool   `json:"points"`
}

func normalizeOrderBooks(src *lib.OrderBooks) *lib.OrderBooks {
	if src == nil {
		return nil
	}
	dst := &lib.OrderBooks{OrderBooks: make([]*lib.OrderBook, 0, len(src.OrderBooks))}
	for _, book := range src.OrderBooks {
		if book == nil {
			continue
		}
		orders := make([]*lib.SellOrder, 0, len(book.Orders))
		for _, order := range book.Orders {
			if order == nil {
				continue
			}
			clone := proto.Clone(order).(*lib.SellOrder)
			clone.Committee = book.ChainId
			orders = append(orders, clone)
		}
		dst.OrderBooks = append(dst.OrderBooks, &lib.OrderBook{ChainId: book.ChainId, Orders: orders})
	}
	return dst
}

// RegisterRoutes wires every mock RPC endpoint onto mux against mc. It's
// exported because mockserver.New (the root package) is the only caller
// outside this package.
func RegisterRoutes(mux *http.ServeMux, mc *chain.MockChain) {
	mux.HandleFunc(headPath, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, &lib.HeightResult{Height: mc.LatestHeight()})
	})

	mux.HandleFunc(blockByHeightPath, func(w http.ResponseWriter, r *http.Request) {
		req := parseHeightReq(r)
		if req == 0 {
			req = mc.LatestHeight()
		}
		block, ok := mc.BlockAt(req)
		if !ok {
			http.Error(w, "unknown height", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, block)
	})

	mux.HandleFunc(certByHeightPath, func(w http.ResponseWriter, r *http.Request) {
		req := parseHeightReq(r)
		if req == 0 {
			req = mc.LatestHeight()
		}
		cert, ok := mc.CertAt(req)
		if !ok {
			http.Error(w, "unknown height", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, cert)
	})

	mux.HandleFunc(statePath, func(w http.ResponseWriter, r *http.Request) {
		h := parseHeightReq(r)
		if h == 0 && r.URL.Query().Get("height") == "" && r.ContentLength == 0 {
			h = mc.LatestHeight()
		}
		state := mc.StateAt(h)
		if state == nil {
			http.Error(w, "unknown height", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, state)
	})

	mux.HandleFunc(txsByHeightPath, func(w http.ResponseWriter, r *http.Request) {
		req := heightPageRequest{}
		decodeBody(r, &req)
		if req.Height == 0 {
			req.Height = mc.LatestHeight()
		}
		txs := mc.TxsAt(req.Height)
		pageItems, params, total := paginate(txs, req.PageNumber, req.PerPage)
		results := lib.TxResults(pageItems)
		page := buildPage(&results, params, lib.TxResultsPageName, total)
		writeJSON(w, http.StatusOK, page)
	})

	mux.HandleFunc(eventsByHeightPath, func(w http.ResponseWriter, r *http.Request) {
		req := heightPageRequest{}
		decodeBody(r, &req)
		if req.Height == 0 {
			req.Height = mc.LatestHeight()
		}
		events := mc.EventsAt(req.Height)
		pageItems, params, total := paginate(events, req.PageNumber, req.PerPage)
		results := lib.Events(pageItems)
		page := buildPage(&results, params, lib.EventsPageName, total)
		writeJSON(w, http.StatusOK, page)
	})

	mux.HandleFunc(accountsByHeightPath, func(w http.ResponseWriter, r *http.Request) {
		req := heightPageRequest{}
		decodeBody(r, &req)
		if req.Height == 0 {
			req.Height = mc.LatestHeight()
		}
		state := mc.StateAt(req.Height)
		if state == nil {
			http.Error(w, "unknown height", http.StatusNotFound)
			return
		}
		accounts := state.Accounts
		pageItems, params, total := paginate(accounts, req.PageNumber, req.PerPage)
		results := fsm.AccountPage(pageItems)
		page := buildPage(&results, params, fsm.AccountsPageName, total)
		writeJSON(w, http.StatusOK, page)
	})

	mux.HandleFunc(ordersByHeightPath, func(w http.ResponseWriter, r *http.Request) {
		req := idHeightRequest{}
		decodeBody(r, &req)
		if req.Height == 0 {
			req.Height = mc.LatestHeight()
		}
		state := mc.StateAt(req.Height)
		if state == nil {
			http.Error(w, "unknown height", http.StatusNotFound)
			return
		}
		target := normalizeOrderBooks(state.OrderBooks)
		if req.ID == 0 {
			writeJSON(w, http.StatusOK, target)
			return
		}
		for _, book := range target.OrderBooks {
			if book.ChainId == req.ID {
				writeJSON(w, http.StatusOK, &lib.OrderBooks{OrderBooks: []*lib.OrderBook{book}})
				return
			}
		}
		http.Error(w, "order book not found", http.StatusNotFound)
	})

	mux.HandleFunc(poolsPath, func(w http.ResponseWriter, r *http.Request) {
		req := heightPageRequest{}
		decodeBody(r, &req)
		if req.Height == 0 {
			req.Height = mc.LatestHeight()
		}
		state := mc.StateAt(req.Height)
		if state == nil {
			http.Error(w, "unknown height", http.StatusNotFound)
			return
		}
		pools := state.Pools
		pageItems, params, total := paginate(pools, req.PageNumber, req.PerPage)
		results := fsm.PoolPage(pageItems)
		page := buildPage(&results, params, fsm.PoolPageName, total)
		writeJSON(w, http.StatusOK, page)
	})

	mux.HandleFunc(dexPricePath, func(w http.ResponseWriter, r *http.Request) {
		req := idHeightRequest{}
		decodeBody(r, &req)
		prices := mc.DexPricesAt(req.Height)
		if prices == nil {
			http.Error(w, "unknown height", http.StatusNotFound)
			return
		}
		if req.ID == 0 {
			writeJSON(w, http.StatusOK, prices)
			return
		}
		filtered := []*lib.DexPrice{}
		for i := range prices {
			if prices[i].LocalChainId == req.ID {
				filtered = append(filtered, &prices[i])
			}
		}
		writeJSON(w, http.StatusOK, filtered)
	})

	mux.HandleFunc(dexBatchByHeightPath, func(w http.ResponseWriter, r *http.Request) {
		req := dexRequest{}
		decodeBody(r, &req)
		if req.Height == 0 {
			req.Height = mc.LatestHeight()
		}
		if req.ID != 0 && req.ID != mc.ChainID() {
			http.Error(w, "unknown committee", http.StatusNotFound)
			return
		}
		if batch, ok := mc.DexBatchAt(req.Height); ok {
			// Return as array to match real API format
			writeJSON(w, http.StatusOK, []*lib.DexBatch{batch})
			return
		}
		http.Error(w, "unknown height", http.StatusNotFound)
	})

	mux.HandleFunc(nextDexBatchByHeightPath, func(w http.ResponseWriter, r *http.Request) {
		req := dexRequest{}
		decodeBody(r, &req)
		if req.Height == 0 {
			req.Height = mc.LatestHeight()
		}
		if req.ID != 0 && req.ID != mc.ChainID() {
			http.Error(w, "unknown committee", http.StatusNotFound)
			return
		}
		if batch, ok := mc.NextDexBatchAt(req.Height); ok {
			// Return as array to match real API format
			writeJSON(w, http.StatusOK, []*lib.DexBatch{batch})
			return
		}
		http.Error(w, "unknown height", http.StatusNotFound)
	})

	mux.HandleFunc(allParamsPath, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, mc.Params())
	})
	mux.HandleFunc(feeParamsPath, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, mc.Params().Fee)
	})
	mux.HandleFunc(conParamsPath, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, mc.Params().Consensus)
	})
	mux.HandleFunc(valParamsPath, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, mc.Params().Validator)
	})
	mux.HandleFunc(govParamsPath, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, mc.Params().Governance)
	})

	mux.HandleFunc(supplyPath, func(w http.ResponseWriter, r *http.Request) {
		h := parseHeightReq(r)
		if h == 0 {
			h = mc.LatestHeight()
		}
		supply := mc.SupplyAt(h)
		if supply == nil {
			http.Error(w, "unknown height", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, supply)
	})

	mux.HandleFunc(validatorsPath, func(w http.ResponseWriter, r *http.Request) {
		req := heightPageRequest{}
		decodeBody(r, &req)
		if req.Height == 0 {
			req.Height = mc.LatestHeight()
		}
		state := mc.StateAt(req.Height)
		if state == nil {
			http.Error(w, "unknown height", http.StatusNotFound)
			return
		}
		validators := state.Validators
		pageItems, params, total := paginate(validators, req.PageNumber, req.PerPage)
		results := fsm.ValidatorPage(pageItems)
		page := buildPage(&results, params, fsm.ValidatorsPageName, total)
		writeJSON(w, http.StatusOK, page)
	})

	mux.HandleFunc(nonSignersPath, func(w http.ResponseWriter, r *http.Request) {
		h := parseHeightReq(r)
		if h == 0 {
			h = mc.LatestHeight()
		}
		results := mc.NonSigners(h)
		writeJSON(w, http.StatusOK, simplePageResp{
			PageNumber: 1,
			PerPage:    100,
			Results:    results,
			Count:      len(results),
			TotalPages: 1,
			TotalCount: len(results),
		})
	})

	mux.HandleFunc(doubleSignersPath, func(w http.ResponseWriter, r *http.Request) {
		h := parseHeightReq(r)
		if h == 0 {
			h = mc.LatestHeight()
		}
		results := mc.DoubleSigners(h)
		writeJSON(w, http.StatusOK, simplePageResp{
			PageNumber: 1,
			PerPage:    100,
			Results:    results,
			Count:      len(results),
			TotalPages: 1,
			TotalCount: len(results),
		})
	})

	mux.HandleFunc(committeesDataPath, func(w http.ResponseWriter, _ *http.Request) {
		state := mc.StateAt(mc.LatestHeight())
		if state != nil && state.Committees != nil {
			writeJSON(w, http.StatusOK, state.Committees)
			return
		}
		writeJSON(w, http.StatusOK, mc.Committees())
	})

	mux.HandleFunc(subsidizedCommitteesPath, func(w http.ResponseWriter, _ *http.Request) {
		state := mc.StateAt(mc.LatestHeight())
		if state != nil && state.Committees != nil {
			writeJSON(w, http.StatusOK, mc.SubsidizedCommittees())
			return
		}
		writeJSON(w, http.StatusOK, mc.SubsidizedCommittees())
	})

	mux.HandleFunc(retiredCommitteesPath, func(w http.ResponseWriter, _ *http.Request) {
		state := mc.StateAt(mc.LatestHeight())
		if state != nil && len(state.RetiredCommittees) > 0 {
			writeJSON(w, http.StatusOK, state.RetiredCommittees)
			return
		}
		writeJSON(w, http.StatusOK, mc.RetiredCommittees())
	})

	mux.HandleFunc(pollPath, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, mc.Poll())
	})

	mux.HandleFunc(proposalsPath, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, mc.Proposals())
	})
}

func parseHeightReq(r *http.Request) uint64 {
	if qs := r.URL.Query().Get("height"); qs != "" {
		if h, err := strconv.ParseUint(qs, 10, 64); err == nil {
			return h
		}
	}
	var req struct {
		Height uint64 `json:"height"`
	}
	decodeBody(r, &req)
	return req.Height
}

func decodeBody(r *http.Request, target any) {
	if r.Body == nil {
		return
	}
	defer r.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil || len(bytes.TrimSpace(raw)) == 0 {
		return
	}
	_ = json.Unmarshal(raw, target)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}

func paginate[T any](items []T, pageNumber, perPage int) ([]T, lib.PageParams, int) {
	params := lib.PageParams{PageNumber: pageNumber, PerPage: perPage}
	if params.PerPage == 0 {
		params.PerPage = 10
	}
	if params.PerPage > 5000 {
		params.PerPage = 5000
	}
	if params.PageNumber <= 0 {
		params.PageNumber = 1
	}
	total := len(items)
	start := (params.PageNumber - 1) * params.PerPage
	if start > total {
		start = total
	}
	end := start + params.PerPage
	if end > total {
		end = total
	}
	return items[start:end], params, total
}

func buildPage(results lib.Pageable, params lib.PageParams, typeName string, total int) *lib.Page {
	count := 0
	if c, ok := results.(interface{ Len() int }); ok {
		count = c.Len()
	}
	totalPages := 0
	if params.PerPage > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(params.PerPage)))
	}
	return &lib.Page{
		PageParams: params,
		Results:    results,
		Type:       typeName,
		Count:      count,
		TotalPages: totalPages,
		TotalCount: total,
	}
}
