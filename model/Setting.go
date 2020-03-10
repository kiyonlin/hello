package model

import (
	"fmt"
	"hello/util"
	"time"
)

type CarryHandler func(setting *Setting)

type Setting struct {
	Valid             bool
	Function          string
	Market            string
	MarketRelated     string
	Symbol            string
	FunctionParameter string
	AccountType       string
	PriceX            float64
	OpenShortMargin   float64 // arbitrary future use
	CloseShortMargin  float64 // arbitrary future use
	Chance            float64 // arbitrary future use
	GridAmount        float64
	GridPriceDistance float64
	AmountLimit       float64
	RefreshLimit      float64
	RefreshLimitLow   float64
	BinanceDisMin     float64
	BinanceDisMax     float64
	RefreshSameTime   int  // 1 stands for same time, otherwise separate
	ID                uint `gorm:"primary_key"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

var marketSymbolSetting map[string]map[string]map[string][]*Setting // function - marketName - symbol - setting
var handlers map[string]map[string]map[string]CarryHandler          // market - symbol - function- carryHandler

func GetSetting(function, market, symbol string) []*Setting {
	if marketSymbolSetting[function] == nil || marketSymbolSetting[function][market] == nil {
		return nil
	}
	return marketSymbolSetting[function][market][symbol]
}

func GetCurrentN(function string) (currentN float64) {
	if marketSymbolSetting[function] == nil {
		return 0
	}
	for _, value := range marketSymbolSetting[function] {
		for _, settings := range value {
			for _, setting := range settings {
				if setting != nil {
					currentN += setting.Chance
				}
			}
		}
	}
	return currentN
}

func GetFunctions(market, symbol string) map[string]CarryHandler {
	if handlers == nil {
		LoadSettings()
	}
	if handlers[market] == nil {
		return nil
	}
	return handlers[market][symbol]
}

func LoadSettings() {
	AppSettings = []Setting{}
	AppDB.Where(`valid = ?`, true).Find(&AppSettings)
	marketSymbolSetting = make(map[string]map[string]map[string][]*Setting)
	handlers = make(map[string]map[string]map[string]CarryHandler)
	for i := range AppSettings {
		market := AppSettings[i].Market
		function := AppSettings[i].Function
		symbol := AppSettings[i].Symbol
		if marketSymbolSetting[function] == nil {
			marketSymbolSetting[function] = make(map[string]map[string][]*Setting)
		}
		if marketSymbolSetting[function][market] == nil {
			marketSymbolSetting[function][market] = make(map[string][]*Setting)
		}
		if marketSymbolSetting[function][market][symbol] == nil {
			marketSymbolSetting[function][market][symbol] = make([]*Setting, 0)
		}
		marketSymbolSetting[function][market][symbol] = append(marketSymbolSetting[function][market][symbol],
			&AppSettings[i])
		if handlers[market] == nil {
			handlers[market] = make(map[string]map[string]CarryHandler)
		}
		if handlers[market][symbol] == nil {
			handlers[market][symbol] = make(map[string]CarryHandler)
		}
		if handlers[market][symbol][function] == nil {
			handlers[market][symbol][function] = HandlerMap[function]
		} else {
			handlers[market][symbol][fmt.Sprintf(`%s_%d`, function, util.GetNow().UnixNano())] =
				HandlerMap[function]
		}
	}
}

func GetMarketSymbols(market string) map[string]bool {
	if AppSettings == nil {
		LoadSettings()
	}
	symbols := make(map[string]bool)
	for _, value := range AppSettings {
		if value.Market == market {
			symbols[value.Symbol] = true
		}
	}
	return symbols
}

func GetMarkets() []string {
	if AppSettings == nil {
		LoadSettings()
	}
	marketMap := make(map[string]bool)
	for _, value := range AppSettings {
		marketMap[value.Market] = true
	}
	markets := make([]string, len(marketMap))
	i := 0
	for key := range marketMap {
		markets[i] = key
		i++
	}
	return markets
}
