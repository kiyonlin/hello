package model

import "time"

type FutureAccount struct {
	Market      string
	Symbol      string
	OpenedLong  float64
	OpenedShort float64
	ID          uint `gorm:"primary_key"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
