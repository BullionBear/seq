package ob

import "sort"

// OrderedPriceMap maintains prices in sorted order using map + sorted keys.
// It provides O(1) lookup/update and O(n) ordered iteration.
type OrderedPriceMap struct {
	data       map[int64]float64 // priceTick -> quantity
	sortedKeys []int64           // sorted price ticks
	descending bool              // true for bids (high to low), false for asks (low to high)
	dirty      bool              // true if sortedKeys needs re-sorting
}

// NewOrderedPriceMap creates a new OrderedPriceMap.
// descending=true for bids (best bid = highest price first)
// descending=false for asks (best ask = lowest price first)
func NewOrderedPriceMap(descending bool) *OrderedPriceMap {
	return &OrderedPriceMap{
		data:       make(map[int64]float64),
		sortedKeys: make([]int64, 0, 100),
		descending: descending,
		dirty:      false,
	}
}

// Set adds or updates a price level.
// If quantity is 0, the level is removed.
func (m *OrderedPriceMap) Set(priceTick int64, quantity float64) {
	if quantity == 0 {
		m.Remove(priceTick)
		return
	}

	_, exists := m.data[priceTick]
	m.data[priceTick] = quantity

	if !exists {
		// New key - insert in sorted position
		m.insertKey(priceTick)
	}
}

// Remove removes a price level.
func (m *OrderedPriceMap) Remove(priceTick int64) {
	if _, exists := m.data[priceTick]; !exists {
		return
	}

	delete(m.data, priceTick)
	m.removeKey(priceTick)
}

// Get returns the quantity at a price level.
// Returns 0 and false if the level doesn't exist.
func (m *OrderedPriceMap) Get(priceTick int64) (float64, bool) {
	qty, ok := m.data[priceTick]
	return qty, ok
}

// GetBest returns the best price level (first in sorted order).
// Returns (0, 0, false) if empty.
func (m *OrderedPriceMap) GetBest() (priceTick int64, quantity float64, ok bool) {
	if len(m.sortedKeys) == 0 {
		return 0, 0, false
	}
	priceTick = m.sortedKeys[0]
	quantity = m.data[priceTick]
	return priceTick, quantity, true
}

// GetTopN returns the top N price levels in sorted order.
// The returned slice is allocated and safe to keep.
func (m *OrderedPriceMap) GetTopN(n int) []PriceLevel {
	count := n
	if count > len(m.sortedKeys) {
		count = len(m.sortedKeys)
	}

	result := make([]PriceLevel, count)
	for i := 0; i < count; i++ {
		priceTick := m.sortedKeys[i]
		result[i] = PriceLevel{
			PriceTick: priceTick,
			Quantity:  m.data[priceTick],
		}
	}
	return result
}

// GetTopNInto writes the top N price levels into the provided slice.
// Returns the number of levels written.
func (m *OrderedPriceMap) GetTopNInto(levels []PriceLevel) int {
	count := len(levels)
	if count > len(m.sortedKeys) {
		count = len(m.sortedKeys)
	}

	for i := 0; i < count; i++ {
		priceTick := m.sortedKeys[i]
		levels[i].PriceTick = priceTick
		levels[i].Quantity = m.data[priceTick]
	}
	return count
}

// Len returns the number of price levels.
func (m *OrderedPriceMap) Len() int {
	return len(m.sortedKeys)
}

// Clear removes all price levels.
func (m *OrderedPriceMap) Clear() {
	m.data = make(map[int64]float64)
	m.sortedKeys = m.sortedKeys[:0]
	m.dirty = false
}

// Iterate calls the given function for each price level in sorted order.
// The function should return true to continue iteration, false to stop.
func (m *OrderedPriceMap) Iterate(fn func(priceTick int64, quantity float64) bool) {
	for _, priceTick := range m.sortedKeys {
		if !fn(priceTick, m.data[priceTick]) {
			break
		}
	}
}

// insertKey inserts a key in sorted position.
func (m *OrderedPriceMap) insertKey(priceTick int64) {
	// Find insertion position using binary search
	idx := m.searchPosition(priceTick)

	// Insert at position
	m.sortedKeys = append(m.sortedKeys, 0)
	copy(m.sortedKeys[idx+1:], m.sortedKeys[idx:])
	m.sortedKeys[idx] = priceTick
}

// removeKey removes a key from the sorted slice.
func (m *OrderedPriceMap) removeKey(priceTick int64) {
	idx := m.searchPosition(priceTick)

	// Verify the key is at this position
	if idx < len(m.sortedKeys) && m.sortedKeys[idx] == priceTick {
		m.sortedKeys = append(m.sortedKeys[:idx], m.sortedKeys[idx+1:]...)
	}
}

// searchPosition finds the position where priceTick should be inserted.
func (m *OrderedPriceMap) searchPosition(priceTick int64) int {
	if m.descending {
		// For descending order (bids), higher prices come first
		return sort.Search(len(m.sortedKeys), func(i int) bool {
			return m.sortedKeys[i] <= priceTick
		})
	}
	// For ascending order (asks), lower prices come first
	return sort.Search(len(m.sortedKeys), func(i int) bool {
		return m.sortedKeys[i] >= priceTick
	})
}

// SetFromSlice sets all price levels from a slice, clearing existing data.
func (m *OrderedPriceMap) SetFromSlice(levels []PriceLevel) {
	m.Clear()

	for _, level := range levels {
		if level.Quantity > 0 {
			m.data[level.PriceTick] = level.Quantity
			m.sortedKeys = append(m.sortedKeys, level.PriceTick)
		}
	}

	// Sort all keys at once (more efficient than individual inserts)
	if m.descending {
		sort.Slice(m.sortedKeys, func(i, j int) bool {
			return m.sortedKeys[i] > m.sortedKeys[j]
		})
	} else {
		sort.Slice(m.sortedKeys, func(i, j int) bool {
			return m.sortedKeys[i] < m.sortedKeys[j]
		})
	}
}
