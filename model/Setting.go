package model

import (
	"time"
)

type CarryHandler func(market, symbol string)

type Setting struct {
	Function          string
	Market            string
	Symbol            string
	FunctionParameter string
	AccountType       string
	Margin            float64 // carry use
	OpenShortMargin   float64 // arbitrary future use
	CloseShortMargin  float64 // arbitrary future use
	Chance            float64 // arbitrary future use
	GridAmount        float64
	GridPriceDistance float64
	TurtleBalanceRate float64
	Valid             bool
	ID                uint `gorm:"primary_key"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

var marketSymbolSetting map[string]map[string]map[string]*Setting // function - marketName - symbol - setting
var handlers map[string]map[string]map[string]CarryHandler        // market - symbol - function- carryHandler

func GetFunctionMarkets(function string) []string {
	if marketSymbolSetting[function] == nil {
		return nil
	}
	markets := make([]string, len(marketSymbolSetting[function]))
	i := 0
	for key := range marketSymbolSetting[function] {
		markets[i] = key
		i++
	}
	return markets
}

func GetSetting(function, market, symbol string) *Setting {
	if marketSymbolSetting[function] == nil || marketSymbolSetting[function][market] == nil {
		return nil
	}
	return marketSymbolSetting[function][market][symbol]
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
	marketSymbolSetting = make(map[string]map[string]map[string]*Setting)
	binanceSettings := make(map[string]*Setting)
	handlers = make(map[string]map[string]map[string]CarryHandler)
	for i := range AppSettings {
		market := AppSettings[i].Market
		function := AppSettings[i].Function
		symbol := AppSettings[i].Symbol
		if marketSymbolSetting[function] == nil {
			marketSymbolSetting[function] = make(map[string]map[string]*Setting)
		}
		if marketSymbolSetting[function][market] == nil {
			marketSymbolSetting[function][market] = make(map[string]*Setting)
		}
		marketSymbolSetting[function][market][symbol] = &AppSettings[i]
		if AppSettings[i].Function == FunctionRefresh {
			binanceSettings[symbol] = &Setting{Market: Binance, Symbol: AppSettings[i].Symbol}
		}
		if handlers[market] == nil {
			handlers[market] = make(map[string]map[string]CarryHandler)
		}
		if handlers[market][symbol] == nil {
			handlers[market][symbol] = make(map[string]CarryHandler)
		}
		handlers[market][symbol][function] = HandlerMap[function]
	}
	for _, setting := range binanceSettings {
		AppSettings = append(AppSettings, *setting)
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
