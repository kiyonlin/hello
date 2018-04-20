package model

import (
	"github.com/jinzhu/gorm"
	"strings"
	"errors"
	"github.com/haoweizh/hello/model"
	"strconv"
	"hello/util"
)

const OKEX = "okex"
const Huobi = "huobi"
const Binance = "binance"

var HuobiAccountId = "1651065"
const BaseCarryCost = 0.001 // 当前搬砖的最低手续费是千分之一
var ApplicationConfig *Config

var ApplicationAccounts = NewAccounts()
var ApplicationDB *gorm.DB
var CarryChannel = make(chan Carry, 50)
var BidChannel = make(chan Carry, 50)
var AskChannel = make(chan Carry, 50)
var AccountChannel = make(chan map[string]*Account, 50)

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
	Markets    []string
	Symbols    []string
	Margins    []float64
	Delays     []float64
	subscribes map[string][]string // marketName - subscribes
	WSUrls     map[string]string   // marketName - ws url
	RestUrls   map[string]string   // marketName - rest url
	ApiKeys    map[string]string
	ApiSecrets map[string]string
}

func NewConfig() *Config {
	config := &Config{}
	config.subscribes = make(map[string][]string)
	config.WSUrls = make(map[string]string)
	config.WSUrls[Huobi] = "wss://api.huobi.pro/ws"
	config.WSUrls[OKEX] = "wss://real.okex.com:10441/websocket"
	config.WSUrls[Binance] = "wss://stream.binance.com:9443/stream?streams="

	config.RestUrls = make(map[string]string)
	// HUOBI用于交易的API，可能不适用于行情
	//config.RestUrls[Huobi] = "https://api.huobipro.com/v1"
	config.RestUrls[Huobi] = "https://api.huobi.pro"
	config.RestUrls[OKEX] = "https://www.okex.com/api/v1"
	config.RestUrls[Binance] = "https://api.binance.com"
	config.ApiKeys = make(map[string]string)
	config.ApiKeys[Huobi] = "ff4f8f05-4993f78c-c707cc5b-22714" // sammi
	config.ApiKeys[OKEX] = "f8b9e6ac-dbd6-469e-9b10-4c1efc9d8d4c"
	config.ApiKeys[Binance] = "SrIXmREkCSaVYiqutcUsQkP0z8srg4OMU9kKLODFtAiUgwbBlzebVIeXOrFWkZv0"
	config.ApiSecrets = make(map[string]string)
	config.ApiSecrets[Huobi] = "2d293cd4-04d5c6e5-2b2d5d15-fb56b" // sammi
	config.ApiSecrets[OKEX] = "66786EDBC5F3230B7943DB520F86B492"
	config.ApiSecrets[Binance] = "rUAAgJxzzSRlrHwhNFZOJtiVsxGQvXBeH1GJbzmy9E72pWX1UL9CxbVyRWGzzdgI"
	return config
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

func (config *Config) GetMargin(symbol string) (float64, error) {
	if len(config.Margins) == 1 {
		if config.Margins[0] < BaseCarryCost {
			config.Margins[0] = BaseCarryCost
		}
		// return first margin as default margin
		return config.Margins[0], nil
	}
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
	if model.ApplicationConfig == nil {
		model.ApplicationConfig = model.NewConfig()
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
		responseBody, _ := util.HttpRequest("GET", model.ApplicationConfig.RestUrls[model.OKEX]+"/ticker.do?symbol="+symbol, "", headers)
		tickerJson, err := util.NewJSON(responseBody)
		if err == nil {
			strBuy, _ := tickerJson.GetPath("ticker", "buy").String()
			currencyPrice[symbol], err = strconv.ParseFloat(strBuy, 64)
		}
	}
	return currencyPrice[symbol], err
}
