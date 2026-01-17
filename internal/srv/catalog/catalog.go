package pms

import (
	"fmt"
)

const (
	QueryActiveInstruments = `
		SELECT * FROM instruments WHERE active = true
	`
)

type InstrumentCatalog struct {
	instruments map[int]Instrument
}

func NewInstrumentCatalog() (*InstrumentCatalog, error) {

	return &InstrumentCatalog{
		instruments: make(map[int]Instrument, 1024),
	}, nil
}

func (c *InstrumentCatalog) GetInstrument(symbolID int) (Instrument, error) {
	instrument, ok := c.instruments[symbolID]
	if !ok {
		return Instrument{}, fmt.Errorf("instrument not found for symbolID: %d", symbolID)
	}
	return instrument, nil
}
