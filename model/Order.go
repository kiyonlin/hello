package model

import "time"

type Order struct {
	OrderId    string
	Symbol     string
	Market     string
	OrderType  string
	OrderSide  string
	MoneySide  int //仅用于统计，OrderSide=sell时MoneySide=1；OrderSide=buy时MoneySide=-1
	ErrCode    string
	Status     string
	AmountType string
	Price      float64
	DealPrice  float64
	Amount     float64
	DealAmount float64
	Fee        float64
	FeeIncome  float64
	OrderTime  time.Time
	ID         uint `gorm:"primary_key"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
