package model

import (
	"strings"
	"time"
)

type Setting struct {
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

var marketSymbolSetting map[string]map[string]*Setting // marketName - symbol - setting

func LoadSettings() {
	AppSettings = []Setting{}
	AppDB.Where(`valid = ?`, true).Find(&AppSettings)
	marketSymbolSetting = make(map[string]map[string]*Setting)
	for i := range AppSettings {
		if marketSymbolSetting[AppSettings[i].Market] == nil {
			marketSymbolSetting[AppSettings[i].Market] = make(map[string]*Setting)
		}
		marketSymbolSetting[AppSettings[i].Market][AppSettings[i].Symbol] = &AppSettings[i]
	}
}

func GetMarketSettings(market string) map[string]*Setting {
	if marketSymbolSetting == nil {
		LoadSettings()
	}
	return marketSymbolSetting[market]
}

func GetMarkets() []string {
	markets := make([]string, len(marketSymbolSetting))
	i := 0
	for key := range marketSymbolSetting {
		markets[i] = key
		i++
	}
	return markets
}

func GetSettings(market, symbolPrefix string) (settings []*Setting) {
	if marketSymbolSetting[market] == nil {
		return nil
	}
	settings = make([]*Setting, 0)
	for _, value := range marketSymbolSetting[market] {
		if strings.Index(value.Symbol, symbolPrefix) == 0 {
			settings = append(settings, value)
		}
	}
	return settings
}

func GetSetting(market, symbol string) *Setting {
	if marketSymbolSetting[market] == nil {
		return nil
	}
	return marketSymbolSetting[market][symbol]
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
