package sms

import (
	"fmt"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

const (
	QueryAllSecrets = `
		SELECT * FROM secrets
	`
	InsertSecret = `
		INSERT INTO secrets (acct_id, acct_name, exchange, api_key, api_secret) VALUES (?, ?, ?, ?, ?)
	`
)

type SecretManager struct {
	db      *gorm.DB
	secrets map[int]Secret
}

func NewSecretManager(db *gorm.DB) (*SecretManager, error) {
	secretManager := &SecretManager{db: db, secrets: make(map[int]Secret, 1024)}
	if err := secretManager.loadSecrets(); err != nil {
		log.Error().Err(err).Msg("Failed to load secrets")
		return nil, err
	}
	return secretManager, nil
}

func (s *SecretManager) loadSecrets() error {
	rows, err := s.db.Raw(QueryAllSecrets).Rows()
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var secret Secret
		err := rows.Scan(&secret.AcctID, &secret.AcctName, &secret.Exchange, &secret.APIKey, &secret.APISecret)
		if err != nil {
			return err
		}
		s.secrets[secret.AcctID] = secret
	}
	return nil
}

func (s *SecretManager) AddSecret(secret Secret) error {
	if _, ok := s.secrets[secret.AcctID]; ok {
		return fmt.Errorf("secret already exists for acctID: %d", secret.AcctID)
	}
	s.secrets[secret.AcctID] = secret
	if err := s.db.Exec(InsertSecret, secret.AcctID, secret.AcctName, secret.Exchange, secret.APIKey, secret.APISecret).Error; err != nil {
		return err
	}
	return nil
}

func (s *SecretManager) GetSecret(acctID int) (Secret, error) {
	secret, ok := s.secrets[acctID]
	if !ok {
		return Secret{}, fmt.Errorf("secret not found for acctID: %d", acctID)
	}
	return secret, nil
}
