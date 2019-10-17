package model

import (
	"sync"
	"time"
)

type Accounts struct {
	Lock sync.Mutex
	//TotalInUsdt float64
	//MarketTotal        map[string]float64             // marketName - totalInUsdt
	//CurrencyTotal      map[string]float64             // currency - totalInUsdt
	//CurrencyPercentage map[string]float64             // currency - percentage
	Data map[string]map[string]*Account // marketName - currency - Account
}

type Account struct {
	Market                       string
	Currency                     string
	Free                         float64
	Frozen                       float64
	PriceInUsdt                  float64
	ProfitReal                   float64
	ProfitUnreal                 float64
	Direction                    string
	AccountUpdateTime            time.Time
	Timestamp                    time.Time
	Margin                       float64 // 仓位保证金，必须为正
	BankruptcyPrice              float64 // 破产价格，以该价格平仓，扣除taker手续费后，其权益恰好为0
	LiquidationPrice             float64 // 强平价格，以该价格平仓，扣除taker手续费后，其剩余权益恰好为仓位价值 x 维持保证金率
	EntryPrice                   float64 // 开仓均价，每次仓位增加或减少时，开仓均价都会调整
	Closed                       bool    // 仓位是否关闭
	MinimumMaintenanceMarginRate float64 //最小维持保证金，如果仓位保证金降低到此，将立刻触发强平
	//Percentage  float64
	ID        uint `gorm:"primary_key"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func NewAccounts() *Accounts {
	accounts := &Accounts{}
	accounts.Data = make(map[string]map[string]*Account)
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

func (accounts *Accounts) SetAccount(marketName string, currency string, account *Account) {
	accounts.Lock.Lock()
	defer accounts.Lock.Unlock()
	if accounts.Data[marketName] == nil {
		accounts.Data[marketName] = make(map[string]*Account)
	}
	accounts.Data[marketName][currency] = account
}
