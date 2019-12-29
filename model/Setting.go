package model

import (
	"fmt"
	"hello/util"
	"strings"
	"time"
)

type CarryHandler func(market, symbol string)

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
func SetSetting(function, market, symbol string, setting *Setting) {
	if marketSymbolSetting[function] == nil {
		marketSymbolSetting[function] = make(map[string]map[string]*Setting)
	}
	if marketSymbolSetting[function][market] == nil {
		marketSymbolSetting[function][market] = make(map[string]*Setting)
	}
	marketSymbolSetting[function][market][symbol] = setting
}

func GetSetting(function, market, symbol string) *Setting {
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
		for _, setting := range value {
			if setting != nil {
				currentN += setting.Chance
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
	marketSymbolSetting = make(map[string]map[string]map[string]*Setting)
	//binanceSettings := make(map[string]*Setting)
	relatedSettings := make(map[string]*Setting)
	fcoinSettings := make(map[string]*Setting)
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
			//binanceSettings[symbol] = &Setting{Market: Binance, Symbol: AppSettings[i].Symbol}
			if AppSettings[i].Symbol == `btc_pax` {
				fcoinSettings[`btc_usdt`] = &Setting{Market: Fcoin, Symbol: `btc_usdt`}
				fcoinSettings[`pax_usdt`] = &Setting{Market: Fcoin, Symbol: `pax_usdt`}
			} else {
				relatedSettings[symbol] = &Setting{Market: Huobi, Symbol: AppSettings[i].Symbol}
			}
		}
		if AppSettings[i].Function == FunctionHangContract {
			relatedSettings[symbol] = &Setting{Market: Bitmex, Symbol: AppSettings[i].Symbol, Valid: true,
				Chance: AppSettings[i].Chance, RefreshLimitLow: AppSettings[i].RefreshLimitLow,
				RefreshLimit: AppSettings[i].RefreshLimit}
		}
		if AppSettings[i].MarketRelated != `` {
			marketsRelated := strings.Split(AppSettings[i].MarketRelated, `,`)
			for _, value := range marketsRelated {
				relatedSettings[symbol] = &Setting{Market: value, Symbol: AppSettings[i].Symbol, Valid: true}
			}
		}
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
	for _, setting := range relatedSettings {
		AppSettings = append(AppSettings, *setting)
	}
	for _, setting := range fcoinSettings {
		needAdd := true
		for _, value := range AppSettings {
			if value.Market == Fcoin && value.Symbol == setting.Symbol {
				needAdd = false
				break
			}
		}
		if needAdd {
			AppSettings = append(AppSettings, *setting)
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
