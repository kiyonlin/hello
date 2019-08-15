package model

import "time"

type Order struct {
	Amount      float64
	AmountType  string
	DealAmount  float64
	DealPrice   float64
	ErrCode     string
	Fee         float64
	FeeIncome   float64
	Function    string
	Market      string
	OrderId     string
	OrderSide   string
	OrderTime   time.Time
	OrderType   string
	Price       float64
	RefreshType string // 1: near refresh 2: far refresh
	Status      string
	Symbol      string
	ID          uint `gorm:"primary_key"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Score struct {
	Symbol    string
	OrderSide string
	Point     float64
	Amount    float64
	Price     float64
	Position  int
	ID        uint `gorm:"primary_key"`
	CreatedAt time.Time
	UpdatedAt time.Time
}
