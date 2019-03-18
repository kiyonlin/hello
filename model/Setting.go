package model

import (
	"strings"
	"time"
)

type Setting struct {
	Function          string
	Market            string
	Symbol            string
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

func LoadSettings() {
	AppSettings = []Setting{}
	AppDB.Where(`valid = ?`, true).Find(&AppSettings)
	marketSymbolSetting = make(map[string]map[string]map[string]*Setting)
	for i := range AppSettings {
		if marketSymbolSetting[AppSettings[i].Function] == nil {
			marketSymbolSetting[AppSettings[i].Function] = make(map[string]map[string]*Setting)
		}
		if marketSymbolSetting[AppSettings[i].Function][AppSettings[i].Market] == nil {
			marketSymbolSetting[AppSettings[i].Function][AppSettings[i].Market] = make(map[string]*Setting)
		}
		marketSymbolSetting[AppSettings[i].Function][AppSettings[i].Market][AppSettings[i].Symbol] = &AppSettings[i]
	}
}

func GetFunctionMarketSettings(function, market string) map[string]*Setting {
	if marketSymbolSetting == nil {
		LoadSettings()
	}
	if marketSymbolSetting[function] == nil {
		return nil
	}
	return marketSymbolSetting[function][market]
}

func GetMarketSettings(market string) map[string]*Setting {
	if AppSettings == nil {
		LoadSettings()
	}
	settings := make(map[string]*Setting)
	for _, value := range AppSettings {
		if value.Market == market {
			settings[value.Symbol] = &value
		}
	}
	return settings
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
	}
	return markets
}

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

func GetSettings(function, market, symbolPrefix string) (settings []*Setting) {
	if marketSymbolSetting[function] == nil || marketSymbolSetting[function][market] == nil {
		return nil
	}
	settings = make([]*Setting, 0)
	for _, value := range marketSymbolSetting[function][market] {
		if strings.Index(value.Symbol, symbolPrefix) == 0 {
			settings = append(settings, value)
		}
	}
	return settings
}

func GetSetting(function, market, symbol string) *Setting {
	if marketSymbolSetting[function] == nil || marketSymbolSetting[function][market] == nil {
		return nil
	}
	return marketSymbolSetting[function][market][symbol]
}

func GetMargin(symbol string) float64 {
	margins := make(map[string]float64)
	for _, value := range AppSettings {
		if margins[value.Symbol] < value.Margin {
			margins[value.Symbol] = value.Margin
		}
	}
	if margins[symbol] == 0 { // 无值状态下的保护策略
		margins[symbol] = 1
	}
	return margins[symbol]
}
