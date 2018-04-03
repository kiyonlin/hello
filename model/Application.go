package model

import (
	"github.com/jinzhu/gorm"
	"errors"
)

const OKEX = "okex"
const Huobi = "huobi"
const Binance = "binance"

const XrpBtc = "xrp_btc"
const BtcUsdt = "btc_usdt"
const EosBtc = "eos_btc"
const EthUsdt = "eth_usdt"
const EthBtc = "eth_btc"

var HuobiAccountId = "1449736"

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

// marketName -symbol - subscribe
var MarketSymbolSubscribe = map[string]map[string]string{
	Huobi: {
		XrpBtc:  "market.xrpbtc.depth.step0",
		BtcUsdt: "market.btcusdt.depth.step0",
		EosBtc:  "market.eosbtc.depth.step0",
		EthUsdt: "market.ethusdt.depth.step0",
		EthBtc:  "market.ethbtc.depth.step0",},
	OKEX: {
		XrpBtc:  "ok_sub_spot_xrp_btc_depth_5",
		BtcUsdt: "ok_sub_spot_btc_usdt_depth_5",
		EosBtc:  "ok_sub_spot_eos_btc_depth_5",
		EthUsdt: "ok_sub_spot_eth_usdt_depth_5",
		EthBtc:  "ok_sub_spot_eth_btc_depth_5"},
	Binance: {
		XrpBtc:  "XRPBTC",
		BtcUsdt: "BTCUSDT",
		EosBtc:  "EOSBTC",
		EthUsdt: "ETHUSDT",
		EthBtc:  "ETHBTC"}}

var SubscribeSymbol = map[string]string{
	// huobi
	"market.xrpbtc.depth.step0":  XrpBtc,
	"market.btcusdt.depth.step0": BtcUsdt,
	"market.eosbtc.depth.step0":  EosBtc,
	"market.ethusdt.depth.step0": EthUsdt,
	"market.ethbtc.depth.step0":  EthBtc,
	// okex
	"ok_sub_spot_xrp_btc_depth_5":  XrpBtc,
	"ok_sub_spot_btc_usdt_depth_5": BtcUsdt,
	"ok_sub_spot_eos_btc_depth_5":  EosBtc,
	"ok_sub_spot_eth_usdt_depth_5": EthUsdt,
	"ok_sub_spot_eth_btc_depth_5":  EthBtc,
	// binance
	"XRPBTC":  XrpBtc,
	"BTCUSDT": BtcUsdt,
	"EOSBTC":  EosBtc,
	"ETHUSDT": EthUsdt,
	"ETHBTC":  EthBtc}

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
	config.ApiKeys[Huobi] = "40efea2f-d65e049a-1fe4c855-c7717"
	config.ApiKeys[OKEX] = "f8b9e6ac-dbd6-469e-9b10-4c1efc9d8d4c"
	config.ApiKeys[Binance] = "SrIXmREkCSaVYiqutcUsQkP0z8srg4OMU9kKLODFtAiUgwbBlzebVIeXOrFWkZv0"
	config.ApiSecrets = make(map[string]string)
	config.ApiSecrets[Huobi] = "e3421043-f8fa52b1-bf026cef-6462f"
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
					config.subscribes[v][i] = MarketSymbolSubscribe[marketName][symbol]
				}
			}
			return config.subscribes[v]
		}
	}
	return nil
}

func (config *Config) GetMargin(symbol string) (float64, error) {
	for i, value := range config.Symbols {
		if value == symbol {
			return config.Margins[i], nil
		}
	}
	return 0, errors.New("no such symbol")
}

func (config *Config) GetDelay(symbol string) (float64, error) {
	for i, value := range config.Symbols {
		if value == symbol {
			return config.Delays[i], nil
		}
	}
	return 0, errors.New("no such symbol")
}
