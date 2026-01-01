package pms

import "gorm.io/gorm"

type PMS struct {
	db *gorm.DB
}

func NewPMS(db *gorm.DB) *PMS {
	return &PMS{db: db}
}
