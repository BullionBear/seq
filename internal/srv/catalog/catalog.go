package pms

import (
	"fmt"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

const (
	QueryActiveInstruments = `
		SELECT * FROM instruments WHERE active = true
	`
)

type InstrumentCatalog struct {
	db          *gorm.DB
	instruments map[int]Instrument
}

func NewInstrumentCatalog(db *gorm.DB) (*InstrumentCatalog, error) {
	instrumentCatalog := &InstrumentCatalog{db: db, instruments: make(map[int]Instrument, 1024)}
	if err := instrumentCatalog.loadInstruments(); err != nil {
		log.Error().Err(err).Msg("Failed to load instruments")
		return nil, err
	}
	return instrumentCatalog, nil
}

func (c *InstrumentCatalog) loadInstruments() error {
	rows, err := c.db.Raw(QueryActiveInstruments).Rows()
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var instrument Instrument
		err := rows.Scan(&instrument.SymbolID, &instrument.Symbol, &instrument.Exchange, &instrument.Type, &instrument.Active)
		if err != nil {
			return err
		}
		c.instruments[instrument.SymbolID] = instrument
	}
	return nil
}

func (c *InstrumentCatalog) GetInstrument(symbolID int) (Instrument, error) {
	instrument, ok := c.instruments[symbolID]
	if !ok {
		return Instrument{}, fmt.Errorf("instrument not found for symbolID: %d", symbolID)
	}
	return instrument, nil
}
