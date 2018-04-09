package model

import (
	"sync"
	"github.com/jinzhu/gorm"
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
	gorm.Model
}

func NewAccounts() *Accounts {
	accounts := &Accounts{}
	accounts.Data = make(map[string]map[string]*Account)
	return accounts
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
