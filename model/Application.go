package model

import (
	"github.com/jinzhu/gorm"
	"strings"
	"errors"
)

const OKEX = "okex"
const Huobi = "huobi"
const Binance = "binance"

var HuobiAccountId = "1651065"

var ApplicationConfig *Config

var ApplicationAccounts = NewAccounts()
var ApplicationDB *gorm.DB
var CarryChannel = make(chan Carry, 50)
var AccountChannel = make(chan Account, 50)

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
	"partial-filled":   CarryStatusWorking,
	"filled":           CarryStatusSuccess,
	"partial-canceled": CarryStatusWorking,
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
	return original[0: strings.LastIndex(original, moneyCurrency)] + split + moneyCurrency
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
	config.ApiKeys[Huobi] = "05005ed9-befbbed5-263dcb1d-8d835"
	config.ApiKeys[OKEX] = "f8b9e6ac-dbd6-469e-9b10-4c1efc9d8d4c"
	config.ApiKeys[Binance] = "SrIXmREkCSaVYiqutcUsQkP0z8srg4OMU9kKLODFtAiUgwbBlzebVIeXOrFWkZv0"
	config.ApiSecrets = make(map[string]string)
	config.ApiSecrets[Huobi] = "a0e763f3-736408f9-5bca700b-8e51f"
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
		// return first margin as default margin
		return config.Margins[0], nil
	}
	for i, value := range config.Symbols {
		if value == symbol {
			return config.Margins[i], nil
		}
	}
	return 0, errors.New("no such symbol")
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
