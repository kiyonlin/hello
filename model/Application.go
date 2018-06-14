package model

import (
	"errors"
	"fmt"
	"github.com/jinzhu/gorm"
	"hello/util"
	"strconv"
	"strings"
	"sync"
)

const OKEX = "okex"
const Huobi = "huobi"
const Binance = "binance"
const Fcoin = "fcoin"

var HuobiAccountId = "1651065"

//const BaseCarryCost = 0.0004 // 当前搬砖的最低手续费是万分之4

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
	"pre-submitted": CarryStatusWorking,
	"submitting":    CarryStatusWorking,
	// Huobi&Fcoin
	"submitted":      CarryStatusWorking,
	"partial-filled": CarryStatusSuccess,
	// Huobi&Fcoin
	"filled":           CarryStatusSuccess,
	"partial-canceled": CarryStatusSuccess,
	// Huobi&Fcoin
	"canceled": CarryStatusFail,
	// Okex
	"-1": CarryStatusFail,    //已撤销
	"0":  CarryStatusWorking, //未成交
	"1":  CarryStatusWorking, //部分成交
	"2":  CarryStatusSuccess, //完全成交
	"3":  CarryStatusWorking, //撤单处理中
	// Fcoin
	"partial_filled":   CarryStatusSuccess,
	"partial_canceled": CarryStatusSuccess,
	"pending_cancel":   CarryStatusSuccess,
}

// TODO filter out unsupported symbol for each market
func getWSSubscribe(market, symbol string) (subscribe string) {
	switch market {
	case Huobi: // xrp_btc: market.xrpbtc.depth.step0
		return "market." + strings.Replace(symbol, "_", "", 1) + ".depth.step0"
	case OKEX: // xrp_btc: ok_sub_spot_xrp_btc_depth_5
		return "ok_sub_spot_" + symbol + "_depth_5"
	case Binance: // xrp_btc: XRPBTC
		return strings.ToUpper(strings.Replace(symbol, "_", "", 1))
	case Fcoin: // btc_usdt: depth.L20.btcusdt
		return `depth.L20.` + strings.ToLower(strings.Replace(symbol, "_", "", 1))
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
	case Fcoin: // btc_usdt: depth.L20.btcusdt
		subscribe := strings.Replace(subscribe, "depth.L20.", "", 1)
		return getSymbolWithSplit(subscribe, "_")
	}
	return ""
}

type Config struct {
	lock          sync.Mutex
	BaseCarryCost float64
	Balance       float64
	Env           string
	DBConnection  string
	Channels      int
	ChannelSlot   float64
	Markets       []string
	Symbols       []string
	Margins       []float64
	Delays        []float64
	Deduction     float64
	MinUsdt       float64             // 折合usdt最小下单金额
	MaxUsdt       float64             // 折合usdt最大下单金额
	subscribes    map[string][]string // marketName - subscribes
	WSUrls        map[string]string   // marketName - ws url
	RestUrls      map[string]string   // marketName - rest url
	ApiKeys       map[string]string
	ApiSecrets    map[string]string
}

func SetApiKeys() {
	ApplicationConfig.ApiKeys = make(map[string]string)
	ApplicationConfig.ApiSecrets = make(map[string]string)
	util.Notice("begin to set keys")
	//if ApplicationConfig.Env == "aws" {
	//	util.Notice("under aws environment")
	ApplicationConfig.ApiKeys[Huobi] = "003fe1c2-1a5a12e1-73668e50-6773e"    // sammi
	ApplicationConfig.ApiSecrets[Huobi] = "05d114f3-6f455bf3-a640f2c4-06050" // sammi
	ApplicationConfig.ApiKeys[OKEX] = "bb709a25-4d5b-4d9a-83ba-17cb514506fc" // sammi
	ApplicationConfig.ApiSecrets[OKEX] = "7D0E1B435964B96D72728215CB369CD7"  // sammi
	ApplicationConfig.ApiKeys[Binance] = "IkR9OHIQPe9YZtCUGa8Haa6hYQuyRFISYfTc05OkU2m3bujqL9evUoOLuKjsGm3q"
	ApplicationConfig.ApiSecrets[Binance] = "xH2xGFmvSoy0LPtAaFElFbChxplbiEpyP2Bp9ZFo3zYlsaAyZ0DlTjA0bH1Tcndy"
	ApplicationConfig.ApiKeys[Fcoin] = "4c1db3d5a7124fb0bcf79579cc94ae1a"    // 25 server ace fcoin
	ApplicationConfig.ApiSecrets[Fcoin] = "98002cf0d4f846a8b01e4ce73248ff28" // 25 server ace fcoin
	//ApplicationConfig.ApiKeys[Fcoin] = "7c26be189ddc4e59aeb6021cfbfc3415"    // 3 server ace fcoin
	//ApplicationConfig.ApiSecrets[Fcoin] = "54342819cbe148859f8d5ebdf384e607" // 3 server ace fcoin
	//}
}
func NewConfig() {
	ApplicationConfig = &Config{}
	ApplicationConfig.subscribes = make(map[string][]string)
	ApplicationConfig.WSUrls = make(map[string]string)
	ApplicationConfig.WSUrls[Huobi] = "wss://api.huobi.pro/ws"
	ApplicationConfig.WSUrls[OKEX] = "wss://real.okex.com:10441/websocket"
	ApplicationConfig.WSUrls[Binance] = "wss://stream.binance.com:9443/stream?streams="
	ApplicationConfig.WSUrls[Fcoin] = "wss://api.fcoin.com/v2/ws"

	ApplicationConfig.RestUrls = make(map[string]string)
	// HUOBI用于交易的API，可能不适用于行情
	//config.RestUrls[Huobi] = "https://api.huobipro.com/v1"
	ApplicationConfig.RestUrls[Fcoin] = "https://api.fcoin.com/v2"
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
	if strs[0] == strs[1] || strs[0] == `ft` {
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
