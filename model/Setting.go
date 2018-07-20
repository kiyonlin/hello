package model

type Setting struct {
	Market           string
	Symbol           string
	Margin           float64
	TurtleLeftCopy   int
	TurtleLeftAmount float64
	TurtlePriceWidth float64
	Valid            bool
	ID               uint `gorm:"primary_key"`
}

var marketSymbolSetting map[string]map[string]*Setting // marketName - symbol - setting

func LoadSettings() {
	ApplicationSettings = []Setting{}
	ApplicationDB.Where(`valid = ?`, true).Find(&ApplicationSettings)
	marketSymbolSetting = make(map[string]map[string]*Setting)
	for _, value := range ApplicationSettings {
		if marketSymbolSetting[value.Market] == nil {
			marketSymbolSetting[value.Market] = make(map[string]*Setting)
		}
		if marketSymbolSetting[value.Market][value.Symbol] == nil {
			marketSymbolSetting[value.Market][value.Symbol] = &value
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

func GetTurtleSetting(market, symbol string) (amount, priceWidth, leftLimit float64) {
	if marketSymbolSetting[market] == nil {
		return 0, 0, 0
	}
	if marketSymbolSetting[market][symbol] == nil {
		return 0, 0, 0
	}
	setting := marketSymbolSetting[market][symbol]
	return setting.TurtleLeftAmount, setting.TurtlePriceWidth, float64(setting.TurtleLeftCopy) * setting.TurtleLeftAmount
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
