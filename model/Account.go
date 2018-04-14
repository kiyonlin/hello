package model

import (
	"sync"
	"time"
)

type Accounts struct {
	lock sync.Mutex
	Data map[string]map[string]*Account // marketName - currency - Account
}

type Account struct {
	BelongDate  string
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
	return accounts
}

func (accounts *Accounts) ClearAccounts(marketName string) {
	accounts.lock.Lock()
	defer accounts.lock.Unlock()
	accounts.Data[marketName] = nil
}

func (accounts *Accounts) GetAccount(marketName string, currency string) *Account {
	accounts.lock.Lock()
	defer accounts.lock.Unlock()
	if accounts.Data[marketName] == nil {
		return nil
	}
	return accounts.Data[marketName][currency]
}

func (accounts *Accounts) SetAccount(marketName string, currency string, account *Account) {
	accounts.lock.Lock()
	defer accounts.lock.Unlock()
	if accounts.Data[marketName] == nil {
		accounts.Data[marketName] = make(map[string]*Account)
	}
	accounts.Data[marketName][currency] = account
}

func (accounts *Accounts) Maintain(marketName string) {
	if accounts.Data[marketName] == nil {
		return
	}
	var totalInUsdt float64
	for key, value := range accounts.Data[marketName] {
		value.PriceInUsdt, _ = GetBuyPriceOkex(key + "_usdt")
		totalInUsdt += value.PriceInUsdt * (value.Free + value.Frozen)
	}
	for _, value := range accounts.Data[marketName] {
		value.Percentage = value.PriceInUsdt * (value.Free + value.Frozen) / totalInUsdt
	}
	AccountChannel <- accounts.Data[marketName]
}
