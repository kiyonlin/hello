package model

import (
	"github.com/jinzhu/gorm"
	"strings"
	"errors"
	"strconv"
	"hello/util"
	"sync"
)

const OKEX = "okex"
const Huobi = "huobi"
const Binance = "binance"

var HuobiAccountId = "1651065"

const BaseCarryCost = 0.0008 // 当前搬砖的最低手续费是万分之八
const MaxCarryMargin = 0.003

var ApplicationConfig *Config

var ApplicationAccounts = NewAccounts()
var ApplicationDB *gorm.DB
var CarryChannel = make(chan Carry, 50)
var BidChannel = make(chan Carry, 50)
var AskChannel = make(chan Carry, 50)
var AccountChannel = make(chan map[string]*Account, 50)
var ApplicationMarkets *Markets

const CarryStatusSuccess = "success"
const CarryStatusFail = "fail"
const CarryStatusWorking = "working"

var OrderStatusMap = map[string]string{
	// Binance
	"NEW":              CarryStatusWorking,
	"PARTIALLY_FILLED": CarryStatusWorking,
	"PENDING_CANCEL":   CarryStatusWorking,
	"FILLED":           CarryStatusSuccess,
	"CANCELED":         CarryStatusFail,
	"REJECTED":         CarryStatusFail,
	"EXPIRED":          CarryStatusFail,
	// Huobi
	"pre-submitted":    CarryStatusWorking,
	"submitting":       CarryStatusWorking,
	"submitted":        CarryStatusWorking,
	"partial-filled":   CarryStatusSuccess,
	"filled":           CarryStatusSuccess,
	"partial-canceled": CarryStatusSuccess,
	"canceled":         CarryStatusFail,
	// Okex
	"-1": CarryStatusFail,    //已撤销
	"0":  CarryStatusWorking, //未成交
	"1":  CarryStatusWorking, //部分成交
	"2":  CarryStatusSuccess, //完全成交
	"3":  CarryStatusWorking} //撤单处理中

// TODO filter out unsupported symbol for each market
func getWSSubscribe(market, symbol string) (subscribe string) {
	switch market {
	case Huobi: // xrp_btc: market.xrpbtc.depth.step0
		return "market." + strings.Replace(symbol, "_", "", 1) + ".depth.step0"
	case OKEX: // xrp_btc: ok_sub_spot_xrp_btc_depth_5
		return "ok_sub_spot_" + symbol + "_depth_5"
	case Binance: // xrp_btc: XRPBTC
		return strings.ToUpper(strings.Replace(symbol, "_", "", 1))
	}
	return ""
}

func getSymbolWithSplit(original, split string) (symbol string) {
	original = strings.ToLower(original)
	var moneyCurrency string
	if strings.LastIndex(original, "btc")+3 == len(original) {
		moneyCurrency = "btc"
	} else if strings.LastIndex(original, "usdt")+4 == len(original) {
		moneyCurrency = "usdt"
	} else if strings.LastIndex(original, "eth")+3 == len(original) {
		moneyCurrency = "eth"
	}
	return original[0:strings.LastIndex(original, moneyCurrency)] + split + moneyCurrency
}

func GetSymbol(market, subscribe string) (symbol string) {
	switch market {
	case Huobi: //market.xrpbtc.depth.step0: xrp_btc
		subscribe = strings.Replace(subscribe, "market.", "", 1)
		subscribe = strings.Replace(subscribe, ".depth.step0", "", 1)
		return getSymbolWithSplit(subscribe, "_")
	case OKEX: //ok_sub_spot_xrp_btc_depth_5: xrp_btc
		subscribe = strings.Replace(subscribe, "ok_sub_spot_", "", 1)
		subscribe = strings.Replace(subscribe, "_depth_5", "", 1)
		return subscribe
	case Binance: // XRPBTC: xrp_btc
		return getSymbolWithSplit(subscribe, "_")
	}
	return ""
}

type Config struct {
	lock         sync.Mutex
	Balance      float64
	Env          string
	DBConnection string
	Channels     int
	ChannelSlot  float64
	Markets      []string
	Symbols      []string
	Margins      []float64
	Delays       []float64
	Deduction    float64
	subscribes   map[string][]string // marketName - subscribes
	WSUrls       map[string]string   // marketName - ws url
	RestUrls     map[string]string   // marketName - rest url
	ApiKeys      map[string]string
	ApiSecrets   map[string]string
}

func (config *Config) DecreaseMargin(symbol string) {
	config.lock.Lock()
	defer config.lock.Unlock()
	currentMargin, _ := config.GetMargin(symbol)
	config.SetMargin(symbol, currentMargin-0.0001)
}

func (config *Config) IncreaseMargin(symbol string) {
	config.lock.Lock()
	defer config.lock.Unlock()
	currentMargin, _ := config.GetMargin(symbol)
	config.SetMargin(symbol, currentMargin+0.0001)
}

func SetApiKeys() {
	ApplicationConfig.ApiKeys = make(map[string]string)
	ApplicationConfig.ApiSecrets = make(map[string]string)
	util.Notice("begin to set keys")
	//if ApplicationConfig.Env == "aws" {
	//	util.Notice("under aws environment")
	ApplicationConfig.ApiKeys[Huobi] = "003fe1c2-1a5a12e1-73668e50-6773e"    // sammi
	ApplicationConfig.ApiKeys[OKEX] = "bb709a25-4d5b-4d9a-83ba-17cb514506fc" // sammi
	ApplicationConfig.ApiKeys[Binance] = "IkR9OHIQPe9YZtCUGa8Haa6hYQuyRFISYfTc05OkU2m3bujqL9evUoOLuKjsGm3q"
	ApplicationConfig.ApiSecrets[Huobi] = "05d114f3-6f455bf3-a640f2c4-06050" // sammi
	ApplicationConfig.ApiSecrets[OKEX] = "7D0E1B435964B96D72728215CB369CD7"  // sammi
	ApplicationConfig.ApiSecrets[Binance] = "xH2xGFmvSoy0LPtAaFElFbChxplbiEpyP2Bp9ZFo3zYlsaAyZ0DlTjA0bH1Tcndy"
	//}
}
func NewConfig() {
	ApplicationConfig = &Config{}
	ApplicationConfig.subscribes = make(map[string][]string)
	ApplicationConfig.WSUrls = make(map[string]string)
	ApplicationConfig.WSUrls[Huobi] = "wss://api.huobi.pro/ws"
	ApplicationConfig.WSUrls[OKEX] = "wss://real.okex.com:10441/websocket"
	ApplicationConfig.WSUrls[Binance] = "wss://stream.binance.com:9443/stream?streams="

	ApplicationConfig.RestUrls = make(map[string]string)
	// HUOBI用于交易的API，可能不适用于行情
	//config.RestUrls[Huobi] = "https://api.huobipro.com/v1"
	ApplicationConfig.RestUrls[Huobi] = "https://api.huobi.pro"
	ApplicationConfig.RestUrls[OKEX] = "https://www.okex.com/api/v1"
	ApplicationConfig.RestUrls[Binance] = "https://api.binance.com"
}

func (config *Config) GetSubscribes(marketName string) []string {
	for _, v := range config.Markets {
		if v == marketName {
			if config.subscribes[v] == nil {
				config.subscribes[v] = make([]string, len(config.Symbols))
				for i, symbol := range config.Symbols {
					config.subscribes[v][i] = getWSSubscribe(marketName, symbol)
				}
			}
			return config.subscribes[v]
		}
	}
	return nil
}

func (config *Config) SetMargin(symbol string, margin float64) {
	for i, value := range config.Symbols {
		if value == symbol {
			if margin < BaseCarryCost {
				margin = BaseCarryCost
			} else if margin > MaxCarryMargin {
				margin = MaxCarryMargin
			}
			config.Margins[i] = margin
			break
		}
	}
}

func (config *Config) GetMargin(symbol string) (float64, error) {
	//if len(config.Margins) == 1 {
	//	if config.Margins[0] < BaseCarryCost {
	//		config.Margins[0] = BaseCarryCost
	//	}
	//	// return first margin as default margin
	//	return config.Margins[0], nil
	//}
	for i, value := range config.Symbols {
		if value == symbol {
			if config.Margins[i] < BaseCarryCost {
				config.Margins[i] = BaseCarryCost
			}
			return config.Margins[i], nil
		}
	}
	return 1, errors.New("no such symbol")
}

func (config *Config) GetDelay(symbol string) (float64, error) {
	if len(config.Delays) == 1 {
		// return first delay as default delay
		return config.Delays[0], nil
	}
	for i, value := range config.Symbols {
		if value == symbol {
			return config.Delays[i], nil
		}
	}
	return 0, errors.New("no such symbol")
}

var currencyPrice = make(map[string]float64)
var getBuyPriceOkexTime = make(map[string]int64)

func GetBuyPriceOkex(symbol string) (buy float64, err error) {
	if ApplicationConfig == nil {
		NewConfig()
	}
	if getBuyPriceOkexTime[symbol] != 0 && util.GetNowUnixMillion()-getBuyPriceOkexTime[symbol] < 3600000 {
		return currencyPrice[symbol], nil
	}
	getBuyPriceOkexTime[symbol] = util.GetNowUnixMillion()
	strs := strings.Split(symbol, "_")
	if strs[0] == strs[1] {
		currencyPrice[symbol] = 1
	} else {
		headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded",
			"User-Agent": "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/39.0.2171.71 Safari/537.36"}
		responseBody, _ := util.HttpRequest("GET", ApplicationConfig.RestUrls[OKEX]+"/ticker.do?symbol="+symbol, "", headers)
		tickerJson, err := util.NewJSON(responseBody)
		if err == nil {
			strBuy, _ := tickerJson.GetPath("ticker", "buy").String()
			currencyPrice[symbol], err = strconv.ParseFloat(strBuy, 64)
		}
	}
	return currencyPrice[symbol], err
}
