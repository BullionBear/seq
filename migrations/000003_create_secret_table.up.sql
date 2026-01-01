CREATE TABLE secrets (
	acct_id INT PRIMARY KEY,
	username VARCHAR(255) NOT NULL,
	acct_name VARCHAR(255) NOT NULL,
	exchange VARCHAR(255) NOT NULL,
	api_key VARCHAR(255) NOT NULL,
	api_secret VARCHAR(255) NOT NULL,
	passphrase VARCHAR(255) NOT NULL,
	master_is INT NOT NULL
);