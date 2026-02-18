package orderbook

import (
	"math"

	coreactor "github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/mem"
	"github.com/BullionBear/seq/core/model/command"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/BullionBear/seq/data"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog"
	"github.com/tidwall/btree"
)

func init() {
	data.Register("orderbook", func(cat *catalog.Catalog, _ *msgbus.MsgBus, c *cache.Cache) coreactor.Actor {
		return NewActor(cat, c)
	})
}

const (
	// DefaultDepthUpdateBufferSize is the default capacity for depth update ring buffers
	DefaultDepthUpdateBufferSize = 100
)

// Ensure Actor implements the coreactor.Actor interface
var _ coreactor.Actor = (*Actor)(nil)

// ============================================================================
// SymbolBook -- per-symbol orderbook state
// ============================================================================

// SymbolBook maintains the orderbook state for a single symbol.
// Uses btree.Map keyed by PriceTick for both bids and asks.
type SymbolBook struct {
	symbolID       int
	pricePrecision int
	sizePrecision  int
	tickMultiplier float64 // = 10^pricePrecision, cached

	// Logger inherited from the parent Actor
	log *zerolog.Logger

	// State machine
	state   common.BookState
	depthID int

	// Order book data (btree, key = PriceTick)
	bids btree.Map[int, common.PriceLevel]
	asks btree.Map[int, common.PriceLevel]

	// Update buffer (for WaitForSnapshot/Updating states)
	updateBuffer *mem.SPSCRingBuffer[DepthUpdateBuffer]

	// Snapshot request tracking
	snapshotPending bool

	lastUpdated uint64
}

// newSymbolBook creates a new SymbolBook.
func newSymbolBook(symbolID, pricePrecision, sizePrecision int, log *zerolog.Logger) *SymbolBook {
	return &SymbolBook{
		symbolID:       symbolID,
		pricePrecision: pricePrecision,
		sizePrecision:  sizePrecision,
		tickMultiplier: math.Pow(10, float64(pricePrecision)),
		log:            log,
		state:          common.BookStateWaitForSnapshot,
		updateBuffer:   mem.NewSPSCRingBuffer[DepthUpdateBuffer](DefaultDepthUpdateBufferSize),
	}
}

// tickToPrice converts integer tick back to float price.
func (sb *SymbolBook) tickToPrice(tick int) float64 {
	return float64(tick) / sb.tickMultiplier
}

// reset resets the orderbook to WaitForSnapshot state.
func (sb *SymbolBook) reset() {
	sb.state = common.BookStateWaitForSnapshot
	sb.depthID = 0
	sb.bids = btree.Map[int, common.PriceLevel]{}
	sb.asks = btree.Map[int, common.PriceLevel]{}
	sb.updateBuffer.Reset()
	sb.snapshotPending = false
	sb.lastUpdated = 0
}

// loadSnapshot loads a full snapshot (clears and rebuilds).
func (sb *SymbolBook) loadSnapshot(depthID int, timestamp uint64, bids, asks []common.PriceLevel) {
	sb.bids = btree.Map[int, common.PriceLevel]{}
	sb.asks = btree.Map[int, common.PriceLevel]{}
	for _, pl := range bids {
		if pl.QuantityTick != 0 {
			sb.bids.Set(pl.PriceTick, pl)
		}
	}
	for _, pl := range asks {
		if pl.QuantityTick != 0 {
			sb.asks.Set(pl.PriceTick, pl)
		}
	}
	sb.depthID = depthID
	sb.lastUpdated = timestamp
}

// applyLevels applies incremental level updates.
// QuantityTick==0 means the level should be deleted.
func (sb *SymbolBook) applyLevels(bids, asks []common.PriceLevel) {
	for _, pl := range bids {
		if pl.QuantityTick != 0 {
			sb.bids.Set(pl.PriceTick, pl)
		} else {
			sb.bids.Delete(pl.PriceTick)
		}
	}
	for _, pl := range asks {
		if pl.QuantityTick != 0 {
			sb.asks.Set(pl.PriceTick, pl)
		} else {
			sb.asks.Delete(pl.PriceTick)
		}
	}
}

// collectBids collects all bids as a slice (descending by PriceTick for Cache).
func (sb *SymbolBook) collectBids() []common.PriceLevel {
	out := make([]common.PriceLevel, 0, sb.bids.Len())
	sb.bids.Reverse(func(key int, value common.PriceLevel) bool {
		out = append(out, value)
		return true
	})
	return out
}

// collectAsks collects all asks as a slice (ascending by PriceTick for Cache).
func (sb *SymbolBook) collectAsks() []common.PriceLevel {
	out := make([]common.PriceLevel, 0, sb.asks.Len())
	sb.asks.Scan(func(key int, value common.PriceLevel) bool {
		out = append(out, value)
		return true
	})
	return out
}

// syncToCache writes the full book state into the cache.
func (sb *SymbolBook) syncToCache(c *cache.Cache) {
	c.UpdateBook(sb.symbolID, sb.collectBids(), sb.collectAsks())
	c.SetBookState(sb.symbolID, sb.state)
}

// applySyncToCache applies incremental updates to cache (no clear+rebuild).
func (sb *SymbolBook) applySyncToCache(c *cache.Cache, bids, asks []common.PriceLevel) {
	c.ApplyBookUpdate(sb.symbolID, bids, asks)
	c.SetBookState(sb.symbolID, sb.state)
}

// requestSnapshot sends a CommandTypeReqDepthSnapshot command via the MsgBus.
func (sb *SymbolBook) requestSnapshot(bus *msgbus.MsgBus) {
	if sb.snapshotPending {
		return
	}
	sb.snapshotPending = true

	req := command.ReqDepthSnapshot{SymbolID: sb.symbolID}
	size := uint64(req.GetBufferLength())
	offset, buf := bus.AllocateCmd(size)
	req.Encode(buf)
	bus.Send(msgbus.CommandRef{
		CommandType: command.CommandTypeReqDepthSnapshot,
		Index:       offset,
		Length:      size,
	})

	sb.log.Debug().Int("symbolID", sb.symbolID).Msg("SymbolBook: requested depth snapshot")
}

// onDepthSnapshot handles a depth snapshot event (from WS).
func (sb *SymbolBook) onDepthSnapshot(snapshot event.DepthSnapshot, c *cache.Cache) {
	sb.log.Info().
		Int("symbolID", sb.symbolID).
		Int("snapshotDepthID", snapshot.DepthID).
		Int("bids", len(snapshot.Bids)).
		Int("asks", len(snapshot.Asks)).
		Str("prevState", sb.state.String()).
		Msg("SymbolBook: Processing DepthSnapshot")

	sb.loadSnapshot(snapshot.DepthID, snapshot.Timestamp, snapshot.Bids, snapshot.Asks)
	sb.snapshotPending = false

	// Transition to Updating to process buffered updates
	sb.state = common.BookStateUpdating
	sb.processBufferedUpdates(c)

	// Full sync after snapshot
	sb.syncToCache(c)
}

// onRespDepthSnapshot handles an HTTP response snapshot.
func (sb *SymbolBook) onRespDepthSnapshot(resp event.RespDepthSnapshot, c *cache.Cache) {
	sb.log.Info().
		Int("symbolID", sb.symbolID).
		Int("snapshotDepthID", resp.DepthID).
		Int("bids", len(resp.Bids)).
		Int("asks", len(resp.Asks)).
		Str("prevState", sb.state.String()).
		Msg("SymbolBook: Processing RespDepthSnapshot")

	sb.loadSnapshot(resp.DepthID, resp.Timestamp, resp.Bids, resp.Asks)
	sb.snapshotPending = false

	// Transition to Updating to process buffered updates
	sb.state = common.BookStateUpdating
	sb.processBufferedUpdates(c)

	// Full sync after snapshot
	sb.syncToCache(c)
}

// onDepthUpdate handles a depth update event.
func (sb *SymbolBook) onDepthUpdate(update event.DepthUpdate, c *cache.Cache, bus *msgbus.MsgBus) {
	switch sb.state {
	case common.BookStateWaitForSnapshot:
		// Buffer the update for later processing
		sb.bufferUpdate(update)
		sb.log.Debug().
			Int("symbolID", sb.symbolID).
			Int("depthID", update.DepthID).
			Uint64("buffered", sb.updateBuffer.Count()).
			Msg("SymbolBook: Buffered update (waiting for snapshot)")

		// Auto-request snapshot on first update
		sb.requestSnapshot(bus)

	case common.BookStateUpdating:
		// Buffer and try to process
		sb.bufferUpdate(update)
		sb.processBufferedUpdates(c)
		// Full sync after processing buffered updates
		sb.syncToCache(c)

	case common.BookStateReady:
		if update.PreviousDepthID <= sb.depthID && update.CurrentDepthID > sb.depthID {
			// Apply the update
			sb.applyLevels(update.Bids, update.Asks)
			sb.depthID = update.CurrentDepthID
			sb.lastUpdated = update.Timestamp
			sb.log.Debug().
				Int("symbolID", sb.symbolID).
				Int("prevDepthID", update.PreviousDepthID).
				Int("bookDepthID", sb.depthID).
				Int("updateFinalDepthID", update.CurrentDepthID).
				Msg("SymbolBook: Applied update")
			// Incremental sync to cache
			sb.applySyncToCache(c, update.Bids, update.Asks)
		} else if update.CurrentDepthID <= sb.depthID {
			// Stale update - ignore
			sb.log.Debug().
				Int("symbolID", sb.symbolID).
				Int("bookDepthID", sb.depthID).
				Int("updateFinalDepthID", update.CurrentDepthID).
				Msg("SymbolBook: Ignoring stale update")
		} else {
			// Gap detected - reset
			gap := update.PreviousDepthID - sb.depthID
			sb.log.Warn().
				Int("symbolID", sb.symbolID).
				Int("bookDepthID", sb.depthID).
				Int("updatePrevDepthID", update.PreviousDepthID).
				Int("updateFinalDepthID", update.CurrentDepthID).
				Int("missedDepthIDs", gap).
				Msg("SymbolBook: Data loss detected, resetting to WaitForSnapshot")
			sb.reset()
			sb.syncToCache(c)
		}
	}
}

// bufferUpdate adds an update to the ring buffer.
func (sb *SymbolBook) bufferUpdate(update event.DepthUpdate) {
	buffered := DepthUpdateBuffer{
		PreviousDepthID: update.PreviousDepthID,
		FirstDepthID:    update.DepthID,
		FinalDepthID:    update.CurrentDepthID,
		Timestamp:       update.Timestamp,
		Bids:            update.Bids,
		Asks:            update.Asks,
	}
	if !sb.updateBuffer.Write(buffered) {
		sb.log.Warn().
			Int("symbolID", sb.symbolID).
			Msg("SymbolBook: Update buffer full, dropping update")
	}
}

// processBufferedUpdates drains applicable buffered updates.
func (sb *SymbolBook) processBufferedUpdates(c *cache.Cache) {
	processed := 0
	for !sb.updateBuffer.IsEmpty() {
		update, ok := sb.updateBuffer.Peek()
		if !ok {
			break
		}

		if update.PreviousDepthID <= sb.depthID && update.FinalDepthID > sb.depthID {
			sb.applyLevels(update.Bids, update.Asks)
			sb.depthID = update.FinalDepthID
			sb.lastUpdated = update.Timestamp
			sb.updateBuffer.Read() // consume
			processed++
		} else if update.FinalDepthID <= sb.depthID {
			sb.updateBuffer.Read() // stale, discard
		} else {
			break // future update, stop
		}
	}

	// Transition to Ready if buffer is drained
	if sb.updateBuffer.IsEmpty() && sb.state == common.BookStateUpdating {
		sb.state = common.BookStateReady
		sb.log.Info().
			Int("symbolID", sb.symbolID).
			Int("depthID", sb.depthID).
			Int("processedUpdates", processed).
			Msg("SymbolBook: Transitioned to Ready state")
	}
}

// ============================================================================
// Actor -- the OrderBook actor
// ============================================================================

// Actor is the orderbook actor that maintains per-symbol order books,
// writes state to Cache, and requests snapshots via MsgBus commands.
type Actor struct {
	coreactor.ActorBase
	catalog *catalog.Catalog
	books   map[int]*SymbolBook // symbolID -> SymbolBook
	cache   *cache.Cache
}

// NewActor creates a new orderbook Actor.
func NewActor(cat *catalog.Catalog, c *cache.Cache) *Actor {
	return &Actor{
		ActorBase: coreactor.NewActorBase("orderbook", []event.Topic{
			event.TopicEventDepthSnapshot,
			event.TopicEventRespDepthSnapshot,
			event.TopicEventDepthUpdate,
		}),
		catalog: cat,
		books:   make(map[int]*SymbolBook),
		cache:   c,
	}
}

// RegisterSymbol registers a symbol with its precision info.
// Creates a new SymbolBook in WaitForSnapshot state.
func (a *Actor) RegisterSymbol(symbolID, pricePrecision, sizePrecision int) {
	if _, exists := a.books[symbolID]; exists {
		a.Log().Warn().Int("symbolID", symbolID).Msg("OrderBook Actor: Symbol already registered")
		return
	}

	a.books[symbolID] = newSymbolBook(symbolID, pricePrecision, sizePrecision, a.Log())
	a.Log().Info().
		Int("symbolID", symbolID).
		Int("pricePrecision", pricePrecision).
		Int("sizePrecision", sizePrecision).
		Msg("OrderBook Actor: Registered symbol")
}

// Handle dispatches events to the appropriate handler.
func (a *Actor) Handle(ev msgbus.Event, bus *msgbus.MsgBus) {

	switch ev.Ref.Topic {
	case event.TopicEventDepthSnapshot:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		snapshot := event.NewDepthSnapshotFromBytes(buf)
		a.onDepthSnapshot(snapshot)

	case event.TopicEventRespDepthSnapshot:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		resp := event.NewRespDepthSnapshotFromBytes(buf)
		a.onRespDepthSnapshot(resp)

	case event.TopicEventDepthUpdate:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		update := event.NewDepthUpdateFromBytes(buf)
		a.onDepthUpdate(update, bus)
	}
}

func (a *Actor) onDepthSnapshot(snapshot event.DepthSnapshot) {
	book, exists := a.books[snapshot.SymbolID]
	if !exists {
		a.Log().Warn().
			Int("symbolID", snapshot.SymbolID).
			Msg("OrderBook Actor: Received snapshot for unregistered symbol")
		return
	}
	book.onDepthSnapshot(snapshot, a.cache)
}

func (a *Actor) onRespDepthSnapshot(resp event.RespDepthSnapshot) {
	book, exists := a.books[resp.SymbolID]
	if !exists {
		a.Log().Warn().
			Int("symbolID", resp.SymbolID).
			Msg("OrderBook Actor: Received RespDepthSnapshot for unregistered symbol")
		return
	}
	book.onRespDepthSnapshot(resp, a.cache)
}

func (a *Actor) onDepthUpdate(update event.DepthUpdate, bus *msgbus.MsgBus) {
	book, exists := a.books[update.SymbolID]
	if !exists {
		a.Log().Debug().
			Int("symbolID", update.SymbolID).
			Msg("OrderBook Actor: Received update for unregistered symbol")
		return
	}
	book.onDepthUpdate(update, a.cache, bus)
}

// OnInit decodes the config and registers the symbol for this orderbook.
func (a *Actor) OnInit(config map[string]any) {
	var cfg OrderbookConfig
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &cfg,
		TagName: "yaml",
	})
	if err != nil {
		a.Log().Error().Err(err).Msg("OrderBook Actor: failed to create decoder")
		return
	}
	if err := decoder.Decode(config); err != nil {
		a.Log().Error().Err(err).Msg("OrderBook Actor: failed to decode config")
		return
	}

	if cfg.Symbol == "" {
		a.Log().Warn().Msg("OrderBook Actor: no symbol configured")
		return
	}

	symbol, err := a.catalog.GetSymbolByUniversalTicker(cfg.Symbol)
	if err != nil {
		a.Log().Error().Err(err).Str("symbol", cfg.Symbol).Msg("OrderBook Actor: failed to resolve symbol")
		return
	}

	a.RegisterSymbol(symbol.ID, symbol.PricePrecision, symbol.SizePrecision)
	a.Log().Info().
		Str("ticker", symbol.UniversalTicker).
		Int("symbolID", symbol.ID).
		Msg("OrderBook Actor: initialized from config")
}

// OnStart is called once when the actor is started.
func (a *Actor) OnStart() {
	a.Log().Info().Msg("OrderBook Actor: started")
}

// OnStop is called once when the actor is stopped.
func (a *Actor) OnStop() {
	for _, book := range a.books {
		book.reset()
		book.syncToCache(a.cache)
	}
	a.Log().Info().Int("symbols", len(a.books)).Msg("OrderBook Actor: stopped, all books reset")
}
