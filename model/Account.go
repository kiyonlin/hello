package model

import (
	"sync"
	"time"
	"hello/util"
)

type Accounts struct {
	lock               sync.Mutex
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
	accounts.lock.Lock()
	defer accounts.lock.Unlock()
	if accounts.Data[marketName] == nil {
		return
	}
	accounts.MarketTotal[marketName] = 0
	for key, value := range accounts.Data[marketName] {
		value.PriceInUsdt, _ = GetBuyPriceOkex(key + "_usdt")
		accounts.MarketTotal[marketName] += value.PriceInUsdt * (value.Free + value.Frozen)
	}
	if accounts.MarketTotal[marketName] == 0 {
		util.Notice(marketName + " balance is empty!!!!!!!!!!!")
		accounts.MarketTotal[marketName] = 1
	}
	for _, value := range accounts.Data[marketName] {
		value.Percentage = value.PriceInUsdt * (value.Free + value.Frozen) / accounts.MarketTotal[marketName]
	}
	// calculate currency percentage of all markets
	accounts.TotalInUsdt = 0
	for _, value := range accounts.MarketTotal {
		accounts.TotalInUsdt += value
	}
	accounts.CurrencyTotal = make(map[string]float64)
	for _, currencies := range accounts.Data {
		for currency, account := range currencies {
			accounts.CurrencyTotal[currency] += (account.Free + account.Frozen) * account.PriceInUsdt
		}
	}
	accounts.CurrencyPercentage = make(map[string]float64)
	for currency, value := range accounts.CurrencyTotal {
		accounts.CurrencyPercentage[currency] = value / accounts.TotalInUsdt
	}
	AccountChannel <- accounts.Data[marketName]
}
