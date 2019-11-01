package model

import "time"

type Candle struct {
	Symbol      string
	Ts          int64
	PriceFmex   float64
	PriceBitmex float64
	ID          uint `gorm:"primary_key"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
