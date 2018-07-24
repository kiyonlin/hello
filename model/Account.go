package model

import (
	"sync"
	"time"
)

type Accounts struct {
	Lock               sync.Mutex
	TotalInUsdt        float64
	MarketTotal        map[string]float64             // marketName - totalInUsdt
	CurrencyTotal      map[string]float64             // currency - totalInUsdt
	CurrencyPercentage map[string]float64             // currency - percentage
	Data               map[string]map[string]*Account // marketName - currency - Account
}

type Account struct {
	Market      string
	Currency    string
	Free        float64
	Frozen      float64
	PriceInUsdt float64
	Percentage  float64
	ID          uint `gorm:"primary_key"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func NewAccounts() *Accounts {
	accounts := &Accounts{}
	accounts.Data = make(map[string]map[string]*Account)
	accounts.CurrencyPercentage = make(map[string]float64)
	accounts.MarketTotal = make(map[string]float64)
	accounts.CurrencyTotal = make(map[string]float64)
	return accounts
}

func (accounts *Accounts) ClearAccounts(marketName string) {
	accounts.Lock.Lock()
	defer accounts.Lock.Unlock()
	accounts.Data[marketName] = nil
}

func (accounts *Accounts) GetAccount(marketName string, currency string) *Account {
	accounts.Lock.Lock()
	defer accounts.Lock.Unlock()
	if accounts.Data[marketName] == nil {
		return nil
	}
	return accounts.Data[marketName][currency]
}

func (accounts *Accounts) GetAccounts(marketName string) map[string]*Account {
	accounts.Lock.Lock()
	defer accounts.Lock.Unlock()
	return accounts.Data[marketName]
}

func (accounts *Accounts) SetAccount(marketName string, currency string, account *Account) {
	accounts.Lock.Lock()
	defer accounts.Lock.Unlock()
	if accounts.Data[marketName] == nil {
		accounts.Data[marketName] = make(map[string]*Account)
	}
	accounts.Data[marketName][currency] = account
}
