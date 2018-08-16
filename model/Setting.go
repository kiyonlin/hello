package model

type Setting struct {
	Market           string
	Symbol           string
	Margin           float64 // carry use
	TurtleLeftCopy   int     // turtle use
	TurtleLeftAmount float64 // turtle use
	TurtlePriceWidth float64 // turtle use
	MinPrice         float64 // turtle use
	MaxPrice         float64 // turtle use
	OpenShortMargin  float64 // future use
	CloseShortMargin float64 // future use
	Valid            bool
	ID               uint `gorm:"primary_key"`
}

var marketSymbolSetting map[string]map[string]*Setting // marketName - symbol - setting

func LoadSettings() {
	ApplicationSettings = []Setting{}
	ApplicationDB.Where(`valid = ?`, true).Find(&ApplicationSettings)
	marketSymbolSetting = make(map[string]map[string]*Setting)
	for i := range ApplicationSettings {
		if marketSymbolSetting[ApplicationSettings[i].Market] == nil {
			marketSymbolSetting[ApplicationSettings[i].Market] = make(map[string]*Setting)
		}
		if marketSymbolSetting[ApplicationSettings[i].Market][ApplicationSettings[i].Symbol] == nil {
			marketSymbolSetting[ApplicationSettings[i].Market][ApplicationSettings[i].Symbol] = &ApplicationSettings[i]
		}
	}
}

func GetMarketSettings(market string) map[string]*Setting {
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

func GetSymbols(market string) []string {
	symbols := make([]string, len(marketSymbolSetting[market]))
	i := 0
	for key := range marketSymbolSetting[market] {
		symbols[i] = key
		i++
	}
	return symbols
}

func GetSetting(market, symbol string) *Setting {
	if marketSymbolSetting[market] == nil {
		return nil
	}
	return marketSymbolSetting[market][symbol]
}

func GetMargin(symbol string) float64 {
	margins := make(map[string]float64)
	for _, value := range ApplicationSettings {
		if margins[value.Symbol] < value.Margin {
			margins[value.Symbol] = value.Margin
		}
	}
	if margins[symbol] == 0 { // 无值状态下的保护策略
		margins[symbol] = 1
	}
	return margins[symbol]
}
