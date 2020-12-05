package model

import "time"

type Balance struct {
	AccountId     string
	Action        float64 // 1: deposit, -1: withdraw, 0: snapshot
	Address       string  // for transaction
	Amount        float64
	BalanceTime   time.Time // confirm time if transaction
	Coin          string
	Fee           string // for transaction
	Market        string
	Notes         string
	Price         float64 // price in usdt
	Status        string  // for transaction
	TransactionId string
	UsdValue      float64
	ID            string `gorm:"primary_key"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
