package model

import (
	"fmt"
	"github.com/jinzhu/gorm"
	"strings"
	"sync"
)

const OKEX = "okex"
const Huobi = "huobi"
const Binance = "binance"
const Fcoin = "fcoin"
const Coinbig = "coinbig"
const Coinpark = "coinpark"
const Btcdo = `btcdo`

const OrderTypeLimit = `limit`
const OrderTypeMarket = `market`
const OrderSideBuy = `buy`
const OrderSideSell = `sell`

var HuobiAccountId = ""
var CurrencyPrice = make(map[string]float64)
var GetBuyPriceTime = make(map[string]int64)

var ApplicationConfig *Config

var ApplicationAccounts = NewAccounts()
var ApplicationDB *gorm.DB
var ApplicationSettings []Setting
var CarryChannel = make(chan Carry, 50)
var AccountChannel = make(chan map[string]*Account, 50)
var InnerCarryChannel = make(chan Carry, 50)
var RefreshCarryChannel = make(chan Carry, 50)

var ApplicationMarkets *Markets
var TurtleCarries = make(map[string]map[string]*Carry)    // market - symbol - *carry
var turtleDealPrice = make(map[string]map[string]float64) // market - symbol - price

const CarryStatusSuccess = "success"
const CarryStatusFail = "fail"
const CarryStatusWorking = "working"

const CarryTypeTurtle = `turtle`

var OrderStatusMap = map[string]string{
	``: CarryStatusFail,
	// Binance
	"NEW":              CarryStatusWorking,
	"PARTIALLY_FILLED": CarryStatusWorking,
	"PENDING_CANCEL":   CarryStatusSuccess,
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
	// Coinpark
	Coinpark + `1`: CarryStatusWorking, // 待成交
	Coinpark + `2`: CarryStatusSuccess, //部分成交
	Coinpark + `3`: CarryStatusSuccess, //完全成交
	Coinpark + `4`: CarryStatusSuccess, //部分撤销
	Coinpark + `5`: CarryStatusFail,    //完全撤销
	Coinpark + `6`: CarryStatusSuccess, //待撤销
	// Coinbig
	Coinbig + `1`: CarryStatusWorking, // 未成交
	Coinbig + `2`: CarryStatusWorking, // 部分成交,
	Coinbig + `3`: CarryStatusSuccess, // 完全成交,
	Coinbig + `4`: CarryStatusSuccess, // 用户撤销,
	Coinbig + `5`: CarryStatusSuccess, // 部分撤回,
	Coinbig + `6`: CarryStatusFail,    // 成交失败
}

func GetTurtleCarry(market, symbol string) (turtleCarry *Carry) {
	if TurtleCarries[market] == nil {
		return nil
	}
	return TurtleCarries[market][symbol]
}

func SetTurtleCarry(market, symbol string, turtleCarry *Carry) {
	if TurtleCarries[market] == nil {
		TurtleCarries[market] = make(map[string]*Carry)
	}
	TurtleCarries[market][symbol] = turtleCarry
}

func GetTurtleDealPrice(market, symbol string) (price float64) {
	if turtleDealPrice[market] == nil {
		return 0
	}
	return turtleDealPrice[market][symbol]
}

func SetTurtleDealPrice(market, symbol string, price float64) {
	if turtleDealPrice[market] == nil {
		turtleDealPrice[market] = make(map[string]float64)
	}
	turtleDealPrice[market][symbol] = price
}

// TODO filter out unsupported symbol for each market
func GetWSSubscribe(market, symbol string) (subscribe string) {
	switch market {
	case Huobi: // xrp_btc: market.xrpbtc.depth.step0
		return "market." + strings.Replace(symbol, "_", "", 1) + ".depth.step0"
	case OKEX: // xrp_btc: ok_sub_spot_xrp_btc_depth_5
		return "ok_sub_spot_" + symbol + "_depth_5"
	case Binance: // xrp_btc: XRPBTC
		return strings.ToUpper(strings.Replace(symbol, "_", "", 1))
	case Fcoin: // btc_usdt: depth.L20.btcusdt
		return `depth.L20.` + strings.ToLower(strings.Replace(symbol, "_", "", 1))
	case Coinpark: //BTC_USDT bibox_sub_spot_BTC_USDT_ticker
		//return `bibox_sub_spot_` + strings.ToUpper(symbol) + `_ticker`
		return `bibox_sub_spot_` + strings.ToUpper(symbol) + `_depth`
	case Btcdo:
		return strings.ToUpper(symbol)
	case Coinbig:
		switch symbol {
		case `btc_usdt`:
			return `27`
		case `eth_usdt`:
			return `28`
		}
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
		subscribe = strings.Replace(subscribe, "depth.L20.", "", 1)
		return getSymbolWithSplit(subscribe, "_")
	case Coinpark: //BTC_USDT bibox_sub_spot_BTC_USDT_ticker
		subscribe = strings.Replace(subscribe, `bibox_sub_spot_`, ``, 1)
		subscribe = strings.Replace(subscribe, `_ticker`, ``, 1)
		return subscribe
	case Coinbig: // btc_usdt 27 // eth_usdt 28
		switch subscribe {
		case `27`:
			return `btc_usdt`
		case `28`:
			return `eth_usdt`
		}
	}
	return ""
}

type Config struct {
	lock           sync.Mutex
	Env            string
	DBConnection   string
	Channels       int
	ChannelSlot    float64
	Functions      []string
	Delay          float64
	Deduction      float64
	MinUsdt        float64            // 折合usdt最小下单金额
	MaxUsdt        float64            // 折合usdt最大下单金额
	WSUrls         map[string]string  // marketName - ws url
	RestUrls       map[string]string  // marketName - rest url
	MarketCost     map[string]float64 // marketName - order cost
	HuobiKey       string
	HuobiSecret    string
	OkexKey        string
	OkexSecret     string
	BinanceKey     string
	BinanceSecret  string
	CoinbigKey     string
	CoinbigSecret  string
	CoinparkKey    string
	CoinparkSecret string
	FcoinKey       string
	FcoinSecret    string
	BtcdoKey       string
	BtcdoSecret    string
	OrderWait      int64   // fcoin/coinpark 刷单平均等待时间
	AmountRate     float64 // 刷单填写数量比率
	Handle         int64   // 0 不执行处理程序，1执行处理程序
	SellRate       float64 // fcoin dk 额外卖单下单比例
	FtMax          float64 // fcoin dk ft上限
	InChina        int     // 1 in china, otherwise outter china
}

func NewConfig() {
	ApplicationConfig = &Config{}
	ApplicationConfig.WSUrls = make(map[string]string)
	ApplicationConfig.WSUrls[Huobi] = "wss://api.huobi.pro/ws"
	ApplicationConfig.WSUrls[OKEX] = "wss://real.okex.com:10441/websocket"
	ApplicationConfig.WSUrls[Binance] = "wss://stream.binance.com:9443/stream?streams="
	ApplicationConfig.WSUrls[Fcoin] = "wss://api.fcoin.com/v2/ws"
	ApplicationConfig.WSUrls[Coinbig] = "wss://ws.coinbig.com/ws"
	ApplicationConfig.WSUrls[Coinpark] = "wss://push.coinpark.cc/"
	ApplicationConfig.WSUrls[Btcdo] = `wss://onli-quotation.btcdo.com/v1/market/?EIO=3&transport=websocket`
	ApplicationConfig.RestUrls = make(map[string]string)
	// HUOBI用于交易的API，可能不适用于行情
	//config.RestUrls[Huobi] = "https://api.huobipro.com/v1"
	ApplicationConfig.RestUrls[Fcoin] = "https://api.fcoin.com/v2"
	ApplicationConfig.RestUrls[Huobi] = "https://api.huobi.pro"
	ApplicationConfig.RestUrls[OKEX] = "https://www.okex.com/api/v1"
	ApplicationConfig.RestUrls[Binance] = "https://api.binance.com"
	ApplicationConfig.RestUrls[Coinbig] = "https://www.coinbig.com/api/publics/v1"
	ApplicationConfig.RestUrls[Coinpark] = "https://api.coinpark.cc/v1"
	ApplicationConfig.RestUrls[Btcdo] = `https://api.btcdo.com`
	ApplicationConfig.MarketCost = make(map[string]float64)
	ApplicationConfig.MarketCost[Fcoin] = 0
	ApplicationConfig.MarketCost[Huobi] = 0.0005
	ApplicationConfig.MarketCost[OKEX] = 0.0005
	ApplicationConfig.MarketCost[Binance] = 0.0004
	ApplicationConfig.MarketCost[Coinpark] = 0
	ApplicationConfig.MarketCost[Coinbig] = 0
}

func GetSubscribes(marketName string) []string {
	settings := GetMarketSettings(marketName)
	subscribes := make([]string, len(settings))
	i := 0
	for symbol := range settings {
		subscribes[i] = GetWSSubscribe(marketName, symbol)
		i++
	}
	return subscribes
}

func (config *Config) ToString() string {
	str := "markets-carry cost:\n"
	for key, value := range config.MarketCost {
		str += fmt.Sprintf("-%s base carry cost: %f\n", key, value)
	}
	str += fmt.Sprintf("deduction: %f\n", config.Deduction)
	str += fmt.Sprintf("delay: %f\n", config.Delay)
	str += fmt.Sprintf("channelslot: %f\n", config.ChannelSlot)
	str += fmt.Sprintf("minusdt: %f\n", config.MinUsdt)
	str += fmt.Sprintf("maxusdt: %f\n", config.MaxUsdt)
	str += "env: " + config.Env + "\n"
	str += fmt.Sprintf("channels: %d\n", config.Channels)
	str += "dbconnection: " + config.DBConnection
	str += fmt.Sprintf(`handle: %d orderwait: %d amountrate: %f sellrate %f ftmax %f`,
		config.Handle, config.OrderWait, config.AmountRate, config.SellRate, config.FtMax)
	return str
}
