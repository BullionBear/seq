CREATE TABLE instruments (
	symbol_id INT PRIMARY KEY,
	exchange VARCHAR(255) NOT NULL,
	type VARCHAR(255) NOT NULL,
	symbol VARCHAR(255) NOT NULL,
	base_ccy VARCHAR(255) NOT NULL,
	base_ccy_id INT NOT NULL,
	quote_ccy VARCHAR(255) NOT NULL,
	quote_ccy_id INT NOT NULL,
	price_tick_size DECIMAL(10, 4) NOT NULL,
	qty_tick_size DECIMAL(10, 4) NOT NULL,
	active BOOLEAN NOT NULL DEFAULT TRUE
);